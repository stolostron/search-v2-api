// Copyright Contributors to the Open Cluster Management project
package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/stolostron/search-v2-api/pkg/config"
	"k8s.io/klog/v2"
)

// NotificationPayload represents the structure of notification data
type NotificationPayload struct {
	Operation string                 `json:"operation"` // INSERT, UPDATE, DELETE
	Table     string                 `json:"table"`     // Table name
	UID       string                 `json:"uid"`       // Resource UID
	Cluster   string                 `json:"cluster"`   // Cluster name
	OldData   map[string]interface{} `json:"old_data,omitempty"`
	NewData   map[string]interface{} `json:"new_data,omitempty"`
	// Timestamp time.Time              `json:"timestamp"`
}

// NotificationFilter defines criteria for filtering notifications
type NotificationFilter struct {
	Kinds      []string               `json:"kinds,omitempty"`      // Filter by resource kinds
	Namespaces []string               `json:"namespaces,omitempty"` // Filter by namespaces
	Clusters   []string               `json:"clusters,omitempty"`   // Filter by clusters
	Labels     map[string]string      `json:"labels,omitempty"`     // Filter by labels
	Properties map[string]interface{} `json:"properties,omitempty"` // Filter by other properties
	Operations []string               `json:"operations,omitempty"` // Filter by operations (INSERT, UPDATE, DELETE)
}

// Subscription represents a notification subscription
type Subscription struct {
	ID       string                   `json:"id"`
	Filter   NotificationFilter       `json:"filter"`
	Channel  chan NotificationPayload `json:"-"`
	UserData interface{}              `json:"user_data,omitempty"` // Store user RBAC data for filtering
	Active   bool                     `json:"active"`
	Created  time.Time                `json:"created"`
}

// Listener manages PostgreSQL LISTEN/NOTIFY functionality
type Listener struct {
	conn            *pgx.Conn
	subscriptions   map[string]*Subscription
	subscriptionsMu sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	channelName     string
}

// NotificationManager is a singleton instance for managing notifications
var (
	notificationManager *Listener
	managerOnce         sync.Once
)

// GetNotificationManager returns the singleton notification manager
func GetNotificationManager() *Listener {
	managerOnce.Do(func() {
		notificationManager = &Listener{
			subscriptions: make(map[string]*Subscription),
			channelName:   config.Cfg.NotificationChannelName,
		}
	})
	return notificationManager
}

// Start initializes the notification listener
func (l *Listener) Start(ctx context.Context) error {
	l.ctx, l.cancel = context.WithCancel(ctx)

	if !config.Cfg.Features.NotificationEnabled {
		klog.Info("PostgreSQL notifications are disabled. To enable set FEATURE_NOTIFICATION=true")
		return nil
	}

	klog.Info("Starting PostgreSQL notification listener...")

	if err := l.connect(); err != nil {
		return fmt.Errorf("failed to connect for notifications: %w", err)
	}

	if err := l.listen(); err != nil {
		return fmt.Errorf("failed to start listening: %w", err)
	}

	l.wg.Add(1)
	go l.handleNotifications()

	klog.Infof("PostgreSQL notification listener started on channel: %s", l.channelName)
	return nil
}

// Stop gracefully shuts down the notification listener
func (l *Listener) Stop() {
	if l.cancel != nil {
		l.cancel()
	}
	l.wg.Wait()

	if l.conn != nil {
		l.conn.Close(context.Background())
	}

	l.subscriptionsMu.Lock()
	defer l.subscriptionsMu.Unlock()

	// Close all subscription channels
	for _, sub := range l.subscriptions {
		if sub.Active {
			close(sub.Channel)
			sub.Active = false
		}
	}

	klog.Info("PostgreSQL notification listener stopped")
}

// connect establishes a dedicated connection for LISTEN/NOTIFY
func (l *Listener) connect() error {
	cfg := config.Cfg
	connString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPass, cfg.DBName,
	)

	conn, err := pgx.Connect(l.ctx, connString)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	l.conn = conn
	return nil
}

// listen starts listening on the PostgreSQL notification channel
func (l *Listener) listen() error {
	sql := fmt.Sprintf("LISTEN %s", l.channelName)
	_, err := l.conn.Exec(l.ctx, sql)
	if err != nil {
		return fmt.Errorf("failed to LISTEN on channel %s: %w", l.channelName, err)
	}

	klog.V(2).Infof("Started LISTEN on channel: %s", l.channelName)
	return nil
}

// handleNotifications processes incoming PostgreSQL notifications
func (l *Listener) handleNotifications() {
	defer l.wg.Done()

	klog.V(2).Info("Starting notification handler goroutine")

	retryCount := 0
	maxRetries := config.Cfg.NotificationMaxRetries

	for {
		select {
		case <-l.ctx.Done():
			klog.V(2).Info("Notification handler context cancelled")
			return
		default:
			notification, err := l.conn.WaitForNotification(l.ctx)
			if err != nil {
				retryCount++
				if retryCount > maxRetries {
					klog.Errorf("Max retries exceeded for PostgreSQL notifications: %v", err)
					return
				}

				klog.Warningf("Error waiting for notification (retry %d/%d): %v", retryCount, maxRetries, err)

				// Attempt to reconnect
				if err := l.reconnect(); err != nil {
					klog.Errorf("Failed to reconnect: %v", err)
					time.Sleep(time.Duration(config.Cfg.NotificationReconnectDelay) * time.Millisecond)
					continue
				}

				continue
			}

			// Reset retry count on successful notification
			retryCount = 0

			if notification != nil {
				l.processNotification(notification)
			}
		}
	}
}

