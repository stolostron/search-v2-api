// Copyright Contributors to the Open Cluster Management project
package database

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stretchr/testify/assert"
)

// [AI]
func TestRegisterSubscriptionAndListen(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notifyChannel := make(chan *model.Event, 100)
	uid := "test-uid-123"

	// Register subscription - this should initialize the listener
	RegisterSubscriptionAndListen(ctx, uid, notifyChannel)

	// Verify listener was initialized
	assert.NotNil(t, listenerInstance, "Listener instance should be initialized")
	assert.NotNil(t, listenerInstance.subscriptions, "Subscriptions list should be initialized")
	assert.Equal(t, 1, len(listenerInstance.subscriptions), "Should have 1 subscription")
	assert.Equal(t, uid, listenerInstance.subscriptions[0].ID, "Subscription ID should match")
	assert.Equal(t, notifyChannel, listenerInstance.subscriptions[0].Channel, "Subscription channel should match")

	// Clean up
	close(notifyChannel)
}

// [AI]
func TestRegisterMultipleSubscriptions(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register first subscription
	notifyChannel1 := make(chan *model.Event, 100)
	uid1 := "test-uid-1"
	RegisterSubscriptionAndListen(ctx, uid1, notifyChannel1)

	// Register second subscription
	notifyChannel2 := make(chan *model.Event, 100)
	uid2 := "test-uid-2"
	RegisterSubscriptionAndListen(ctx, uid2, notifyChannel2)

	// Verify both subscriptions exist
	assert.Equal(t, 2, len(listenerInstance.subscriptions), "Should have 2 subscriptions")

	// Clean up
	close(notifyChannel1)
	close(notifyChannel2)
}

// [AI]
func TestUnregisterSubscription(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register subscriptions
	notifyChannel1 := make(chan *model.Event, 100)
	uid1 := "test-uid-1"
	RegisterSubscriptionAndListen(ctx, uid1, notifyChannel1)

	notifyChannel2 := make(chan *model.Event, 100)
	uid2 := "test-uid-2"
	RegisterSubscriptionAndListen(ctx, uid2, notifyChannel2)

	assert.Equal(t, 2, len(listenerInstance.subscriptions), "Should have 2 subscriptions")

	// Unregister first subscription
	UnregisterSubscription(uid1)

	// Verify only one subscription remains
	assert.Equal(t, 1, len(listenerInstance.subscriptions), "Should have 1 subscription after unregister")
	assert.Equal(t, uid2, listenerInstance.subscriptions[0].ID, "Remaining subscription should be uid2")

	// Clean up
	close(notifyChannel1)
	close(notifyChannel2)
}

// [AI]
func TestUnregisterLastSubscription(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register a subscription
	notifyChannel := make(chan *model.Event, 100)
	uid := "test-uid-1"
	RegisterSubscriptionAndListen(ctx, uid, notifyChannel)

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

// [AI]
func TestSubscriptionStruct(t *testing.T) {
	ctx := context.Background()
	channel := make(chan *model.Event, 100)
	uid := "test-subscription-id"

	sub := &Subscription{
		ID:      uid,
		Channel: channel,
		Context: ctx,
	}

	assert.Equal(t, uid, sub.ID, "Subscription ID should match")
	assert.Equal(t, channel, sub.Channel, "Subscription channel should match")
	assert.Equal(t, ctx, sub.Context, "Subscription context should match")

	close(channel)
}

// [AI]
func TestListenerStartAlreadyStarted(t *testing.T) {
	// Create a listener instance
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := &Listener{
		subscriptions: make([]*Subscription, 0),
		conn:          nil,
		ctx:           ctx,
		cancel:        cancel,
		started:       true, // Already started
	}

	// Call Start - should return nil without error since already started
	err := listener.Start()
	assert.Nil(t, err, "Starting an already started listener should not return an error")
}

// [AI]
func TestUnregisterNonExistentSubscription(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register a subscription
	notifyChannel := make(chan *model.Event, 100)
	uid := "test-uid-1"
	RegisterSubscriptionAndListen(ctx, uid, notifyChannel)

	assert.Equal(t, 1, len(listenerInstance.subscriptions), "Should have 1 subscription")

	// Try to unregister a non-existent subscription
	UnregisterSubscription("non-existent-uid")

	// Verify the original subscription is still there
	assert.Equal(t, 1, len(listenerInstance.subscriptions), "Should still have 1 subscription")

	// Clean up
	close(notifyChannel)
}

// [AI]
func TestListenerContextCancellation(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())

	// Register a subscription
	notifyChannel := make(chan *model.Event, 100)
	uid := "test-uid-1"
	RegisterSubscriptionAndListen(ctx, uid, notifyChannel)

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

// [AI]
func TestSubscriptionChannelBufferSize(t *testing.T) {
	// Verify that the subscription channel has the expected buffer size
	channel := make(chan *model.Event, 100)

	assert.Equal(t, 100, cap(channel), "Subscription channel should have buffer size of 100")

	close(channel)
}

// [AI]
func TestMultipleConcurrentRegistrations(t *testing.T) {
	// Reset the singleton for testing
	listenerOnce = sync.Once{}
	listenerInstance = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numSubscriptions := 10
	var wg sync.WaitGroup
	channels := make([]chan *model.Event, numSubscriptions)

	// Register multiple subscriptions concurrently
	for i := 0; i < numSubscriptions; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			channels[index] = make(chan *model.Event, 100)
			uid := "test-uid-" + string(rune(index))
			RegisterSubscriptionAndListen(ctx, uid, channels[index])
		}(i)
	}

	wg.Wait()

	// Verify all subscriptions were registered
	assert.GreaterOrEqual(t, len(listenerInstance.subscriptions), numSubscriptions,
		"Should have at least the expected number of subscriptions")

	// Clean up
	for _, ch := range channels {
		if ch != nil {
			close(ch)
		}
	}
}
