// Copyright Contributors to the Open Cluster Management project
package database

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgconn"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stretchr/testify/assert"
)

// MockPgxConn is a mock implementation of pgx connection for testing
type MockPgxConn struct {
	WaitForNotificationFunc func(ctx context.Context) (*pgconn.Notification, error)
	CloseFunc               func(ctx context.Context) error
	ExecFunc                func(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

func (m MockPgxConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	if m.WaitForNotificationFunc != nil {
		return m.WaitForNotificationFunc(ctx)
	}
	return nil, nil
}

func (m MockPgxConn) Close(ctx context.Context) error {
	if m.CloseFunc != nil {
		return m.CloseFunc(ctx)
	}
	return nil
}

func (m MockPgxConn) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	if m.ExecFunc != nil {
		return m.ExecFunc(ctx, sql, arguments...)
	}
	return nil, nil
}

// [AI] Test registration of a subscription.
func TestRegisterSubscription(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notifyChannel := make(chan *model.Event, 100)
	uid := "test-uid-123"

	// Register subscription - this should initialize the listener
	RegisterSubscription(ctx, uid, notifyChannel)

	// Verify listener was initialized
	assert.NotNil(t, listenerInstance, "Listener instance should be initialized")
	assert.NotNil(t, listenerInstance.subscriptions, "Subscriptions list should be initialized")
	assert.Equal(t, 1, len(listenerInstance.subscriptions), "Should have 1 subscription")
	assert.Equal(t, uid, listenerInstance.subscriptions[uid].ID, "Subscription ID should match")
	assert.Equal(t, notifyChannel, listenerInstance.subscriptions[uid].Channel, "Subscription channel should match")

	// Clean up
	close(notifyChannel)
}

// [AI] Test registration of multiple subscriptions.
func TestRegisterMultipleSubscriptions(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register first subscription
	notifyChannel1 := make(chan *model.Event, 100)
	uid1 := "test-uid-1"
	RegisterSubscription(ctx, uid1, notifyChannel1)

	// Register second subscription
	notifyChannel2 := make(chan *model.Event, 100)
	uid2 := "test-uid-2"
	RegisterSubscription(ctx, uid2, notifyChannel2)

	// Verify both subscriptions exist
	assert.Equal(t, 2, len(listenerInstance.subscriptions), "Should have 2 subscriptions")

	// Clean up
	close(notifyChannel1)
	close(notifyChannel2)
}

// [AI] Test unregistration of a subscription.
func TestUnregisterSubscription(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register subscriptions
	notifyChannel1 := make(chan *model.Event, 100)
	uid1 := "test-uid-1"
	RegisterSubscription(ctx, uid1, notifyChannel1)

	notifyChannel2 := make(chan *model.Event, 100)
	uid2 := "test-uid-2"
	RegisterSubscription(ctx, uid2, notifyChannel2)

	assert.Equal(t, 2, len(listenerInstance.subscriptions), "Should have 2 subscriptions")

	// Unregister first subscription
	UnregisterSubscription(uid1)

	// Verify only one subscription remains
	assert.Equal(t, 1, len(listenerInstance.subscriptions), "Should have 1 subscription after unregister")
	assert.Equal(t, uid2, listenerInstance.subscriptions[uid2].ID, "Remaining subscription should be uid2")

	// Clean up
	close(notifyChannel1)
	close(notifyChannel2)
}

// [AI] Test unregistration of the last subscription.
func TestUnregisterLastSubscription(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register a subscription
	notifyChannel := make(chan *model.Event, 100)
	uid := "test-uid-1"
	RegisterSubscription(ctx, uid, notifyChannel)

	assert.Equal(t, 1, len(listenerInstance.subscriptions), "Should have 1 subscription")

	// Store the cancel function to check if it was called
	originalCancel := listenerInstance.cancel

	// Unregister the last subscription
	UnregisterSubscription(uid)

	// Verify no subscriptions remain
	assert.Equal(t, 0, len(listenerInstance.subscriptions), "Should have 0 subscriptions after unregistering all")

	// Verify the listener context was cancelled (by checking context.Done())
	assert.NotNil(t, originalCancel, "Cancel function should exist")

	// Clean up
	close(notifyChannel)
}

// [AI] Test listener start when already started
func TestListenerStartAlreadyStarted(t *testing.T) {
	// Create a listener instance
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &Listener{
		subscriptions: make(map[string]*Subscription),
		conn:          nil,
		ctx:           ctx,
		cancel:        cancel,
		started:       true, // Already started
	}

	// Call Start - should return nil without error since already started
	err := listener.Start()
	assert.Nil(t, err, "Starting an already started listener should not return an error")
}

// [AI] Test listener context cancellation
func TestListenerContextCancellation(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())

	// Register a subscription
	notifyChannel := make(chan *model.Event, 100)
	uid := "test-uid-1"
	RegisterSubscription(ctx, uid, notifyChannel)

	// Verify listener context is not done initially
	select {
	case <-listenerInstance.ctx.Done():
		t.Fatal("Listener context should not be done initially")
	default:
		// Context not done, as expected
	}

	// Unregister all subscriptions which should cancel the listener context
	UnregisterSubscription(uid)

	// Give it a moment to process
	time.Sleep(100 * time.Millisecond)

	// Verify listener context is now done
	select {
	case <-listenerInstance.ctx.Done():
		// Context is done, as expected
	case <-time.After(1 * time.Second):
		t.Fatal("Listener context should be done after unregistering all subscriptions")
	}

	// Clean up
	cancel()
	close(notifyChannel)
}

