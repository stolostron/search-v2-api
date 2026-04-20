package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"k8s.io/klog/v2"
)

type MockPgxConnIface interface {
	WaitForNotification(ctx context.Context) (*pgconn.Notification, error)
	Close(ctx context.Context) error
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, arguments ...interface{}) (pgx.Rows, error)
}

const (
	channelName = "search_resources_notify"
)

var (
	listenerInstance *Listener
	listenerOnce     sync.Once
	listenerMu       sync.Mutex // Protects listenerOnce and listenerInstance during reset
)

type Subscription struct {
	ID           string             // Unique UUID
	Channel      chan *model.Event  // Buffered (100)
	Context      context.Context    // Derived context — cancelled to stop the subscription gracefully
	Cancel       context.CancelFunc // Cancels Context; must be called exactly once on teardown
	CreatedAt    time.Time          // When the subscription was created
	LastActivity time.Time          // Last time an event was successfully delivered (after filters and RBAC)
	mu           sync.RWMutex       // Protects LastActivity
	// Lock ordering (outer → inner): listenerMu → listener.mu → sub.mu
}

// Listener manages the single goroutine that listens for Postgres events
type Listener struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
	conn          MockPgxConnIface //pgxmock.PgxConnIface
	ctx           context.Context
	cancel        context.CancelFunc
	started       bool
}

// RegisterSubscription registers a channel to forward events received from the database.
// Starts the listener if not already started.
// Returns a derived context that the caller must select on (it is cancelled when the
// subscription is evicted by the cleanup goroutine or unregistered), and an error if
// the maximum number of active subscriptions has been reached.
func RegisterSubscription(ctx context.Context, subID string, notifyChannel chan *model.Event) (context.Context, error) {

	// Initialize the listener instance if not already initialized.
	listenerMu.Lock()
	defer listenerMu.Unlock()
	listenerOnce.Do(func() {
		listenCtx := context.Background()
		listenCtx, listenCancel := context.WithCancel(listenCtx)
		listenerInstance = &Listener{
			subscriptions: make(map[string]*Subscription),
			conn:          nil,
			ctx:           listenCtx,
			cancel:        listenCancel,
			started:       false,
		}
		if err := listenerInstance.Start(); err != nil {
			klog.Errorf("Failed to start listener: %v", err)
		}
	})

	listenerInstance.mu.Lock()
	defer listenerInstance.mu.Unlock()

	// Check if we've reached the maximum number of active subscriptions
	maxActive := config.Cfg.Subscription.MaxActive
	if len(listenerInstance.subscriptions) >= maxActive {
		klog.Warningf("Maximum active subscriptions reached (%d). Rejecting new subscription [%s]", maxActive, subID)
		return nil, fmt.Errorf("maximum active subscriptions reached (%d)", maxActive)
	}

	// Wrap the caller's context so the cleanup goroutine can cancel this subscription
	// independently, without closing the channel directly (which would race with the
	// watchSubscription defer that also calls close(receiver)).
	subCtx, subCancel := context.WithCancel(ctx)
	now := time.Now()
	sub := &Subscription{
		ID:           subID,
		Channel:      notifyChannel,
		Context:      subCtx,
		Cancel:       subCancel,
		CreatedAt:    now,
		LastActivity: now,
	}

	listenerInstance.subscriptions[subID] = sub
	klog.Infof("Registered subscription [%s]. (%d active subscriptions)", subID, len(listenerInstance.subscriptions))
	return subCtx, nil
}

func UnregisterSubscription(subID string) {
	listenerMu.Lock()
	listener := listenerInstance
	listenerMu.Unlock()

	if listener == nil {
		return
	}

	listener.mu.Lock()
	defer listener.mu.Unlock()

	// Cancel the subscription's derived context to free its resources.
	// This is safe to call even if the cleanup goroutine already called Cancel
	// (context.CancelFunc is idempotent).
	if sub, ok := listener.subscriptions[subID]; ok {
		sub.Cancel()
	}

	delete(listener.subscriptions, subID)
	klog.Infof("Unregistered subscription %s. (%d active subscriptions)", subID, len(listener.subscriptions))

	if len(listener.subscriptions) == 0 {
		klog.Info("No more active subscriptions, shutting down listener.")
		listener.cancel()
	}
}

// UpdateSubscriptionActivity updates the last activity time for a subscription.
// Called when an event is successfully delivered to the client (after filters and RBAC),
// so idle timeout tracks actual subscription activity rather than global database traffic.
func UpdateSubscriptionActivity(subID string) {
	listenerMu.Lock()
	listener := listenerInstance
	listenerMu.Unlock()

	if listener == nil {
		return
	}

	listener.mu.RLock()
	sub, exists := listener.subscriptions[subID]
	listener.mu.RUnlock()

	if exists {
		sub.mu.Lock()
		sub.LastActivity = time.Now()
		sub.mu.Unlock()
	}
}

