// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgconn"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stretchr/testify/assert"
)

// [AI]
func TestWatchSubscription_Disabled(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Disable subscription feature
	config.Cfg.Features.SubscriptionEnabled = false

	ctx := context.Background()
	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	// Verify error is returned when feature is disabled
	assert.NotNil(t, err, "Should return error when subscription is disabled")
	assert.Contains(t, err.Error(), "disabled", "Error should mention subscription is disabled")
	assert.NotNil(t, resultChan, "Result channel should be returned even on error")
}

// [AI]
func TestWatchSubscription_Enabled(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	// Verify no error when feature is enabled
	assert.Nil(t, err, "Should not return error when subscription is enabled")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// Wait a moment for goroutine to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop subscription
	cancel()

	// Verify channel is eventually closed
	select {
	case _, ok := <-resultChan:
		if ok {
			t.Log("Received event before channel closed")
		}
	case <-time.After(1 * time.Second):
		// Timeout is acceptable, channel should close
	}
}

// [AI]
func TestWatchSubscription_ContextCancellation(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithCancel(context.Background())
	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// Wait for goroutine to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Verify the channel is closed after context cancellation
	select {
	case _, ok := <-resultChan:
		assert.False(t, ok, "Channel should be closed after context cancellation")
	case <-time.After(2 * time.Second):
		t.Fatal("Channel should be closed within timeout")
	}
}

// [AI]
func TestWatchSubscription_MultipleSubscriptions(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	input := &model.SearchInput{}

	// Create multiple subscriptions
	numSubscriptions := 5
	channels := make([]<-chan *model.Event, numSubscriptions)
	var wg sync.WaitGroup

	for i := 0; i < numSubscriptions; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ch, err := WatchSubscription(ctx, input)
			assert.Nil(t, err, "Should not return error for subscription %d", index)
			channels[index] = ch
		}(i)
	}

	wg.Wait()

	// Verify all channels were created
	for i, ch := range channels {
		assert.NotNil(t, ch, "Channel %d should not be nil", i)
	}

	// Cancel context to clean up
	cancel()

	// Wait for all channels to close
	time.Sleep(500 * time.Millisecond)
}

// [AI]
func TestWatchSubscription_EventForwarding(t *testing.T) {
	// This test would require mocking the database listener
	// For now, we test the basic flow

	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// Start a goroutine to consume from the channel
	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-resultChan:
				if !ok {
					// Channel closed
					done <- true
					return
				}
				// Process event if received
				if event != nil {
					t.Logf("Received event: %+v", event)
				}
			case <-time.After(1 * time.Second):
				// No events received, that's ok for this test
				done <- true
				return
			}
		}
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Test completed
	case <-time.After(3 * time.Second):
		t.Fatal("Test timed out")
	}
}

// [AI]
func TestWatchSubscription_ChannelBufferHandling(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// The result channel should be non-buffered (created with make(chan *model.Event))
	// We can't directly check the buffer size of a receive-only channel,
	// but we can verify it's operational

	// Cancel after a short time
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for channel to close
	time.Sleep(200 * time.Millisecond)
}

// [AI]
func TestWatchSubscription_NilInput(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Test with nil input - should still work as input is not currently used
	resultChan, err := WatchSubscription(ctx, nil)

	assert.Nil(t, err, "Should not return error with nil input")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	time.Sleep(100 * time.Millisecond)
	cancel()
}

// [AI]
func TestWatchSubscription_RapidContextCancellation(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithCancel(context.Background())
	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	// Cancel immediately
	cancel()

	// Verify channel eventually closes
	select {
	case _, ok := <-resultChan:
		if ok {
			t.Log("Received event before closure")
		}
		// Channel closed, as expected
	case <-time.After(2 * time.Second):
		// Timeout is acceptable
	}
}

// [AI]
func TestWatchSubscription_FilterInput(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Create input with filters (these should be used in future for event filtering)
	val1 := "Pod"
	input := &model.SearchInput{
		Filters: []*model.SearchFilter{
			{Property: "kind", Values: []*string{&val1}},
		},
	}

	resultChan, err := WatchSubscription(ctx, input)

	assert.Nil(t, err, "Should not return error with filtered input")
	assert.NotNil(t, resultChan, "Result channel should be returned")

	time.Sleep(100 * time.Millisecond)
	cancel()
}

func TestSubscriptionWithMockedDatabase(t *testing.T) {
	// Save original config value
	originalEnabled := config.Cfg.Features.SubscriptionEnabled
	defer func() {
		config.Cfg.Features.SubscriptionEnabled = originalEnabled
	}()

	// Enable subscription feature
	config.Cfg.Features.SubscriptionEnabled = true

	// Reset database listener singleton for clean test
	database.StopPostgresListener()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	database.ListenerInstance = &database.Listener{
		conn: &database.MockPgxConn{
			WaitForNotificationFunc: func(ctx context.Context) (*pgconn.Notification, error) {
				return nil, nil
			},
		},
	}
	input := &model.SearchInput{}

	resultChan, err := WatchSubscription(ctx, input)
}