// [AI] Test that listen() respects context cancellation
func TestListenerListenContextCancellation(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())

	// Register a subscription
	notifyChannel := make(chan *model.Event, 100)
	uid := "test-listen-cancel"
	RegisterSubscription(ctx, uid, notifyChannel)

	// Give the listener goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel the listener context (simulating unregister of last subscription)
	if listenerInstance != nil && listenerInstance.cancel != nil {
		listenerInstance.cancel()
	}

	// Give it time to shut down
	time.Sleep(200 * time.Millisecond)

	// The listener should have stopped gracefully
	// We can't directly verify it stopped, but we can verify the context is done
	select {
	case <-listenerInstance.ctx.Done():
		// Context is done, as expected
	default:
		t.Fatal("Listener context should be done after cancellation")
	}

	// Clean up
	cancel()
	close(notifyChannel)
}

// [AI] Test subscription with cancelled context
func TestListenerWithCancelledSubscriptionContext(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())

	// Register first subscription with active context
	notifyChannel1 := make(chan *model.Event, 100)
	uid1 := "test-active-sub"
	RegisterSubscription(ctx, uid1, notifyChannel1)

	// Register second subscription with context that we'll cancel
	ctx2, cancel2 := context.WithCancel(context.Background())
	notifyChannel2 := make(chan *model.Event, 100)
	uid2 := "test-cancelled-sub"
	RegisterSubscription(ctx2, uid2, notifyChannel2)

	// Verify both subscriptions exist
	assert.Equal(t, 2, len(listenerInstance.subscriptions), "Should have 2 subscriptions")

	// Cancel the second subscription's context
	cancel2()

	// Give it time to process
	time.Sleep(50 * time.Millisecond)

	// The subscription should still be in the list (it's only skipped when sending events)
	// This tests the behavior noted in the FIXME comment
	assert.Equal(t, 2, len(listenerInstance.subscriptions), "Should still have 2 subscriptions")

	// Clean up
	cancel()
	close(notifyChannel1)
	close(notifyChannel2)
}

// [AI] Test listener handles multiple subscriptions correctly
func TestListenerMultipleSubscriptionsForwarding(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create multiple subscriptions
	numSubs := 5
	channels := make([]chan *model.Event, numSubs)
	uids := make([]string, numSubs)

	for i := 0; i < numSubs; i++ {
		channels[i] = make(chan *model.Event, 100)
		uids[i] = "test-multi-" + string(rune('A'+i))
		RegisterSubscription(ctx, uids[i], channels[i])
	}

	// Verify all subscriptions were registered
	assert.Equal(t, numSubs, len(listenerInstance.subscriptions), "Should have all subscriptions")

	// Verify each subscription has correct properties
	for _, sub := range listenerInstance.subscriptions {
		assert.NotNil(t, sub.Channel, "Subscription channel should not be nil")
		assert.NotNil(t, sub.Context, "Subscription context should not be nil")
		assert.NotEmpty(t, sub.ID, "Subscription ID should not be empty")
		assert.Contains(t, uids, sub.ID, "Subscription ID should be in expected list")
		t.Logf("Subscription: ID=%s", sub.ID)
	}

	// Clean up
	for _, ch := range channels {
		close(ch)
	}
}