// Start initializes and starts the listener goroutine
func (l *Listener) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.started {
		klog.V(2).Info("Listener already started")
		return nil
	}

	if err := l.connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// TODO: We should move this TRIGGER to the search-v2-operator.
	// Register the trigger defined in listernerTrigger.sql
	listenerTriggerSQL, err := os.ReadFile("pkg/database/listenerTrigger.sql")
	if err != nil {
		return fmt.Errorf("failed to read listener trigger SQL: %w", err)
	}
	_, err = l.conn.Exec(l.ctx, string(listenerTriggerSQL))
	if err != nil {
		return fmt.Errorf("failed to create trigger: %w", err)
	}

	l.started = true
	go l.listen()
	go l.cleanupExpiredSubscriptions()
	klog.Info("Subscription postgres listener started successfully")
	return nil
}

// connect establishes a dedicated connection to Postgres for LISTEN.
// Does not use the pgxpool connection pool.
func (l *Listener) connect() error {
	cfg := config.Cfg
	dbConnString := fmt.Sprint(
		"host=", cfg.DBHost,
		" port=", cfg.DBPort,
		" user=", cfg.DBUser,
		" password=", cfg.DBPass,
		" dbname=", cfg.DBName,
		" sslmode=require",
	)

	redactedDbConn := strings.ReplaceAll(dbConnString, "password="+cfg.DBPass, "password=[REDACTED]")
	klog.V(2).Infof("Connecting subscription listener to PostgreSQL: %s", redactedDbConn)

	conn, err := pgx.Connect(l.ctx, dbConnString)
	if err != nil {
		return fmt.Errorf("unable to connect to database: %w", err)
	}

	// Start listening to the channel
	_, err = conn.Exec(l.ctx, fmt.Sprintf("LISTEN %s", channelName))
	if err != nil {
		if err := conn.Close(l.ctx); err != nil {
			klog.Errorf("Failed to close connection: %v", err)
		}
		return fmt.Errorf("unable to listen to channel %s: %w", channelName, err)
	}

	l.conn = conn
	klog.V(2).Infof("Listening to Postgres channel: %s", channelName)
	return nil
}

// listen is the main goroutine that receives notifications and forwards them
func (l *Listener) listen() {
	defer func() {
		klog.V(1).Info("Subscription listener shutting down...")
		if l.conn != nil {
			if err := l.conn.Close(context.Background()); err != nil {
				klog.Errorf("Failed to close connection: %v", err)
				return
			}
		}
		listenerMu.Lock()
		defer listenerMu.Unlock()
		listenerInstance = nil
		listenerOnce = sync.Once{}
		klog.Info("Subscription listener stopped.")
	}()

	for {
		select {
		case <-l.ctx.Done():
			klog.V(2).Info("Listener context cancelled, shutting down.")
			return
		default:
			if l.conn == nil {
				// Connection lost, attempt to reconnect.
				l.handleConnectionError()
				continue
			}
			// Wait for notification with timeout
			klog.V(3).Infof("Waiting for notification on: %s", channelName)
			notification, err := l.conn.WaitForNotification(l.ctx)

			if err != nil {
				if l.ctx.Err() != nil {
					// Context was cancelled, exit gracefully
					return
				}
				klog.Errorf("Error waiting for notification: %v", err)
				l.handleConnectionError()
				continue
			}

			if notification != nil {
				l.forwardNotification(notification)
			}
		}
	}
}

