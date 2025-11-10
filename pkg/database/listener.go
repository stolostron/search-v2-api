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
	listenerOnce.Do(func() {
		listenCtx := context.Background() // FIXME: use context from main.
		listenCtx, listenCancel := context.WithCancel(listenCtx)
		listenerInstance = &Listener{
			subscriptions: make([]*Subscription, 0),
			conn:          nil,
			ctx:           listenCtx,
			cancel:        listenCancel,
			started:       false,
		}
		listenerInstance.Start()
	})

	sub := &Subscription{
		ID:      uid,
		Channel: notifyChannel,
		Context: ctx,
	}
	listenerInstance.subscriptions = append(listenerInstance.subscriptions, sub)
}

func UnregisterSubscription(uid string) {
	listenerInstance.mu.Lock()
	defer listenerInstance.mu.Unlock()
	for i, sub := range listenerInstance.subscriptions {
		if sub.ID == uid {
			listenerInstance.subscriptions = append(listenerInstance.subscriptions[:i], listenerInstance.subscriptions[i+1:]...)
		}
	}
	klog.Infof("Unregistered subscription %s, %d subscriptions remaining.", uid, len(listenerInstance.subscriptions))

	if len(listenerInstance.subscriptions) == 0 {
		klog.Info("No more activesubscriptions, shutting down listener.")
		listenerInstance.cancel()
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

	// FIXME: We should create the trigger from the search-v2-operator.
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
		conn.Close(l.ctx)
		return fmt.Errorf("unable to listen to channel %s: %w", channelName, err)
	}

	l.conn = conn
	klog.V(2).Infof("Successfully listening to Postgres channel: %s", channelName)
	return nil
}

// listen is the main goroutine that receives notifications and forwards them
func (l *Listener) listen() {
	defer func() {
		if l.conn != nil {
			l.conn.Close(context.Background())
		}
		klog.Info("Subscription listener stopped")
	}()

	for {
		select {
		case <-l.ctx.Done():
			klog.Info("Listener context cancelled, shutting down")
			return
		default:
			// Wait for notification with timeout
			klog.Info("Waiting for notification...")
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
				klog.Infof("Received postgres event, forwarding to %d subscriptions. %+v ",
					len(l.subscriptions), notification)

				l.mu.RLock()
				defer l.mu.RUnlock()
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
			}
		}
	}
}

// handleConnectionError attempts to reconnect to the database
func (l *Listener) handleConnectionError() {
	klog.Warning("Connection lost, attempting to reconnect...")

	l.mu.Lock()
	if l.conn != nil {
		l.conn.Close(context.Background())
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