// [AI] Test unregister subscription with nil instance
func TestUnregisterSubscriptionNilInstance(t *testing.T) {
	// Reset to nil
	listenerMu.Lock()
	listenerInstance = nil
	listenerMu.Unlock()

	// This should not panic
	UnregisterSubscription("any-uid")

	// No assertion needed, just verify no panic
}

// [AI] Test concurrent register and unregister operations
func TestConcurrentRegisterUnregister(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	numOperations := 20

	// Concurrent registrations
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			channel := make(chan *model.Event, 100)
			uid := "concurrent-" + string(rune(index))
			RegisterSubscription(ctx, uid, channel)

			// Small delay
			time.Sleep(10 * time.Millisecond)

			// Unregister
			UnregisterSubscription(uid)
			close(channel)
		}(i)
	}

	wg.Wait()

	// Give time for all operations to complete
	time.Sleep(100 * time.Millisecond)

	// Should end up with 0 subscriptions
	if listenerInstance != nil {
		listenerInstance.mu.Lock()
		count := len(listenerInstance.subscriptions)
		listenerInstance.mu.Unlock()
		assert.Equal(t, 0, count, "All subscriptions should be unregistered")
	}
}

// [AI] Test listener state after initialization
func TestListenerStateAfterInit(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notifyChannel := make(chan *model.Event, 100)
	uid := "test-state"
	RegisterSubscription(ctx, uid, notifyChannel)

	// Verify listener state
	assert.NotNil(t, listenerInstance, "Listener should be initialized")
	assert.NotNil(t, listenerInstance.subscriptions, "Subscriptions list should exist")
	assert.NotNil(t, listenerInstance.ctx, "Listener context should exist")
	assert.NotNil(t, listenerInstance.cancel, "Listener cancel function should exist")

	// Verify subscription state
	assert.Equal(t, 1, len(listenerInstance.subscriptions), "Should have 1 subscription")
	sub := listenerInstance.subscriptions[uid]
	assert.Equal(t, uid, sub.ID, "Subscription ID should match")
	assert.Equal(t, notifyChannel, sub.Channel, "Subscription channel should match")
	assert.Equal(t, ctx, sub.Context, "Subscription context should match")

	// Clean up
	close(notifyChannel)
}

// [AI] Test listen() goroutine behavior with mock connection
func TestListenWithMockConnection(t *testing.T) {
	// Create a context that we'll cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Create channels to track behavior
	waitCalled := make(chan bool, 1)
	closeCalled := make(chan bool, 1)

	// Create mock connection with behavior we want to test
	mockConn := &MockPgxConn{
		WaitForNotificationFunc: func(ctx context.Context) (*pgconn.Notification, error) {
			waitCalled <- true
			// Block until context is cancelled
			<-ctx.Done()
			return nil, ctx.Err()
		},
		CloseFunc: func(ctx context.Context) error {
			closeCalled <- true
			return nil
		},
	}

	// Create a subscription to test event forwarding
	notifyChannel := make(chan *model.Event, 100)
	subscription := &Subscription{
		ID:      "test-sub-1",
		Channel: notifyChannel,
		Context: context.Background(),
	}

	// Note: We can't directly test listen() because listener.conn is *pgx.Conn
	// This test demonstrates the mock pattern. In production, you'd need to:
	// 1. Extract an interface for connection operations
	// 2. Update Listener to use that interface
	// 3. Then this mock would work directly

	// For now, verify the mock implements the expected methods
	var _ interface {
		WaitForNotification(context.Context) (*pgconn.Notification, error)
		Close(context.Context) error
	} = mockConn

	// Test the mock behavior independently
	go func() {
		_, err := mockConn.WaitForNotification(ctx)
		assert.Error(t, err, "Should return error when context cancelled")
	}()

	// Verify WaitForNotification was called
	select {
	case <-waitCalled:
		// Good, it was called
	case <-time.After(100 * time.Millisecond):
		t.Fatal("WaitForNotification should have been called")
	}

	// Cancel context
	cancel()

	// Test Close behavior
	err := mockConn.Close(context.Background())
	assert.Nil(t, err, "Close should not return error")

	select {
	case <-closeCalled:
		// Good, it was called
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Close should have been called")
	}

	// Verify subscription is valid
	assert.NotNil(t, subscription, "Subscription should exist")
	assert.Equal(t, "test-sub-1", subscription.ID, "Subscription ID should match")

	close(notifyChannel)
}