// forwardNotification parses a Postgres notification and sends it to all registered subscriptions.
// The RLock is held only long enough to snapshot the current subscribers; the DB backfill and
// channel sends happen outside the lock so they cannot block RegisterSubscription/UnregisterSubscription.
func (l *Listener) forwardNotification(notification *pgconn.Notification) {
	var notificationPayload model.Event
	err := json.Unmarshal([]byte(notification.Payload), &notificationPayload)
	if err != nil {
		klog.Errorf("Failed to unmarshal notification payload: %v", err)
		return
	}

	// If the notification payload was too large, data was truncated.
	// We need to query the database to rebuild the data.
	// This DB call happens without holding l.mu to avoid blocking subscription registration.
	if notificationPayload.NewData == nil &&
		(notificationPayload.Operation == "INSERT" || notificationPayload.Operation == "UPDATE") {
		klog.V(2).Infof("Notification payload missing newData, querying the database. UID: %s", notificationPayload.UID)
		rows, err := l.conn.Query(l.ctx, "SELECT data FROM search.resources WHERE uid = $1", notificationPayload.UID)
		if err != nil {
			klog.Errorf("Failed to execute query: %v", err)
			return
		}

		// Explicitly close rows after processing to avoid resource leak
		for rows.Next() {
			var data map[string]any
			err := rows.Scan(&data)
			if err != nil {
				klog.Errorf("Failed to scan result: %v", err)
				continue
			}
			notificationPayload.NewData = data
		}
		rows.Close()

		// Check for errors from iteration
		if err := rows.Err(); err != nil {
			klog.Errorf("Error iterating rows: %v", err)
		}
	}
	if notificationPayload.OldData == nil &&
		(notificationPayload.Operation == "UPDATE" || notificationPayload.Operation == "DELETE") {
		klog.Warningf("Notification payload was truncated, 'oldData' is missing. This is a current limitation. UID: %s", notificationPayload.UID)
	}

	// Snapshot subscribers under RLock, then release before sending to avoid
	// blocking RegisterSubscription/UnregisterSubscription on slow channel sends.
	l.mu.RLock()
	subs := make([]*Subscription, 0, len(l.subscriptions))
	for _, sub := range l.subscriptions {
		subs = append(subs, sub)
	}
	klog.V(3).Infof("Received postgres event, forwarding to %d subscriptions.", len(subs))
	l.mu.RUnlock()

	for _, sub := range subs {
		select {
		case <-sub.Context.Done():
			klog.V(3).Infof("Subscription %s context is done, skipping", sub.ID)
		case sub.Channel <- &notificationPayload:
		default:
			klog.Warningf("Subscription %s channel buffer is full, dropping event.", sub.ID)
		}
	}
}

// handleConnectionError attempts to reconnect to the database
func (l *Listener) handleConnectionError() {
	klog.Warning("Connection lost, attempting to reconnect...")

	l.mu.Lock()
	if l.conn != nil {
		if err := l.conn.Close(context.Background()); err != nil {
			klog.Errorf("Failed to close connection: %v", err)
		}
		l.conn = nil
	}
	l.mu.Unlock()

	// Wait before reconnecting
	time.Sleep(time.Duration(config.Cfg.DBReconnectDelay) * time.Millisecond)

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.connect(); err != nil {
		klog.Errorf("Failed to reconnect: %v", err)
	} else {
		klog.Info("Successfully reconnected to database")
	}
}

// cleanupExpiredSubscriptions periodically checks for subscriptions that have exceeded
// their maximum lifetime or idle timeout and closes them.
func (l *Listener) cleanupExpiredSubscriptions() {
	// Check for expired subscriptions at the configured interval
	cleanupInterval := time.Duration(config.Cfg.Subscription.CleanupInterval) * time.Millisecond
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			klog.V(2).Info("Subscriptions cleanup goroutine shutting down.")
			return
		case <-ticker.C:
			l.checkAndCloseExpiredSubscriptions()
		}
	}
}

// checkAndCloseExpiredSubscriptions checks all subscriptions and closes those that have expired.
func (l *Listener) checkAndCloseExpiredSubscriptions() {
	now := time.Now()
	maxLifetime := time.Duration(config.Cfg.Subscription.MaxLifetime) * time.Millisecond
	idleTimeout := time.Duration(config.Cfg.Subscription.IdleTimeout) * time.Millisecond

	l.mu.RLock()
	defer l.mu.RUnlock()
	for subID, sub := range l.subscriptions {
		sub.mu.RLock()
		age := now.Sub(sub.CreatedAt)
		idleTime := now.Sub(sub.LastActivity)
		sub.mu.RUnlock()

		// Check if subscription has exceeded max lifetime
		if age > maxLifetime {
			klog.Infof("Subscription [%s] exceeded max lifetime (%v). Closing.", subID, maxLifetime)
			sub.Cancel()
			continue
		}

		// Check if subscription has been idle for too long
		if idleTime > idleTimeout {
			klog.Infof("Subscription [%s] idle for %v (max: %v). Closing.", subID, idleTime, idleTimeout)
			sub.Cancel()
		}
	}

	// Cancel expired subscriptions via their derived context.
	// This signals the watchSubscription goroutine to exit via <-ctx.Done(), which then
	// runs its defer (UnregisterSubscription + close(channel)) exactly once — avoiding the
	// double-close panic that would occur if we closed sub.Channel directly here.
	// context.CancelFunc is idempotent, so a concurrent UnregisterSubscription call is safe.
}

// StopPostgresListener stops the Postgres listenerInstance.
func StopPostgresListener() {
	listenerMu.Lock()
	defer listenerMu.Unlock()

	// Cancel the listener context if it exists
	if listenerInstance != nil && listenerInstance.cancel != nil {
		listenerInstance.cancel()
		// Give the listener goroutine time to shut down
		time.Sleep(50 * time.Millisecond)
	}

	listenerInstance = nil
	listenerOnce = sync.Once{}
	klog.Info("Postgres listener stopped.")
}