// reconnect attempts to reestablish the PostgreSQL connection
func (l *Listener) reconnect() error {
	klog.Info("Attempting to reconnect to PostgreSQL for notifications...")

	if l.conn != nil {
		l.conn.Close(context.Background())
	}

	if err := l.connect(); err != nil {
		return err
	}

	if err := l.listen(); err != nil {
		return err
	}

	klog.Info("Successfully reconnected to PostgreSQL for notifications")
	return nil
}

// processNotification parses and distributes notifications to subscribers
func (l *Listener) processNotification(notification *pgconn.Notification) {
	klog.V(5).Infof("Received notification on channel %s: %s", notification.Channel, notification.Payload)

	var payload NotificationPayload
	if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
		klog.Errorf("Failed to parse notification payload: %v", err)
		klog.Infof("Notification payload: %v", notification.Payload)
		return
	}

	l.subscriptionsMu.RLock()
	defer l.subscriptionsMu.RUnlock()

	for _, sub := range l.subscriptions {
		if !sub.Active {
			continue
		}

		if l.matchesFilter(payload, sub.Filter) {
			select {
			case sub.Channel <- payload:
				klog.V(4).Infof("Received notification for subscription %s", sub.ID)
			default:
				klog.Warningf("Subscription %s channel is full, dropping notification", sub.ID)
			}
		}
	}
}

// matchesFilter checks if a notification payload matches the subscription filter
func (l *Listener) matchesFilter(payload NotificationPayload, filter NotificationFilter) bool {
	// Filter by operations
	if len(filter.Operations) > 0 {
		found := false
		for _, op := range filter.Operations {
			if payload.Operation == op {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by clusters
	if len(filter.Clusters) > 0 {
		found := false
		for _, cluster := range filter.Clusters {
			if payload.Cluster == cluster {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Get the data to check (new data for INSERT/UPDATE, old data for DELETE)
	var dataToCheck map[string]interface{}
	if payload.Operation == "DELETE" && payload.OldData != nil {
		dataToCheck = payload.OldData
	} else if payload.NewData != nil {
		dataToCheck = payload.NewData
	} else {
		return false
	}

	// Filter by kinds
	if len(filter.Kinds) > 0 {
		kind, ok := dataToCheck["kind"].(string)
		if !ok {
			return false
		}

		found := false
		for _, filterKind := range filter.Kinds {
			if kind == filterKind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by namespaces
	if len(filter.Namespaces) > 0 {
		namespace, ok := dataToCheck["namespace"].(string)
		if !ok {
			// Resource without namespace, check if filter includes empty namespace
			found := false
			for _, filterNs := range filter.Namespaces {
				if filterNs == "" {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		} else {
			found := false
			for _, filterNs := range filter.Namespaces {
				if namespace == filterNs {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Filter by labels
	if len(filter.Labels) > 0 {
		labels, ok := dataToCheck["label"].(map[string]interface{})
		if !ok {
			return false
		}

		for key, value := range filter.Labels {
			if labelValue, exists := labels[key]; !exists || labelValue != value {
				return false
			}
		}
	}

	// Filter by custom properties
	if len(filter.Properties) > 0 {
		for key, value := range filter.Properties {
			if propValue, exists := dataToCheck[key]; !exists || propValue != value {
				return false
			}
		}
	}

	return true
}

// Subscribe creates a new notification subscription
func (l *Listener) Subscribe(id string, filter NotificationFilter, userData interface{}) (*Subscription, error) {
	l.subscriptionsMu.Lock()
	defer l.subscriptionsMu.Unlock()

	// Check if subscription already exists
	if _, exists := l.subscriptions[id]; exists {
		return nil, fmt.Errorf("subscription with ID %s already exists", id)
	}

	subscription := &Subscription{
		ID:       id,
		Filter:   filter,
		Channel:  make(chan NotificationPayload, config.Cfg.NotificationBufferSize),
		UserData: userData,
		Active:   true,
		Created:  time.Now(),
	}

	l.subscriptions[id] = subscription

	klog.V(2).Infof("Created notification subscription: %s", id)
	return subscription, nil
}

// Unsubscribe removes a notification subscription
func (l *Listener) Unsubscribe(id string) error {
	l.subscriptionsMu.Lock()
	defer l.subscriptionsMu.Unlock()

	subscription, exists := l.subscriptions[id]
	if !exists {
		return fmt.Errorf("subscription with ID %s not found", id)
	}

	subscription.Active = false
	close(subscription.Channel)
	delete(l.subscriptions, id)

	klog.V(2).Infof("Removed notification subscription: %s", id)
	return nil
}

// GetSubscription returns a subscription by ID
func (l *Listener) GetSubscription(id string) (*Subscription, bool) {
	l.subscriptionsMu.RLock()
	defer l.subscriptionsMu.RUnlock()

	sub, exists := l.subscriptions[id]
	return sub, exists
}

// ListSubscriptions returns all active subscriptions
func (l *Listener) ListSubscriptions() []*Subscription {
	l.subscriptionsMu.RLock()
	defer l.subscriptionsMu.RUnlock()

	subscriptions := make([]*Subscription, 0, len(l.subscriptions))
	for _, sub := range l.subscriptions {
		if sub.Active {
			subscriptions = append(subscriptions, sub)
		}
	}

	return subscriptions
}