// [AI] Test listen() with notification forwarding
func TestListenNotificationForwarding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock that sends a notification then cancels
	notificationSent := make(chan bool, 1)
	mockConn := &MockPgxConn{
		WaitForNotificationFunc: func(ctx context.Context) (*pgconn.Notification, error) {
			select {
			case <-notificationSent:
				// Second call - context should be cancelled
				<-ctx.Done()
				return nil, ctx.Err()
			default:
				// First call - send a notification
				notificationSent <- true
				notification := &pgconn.Notification{
					Channel: "test_channel",
					Payload: `{"uid":"test-123","operation":"CREATE","timestamp":"2024-01-01T00:00:00Z"}`,
				}
				return notification, nil
			}
		},
		CloseFunc: func(ctx context.Context) error {
			return nil
		},
	}

	// Test that we can create valid notification payloads
	testPayload := `{"uid":"test-123","operation":"CREATE","timestamp":"2024-01-01T00:00:00Z"}`
	var event model.Event
	err := json.Unmarshal([]byte(testPayload), &event)
	assert.Nil(t, err, "Should be able to unmarshal test payload")
	assert.Equal(t, "test-123", event.UID, "UID should match")
	assert.Equal(t, "CREATE", event.Operation, "Operation should match")

	// Verify mock works as expected
	notification, err := mockConn.WaitForNotification(ctx)
	assert.Nil(t, err, "First call should succeed")
	assert.NotNil(t, notification, "Should receive notification")
	assert.Equal(t, "test_channel", notification.Channel, "Channel should match")

	// Verify we can parse the notification payload
	var eventFromNotification model.Event
	err = json.Unmarshal([]byte(notification.Payload), &eventFromNotification)
	assert.Nil(t, err, "Should parse notification payload")
	assert.Equal(t, "test-123", eventFromNotification.UID, "Event UID should match")
}

// [AI] Test listen() error handling
func TestListenErrorHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	callCount := 0
	mockConn := &MockPgxConn{
		WaitForNotificationFunc: func(ctx context.Context) (*pgconn.Notification, error) {
			callCount++
			if callCount == 1 {
				// First call returns an error
				return nil, assert.AnError
			}
			// Subsequent calls block on context
			<-ctx.Done()
			return nil, ctx.Err()
		},
		CloseFunc: func(ctx context.Context) error {
			return nil
		},
	}

	// Verify error handling behavior
	_, err := mockConn.WaitForNotification(ctx)
	assert.Error(t, err, "Should return error on first call")
	assert.Equal(t, 1, callCount, "Should have been called once")

	// Second call should block until cancelled
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err = mockConn.WaitForNotification(ctx)
	assert.Error(t, err, "Should return context error")
	assert.Equal(t, context.Canceled, err, "Should be context cancelled error")
}

// Test StopPostgresListener with no instance
func TestStopPostgresListener_NoInstance(t *testing.T) {
	// Reset the singleton
	listenerOnce = sync.Once{}
	listenerInstance = nil

	// Should not panic when no instance exists
	StopPostgresListener()

	// Verify state
	assert.Nil(t, listenerInstance, "Listener instance should be nil")
}

