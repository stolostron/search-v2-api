package database

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"k8s.io/klog/v2"
)

var (
	channelName      = "search_resources_changes"
	listenerInstance *Listener
	listenerOnce     sync.Once
	listenerMu       sync.Mutex // Protects listenerOnce and listenerInstance during reset
)

type Subscription struct {
	ID      string            // Unique UUID
	Channel chan *model.Event // Buffered (100)
	Context context.Context   // For cleanup
}

// Listener manages the single goroutine that listens for Postgres events
type Listener struct {
	mu            sync.RWMutex
	subscriptions []*Subscription
	conn          *pgx.Conn
	ctx           context.Context
	cancel        context.CancelFunc
	started       bool
}

// RegisterSubscriptionAndListen registers a channel to send events received from the database.
// It is used to send events to the subscription resolver.
func RegisterSubscriptionAndListen(ctx context.Context, uid string, notifyChannel chan *model.Event) {

	// Iniialize the listener instance if not already initialized.
	listenerMu.Lock()
	defer listenerMu.Unlock()
	listenerOnce.Do(func() {
		listenCtx := context.Background()
		listenCtx, listenCancel := context.WithCancel(listenCtx)
		listenerInstance = &Listener{
			subscriptions: make([]*Subscription, 0),
			conn:          nil,
			ctx:           listenCtx,
			cancel:        listenCancel,
			started:       false,
		}
		if err := listenerInstance.Start(); err != nil {
			klog.Errorf("Failed to start listener: %v", err)
		}
	})

	sub := &Subscription{
		ID:      uid,
		Channel: notifyChannel,
		Context: ctx,
	}

	listenerInstance.mu.Lock()
	defer listenerInstance.mu.Unlock()
	listenerInstance.subscriptions = append(listenerInstance.subscriptions, sub)
	klog.Infof("Registered subscription %s, %d subscriptions total.", uid, len(listenerInstance.subscriptions))
}

func UnregisterSubscription(uid string) {
	listenerMu.Lock()
	listener := listenerInstance
	listenerMu.Unlock()

	if listener == nil {
		return
	}

	listener.mu.Lock()
	defer listener.mu.Unlock()
	for i, sub := range listener.subscriptions {
		if sub.ID == uid {
			listener.subscriptions = append(listener.subscriptions[:i], listener.subscriptions[i+1:]...)
		}
	}
	klog.Infof("Unregistered subscription %s, %d subscriptions remaining.", uid, len(listener.subscriptions))

	if len(listener.subscriptions) == 0 {
		klog.Info("No more active subscriptions, shutting down listener.")
		listener.cancel()
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

	// FIXME: We should move this TRIGGER to the search-v2-operator.
	// Register the trigger defined in listernerTrigger.sql
	listenerTriggerSQL, err := os.ReadFile("pkg/database/listenerTrigger.sql")
	if err != nil {
		return fmt.Errorf("failed to read listener trigger SQL: %w", err)
	}
	_, err = l.conn.Exec(l.ctx, string(listenerTriggerSQL))
	if err != nil {
		return fmt.Errorf("failed to create trigger: %w", err)
	} else {
		klog.Info("Successfully created postgres TRIGGER to NOTIFY the listener.")
	}

	l.started = true
	go l.listen()
	klog.Info("Subscription listener started successfully")
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
			}
		}
		klog.Info("Subscription listener stopped.")
	}()

	for {
		select {
		case <-l.ctx.Done():
			klog.V(2).Info("Listener context cancelled, shutting down.")
			return
		default:
			// Wait for notification with timeout
			klog.V(3).Infof("Waiting for notification on: %s", channelName)
			notification, err := l.conn.WaitForNotification(l.ctx) // FIXME: this panics when connection is lost.

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
				l.mu.RLock()
				klog.V(3).Infof("Received postgres event, forwarding to %d subscriptions.",
					len(l.subscriptions))

				for _, sub := range l.subscriptions {
					var notificationPayload model.Event
					err := json.Unmarshal([]byte(notification.Payload), &notificationPayload)
					if err != nil {
						klog.Errorf("Failed to unmarshal notification payload: %v", err)
						continue
					}

					select {
					case <-sub.Context.Done():
						klog.V(3).Infof("Subscription %s context is done, skipping", sub.ID)
						// FIXME: remove subscription from list.

						continue
					default:
						sub.Channel <- &notificationPayload
					}
				}
				l.mu.RUnlock()
			}
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
	time.Sleep(time.Duration(config.Cfg.DBReconnectDelay) * time.Second)

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.connect(); err != nil {
		klog.Errorf("Failed to reconnect: %v", err)
	} else {
		klog.Info("Successfully reconnected to database")
	}
}

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