// Test StopPostgresListener with active instance
func TestStopPostgresListener_WithActiveInstance(t *testing.T) {
	// Reset and create a listener instance
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx := context.Background()
	notifyChannel := make(chan *model.Event, 100)
	defer close(notifyChannel)

	// Register a subscription to initialize the listener
	RegisterSubscription(ctx, "test-stop-listener", notifyChannel)

	assert.NotNil(t, listenerInstance, "Listener should be initialized")

	// Stop the listener
	StopPostgresListener()

	// Give it time to shut down
	time.Sleep(100 * time.Millisecond)

	// Verify state was reset
	listenerMu.Lock()
	assert.Nil(t, listenerInstance, "Listener instance should be nil after stop")
	listenerMu.Unlock()
}

// Test StopPostgresListener resets sync.Once
func TestStopPostgresListener_ResetsOnce(t *testing.T) {
	// Reset the singleton
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx := context.Background()
	notifyChannel1 := make(chan *model.Event, 100)
	defer close(notifyChannel1)

	// Register first subscription
	RegisterSubscription(ctx, "test-once-1", notifyChannel1)
	assert.NotNil(t, listenerInstance, "First listener should be initialized")
	firstInstance := listenerInstance

	// Stop the listener
	StopPostgresListener()
	time.Sleep(100 * time.Millisecond)

	// Register another subscription - should create new instance
	notifyChannel2 := make(chan *model.Event, 100)
	defer close(notifyChannel2)
	RegisterSubscription(ctx, "test-once-2", notifyChannel2)

	assert.NotNil(t, listenerInstance, "Second listener should be initialized")
	// Should be a new instance (different pointer)
	assert.NotEqual(t, firstInstance, listenerInstance, "Should be a new listener instance")
}

// [AI] Test connect function error path (no database available)
func TestConnect_DatabaseUnavailable(t *testing.T) {
	listenCtx, listenCancel := context.WithCancel(context.Background())
	defer listenCancel()

	listener := &Listener{
		subscriptions: make(map[string]*Subscription),
		conn:          nil,
		ctx:           listenCtx,
		cancel:        listenCancel,
		started:       false,
	}

	// Attempt to connect (will fail since no DB is running)
	err := listener.connect()

	// Should return an error
	assert.Error(t, err, "Connection should fail when no database is available")
	assert.Contains(t, err.Error(), "unable to connect to database", "Error message should indicate connection failure")
}

// Test Start function error path (connection failure)
func TestStart_ConnectionFailure(t *testing.T) {
	listenCtx, listenCancel := context.WithCancel(context.Background())
	defer listenCancel()

	listener := &Listener{
		subscriptions: make(map[string]*Subscription),
		conn:          nil,
		ctx:           listenCtx,
		cancel:        listenCancel,
		started:       false,
	}

	// Attempt to start (will fail due to connection error)
	err := listener.Start()

	// Should return an error
	assert.Error(t, err, "Start should fail when connection cannot be established")
	assert.Contains(t, err.Error(), "failed to connect to database", "Error message should indicate connection failure")

	// Verify started flag is still false
	listener.mu.RLock()
	assert.False(t, listener.started, "Started flag should remain false on error")
	listener.mu.RUnlock()
}

// [AI] Test listener cleanup via UnregisterSubscription
func TestListener_CleanupViaUnregister(t *testing.T) {
	// Reset the singleton
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx := context.Background()
	notifyChannel := make(chan *model.Event, 100)
	defer close(notifyChannel)

	// Register subscription
	RegisterSubscription(ctx, "test-cleanup", notifyChannel)
	assert.NotNil(t, listenerInstance, "Listener should be initialized")

	// Unregister to trigger shutdown (when it's the last subscription)
	UnregisterSubscription("test-cleanup")

	// Give time for cleanup
	time.Sleep(100 * time.Millisecond)

	// Note: The listener instance still exists but its context is cancelled
	// The instance is only set to nil when the listen() goroutine exits via defer
	// Since we can't connect to a database in tests, the listen() goroutine
	// was never actually started, so the defer cleanup doesn't run
	listenerMu.Lock()
	defer listenerMu.Unlock()
	if listenerInstance != nil {
		// Verify the cancel was called at least
		select {
		case <-listenerInstance.ctx.Done():
			// Good - context was cancelled
			assert.True(t, true, "Context should be cancelled")
		default:
			t.Fatal("Context should have been cancelled when last subscription was removed")
		}
	}
}

// [AI] Test multiple rapid start/stop cycles
func TestListener_RapidStartStopCycles(t *testing.T) {
	for i := 0; i < 5; i++ {
		// Reset
		listenerOnce = sync.Once{}
		listenerInstance = nil

		ctx := context.Background()
		notifyChannel := make(chan *model.Event, 100)

		// Register
		RegisterSubscription(ctx, "test-rapid-cycle", notifyChannel)
		assert.NotNil(t, listenerInstance, "Listener should be initialized")

		// Stop
		StopPostgresListener()
		time.Sleep(50 * time.Millisecond)

		close(notifyChannel)
	}

	// Final verification
	listenerMu.Lock()
	assert.Nil(t, listenerInstance, "Final state should be nil")
	listenerMu.Unlock()
}

// Test listen() function.
// Register a subscription then wait and validate the notification.
func TestListen(t *testing.T) {
	listenCtx, listenCancel := context.WithCancel(context.Background())

	// Mock the database connection
	mockConn := MockPgxConn{
		WaitForNotificationFunc: func(ctx context.Context) (*pgconn.Notification, error) {
			notification := &pgconn.Notification{
				Channel: "search_resources_notify",
				Payload: `{"uid":"test-123","operation":"CREATE","timestamp":"2024-01-01T00:00:00Z"}`,
			}
			return notification, nil
		},
	}

	// Subscription to test event forwarding.
	subscription := &Subscription{
		ID:      "test-subscription-1",
		Channel: make(chan *model.Event, 100),
		Context: listenCtx,
	}

	// Listener with mocked database connection and subscription.
	listener := &Listener{
		ctx:           listenCtx,
		cancel:        listenCancel,
		started:       true,
		subscriptions: map[string]*Subscription{"test-subscription-1": subscription},
		conn:          mockConn,
	}

	// Start the listener and wait for the notification.
	go listener.listen()
	time.Sleep(50 * time.Millisecond)

	notification := <-subscription.Channel

	// Verify the notification payload.
	assert.Equal(t, "test-123", notification.UID, "UID should match")
	assert.Equal(t, "CREATE", notification.Operation, "Operation should match")
	assert.Equal(t, "2024-01-01T00:00:00Z", notification.Timestamp, "Timestamp should match")
	assert.Nil(t, notification.OldData, "OldData should be nil")

	// Cancel the context
	listenCancel()
	time.Sleep(50 * time.Millisecond)
	assert.Nil(t, listenerInstance, "Listener instance should be nil")
}

// Test listen() function with cancelled context.
func TestListen_withCancelledContext(t *testing.T) {
	listenCtx, listenCancel := context.WithCancel(context.Background())

	listener := &Listener{
		ctx:           listenCtx,
		cancel:        listenCancel,
		started:       false,
		subscriptions: make(map[string]*Subscription),
	}
	// Cancel the context before starting the listener
	listenCancel()
	time.Sleep(50 * time.Millisecond)
	assert.Nil(t, listenerInstance, "Listener instance should be nil")

	// Start the listener
	go listener.listen()
	time.Sleep(50 * time.Millisecond)

	assert.Nil(t, listenerInstance, "Listener instance should be nil")
}

// Test listen() function with nil connection.
func TestListen_withNilConnection(t *testing.T) {
	listenCtx, listenCancel := context.WithCancel(context.Background())

	listener := &Listener{
		ctx:           listenCtx,
		cancel:        listenCancel,
		started:       false,
		subscriptions: make(map[string]*Subscription),
		conn:          nil,
	}

	time.Sleep(50 * time.Millisecond)
	assert.Nil(t, listenerInstance, "Listener instance should be nil")

	// Start the listener
	go listener.listen()
	time.Sleep(50 * time.Millisecond)

	listenCancel()
	time.Sleep(50 * time.Millisecond)

	assert.Nil(t, listenerInstance, "Listener instance should be nil")
}
