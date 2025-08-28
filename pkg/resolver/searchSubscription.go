// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"errors"
	"fmt"
	"time"

	klog "k8s.io/klog/v2"

	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/notification"
)

func SearchSubscription(ctx context.Context, input []*model.SearchInput) (<-chan []*SearchResult, error) {
	ch := make(chan []*SearchResult)

	// Check if subscriptions are enabled via feature flag
	if !config.Cfg.Features.SubscriptionEnabled {
		klog.Infof("GraphQL subscription requests are disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
		ctx.Done()
		close(ch)
		return ch, errors.New("GraphQL subscription requests are disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
	}

	// Determine subscription method: real-time vs polling
	useRealTimeNotifications := config.Cfg.Features.NotificationEnabled

	if useRealTimeNotifications {
		klog.V(3).Info("Using real-time PostgreSQL notifications for search subscription")
		return realTimeSearchSubscription(ctx, input, ch)
	} else {
		klog.V(3).Info("Using polling-based search subscription")
		return pollingSearchSubscription(ctx, input, ch)
	}
}

// pollingSearchSubscription implements the original polling-based subscription
func pollingSearchSubscription(ctx context.Context, input []*model.SearchInput, ch chan []*SearchResult) (<-chan []*SearchResult, error) {
	// You can (and probably should) handle your channels in a central place outside of `schema.resolvers.go`.
	// For this example we'll simply use a Goroutine with a simple loop.
	go func() {
		timeout := time.After(time.Duration(config.Cfg.SubscriptionRefreshTimeout) * time.Millisecond)
		// Handle deregistration of the channel here. Note the `defer`
		defer close(ch)

		for {
			klog.V(3).Info("Search subscription new poll interval")
			searchResult, err := Search(ctx, input)

			if err != nil {
				klog.Errorf("Error occurred during the search subscription request: %s", err)
			}

			// The subscription may have been closed due to the client disconnecting.
			// Hence we do send in a select block with a check for context cancellation.
			// This avoids goroutine getting blocked forever or panicking,
			select {
			case <-ctx.Done(): // This runs when context gets cancelled. Subscription closes.
				klog.V(3).Info("Search subscription Closed")
				return // Remember to return to end the routine.

			case <-timeout: // This runs when timeoout is hit. Subscription closes.
				klog.V(3).Info("Subscription timeout reached. Closing connection.")
				return // Remember to return to end the routine.

			case ch <- searchResult: // This is the actual send.
				// Our message went through, do nothing
			}

			// Wait SubscriptionRefreshInterval seconds for next search reuslt send.
			time.Sleep(time.Duration(config.Cfg.SubscriptionRefreshInterval) * time.Millisecond)
		}
	}()

	// We return the channel and no error.
	return ch, nil
}

// realTimeSearchSubscription implements PostgreSQL LISTEN/NOTIFY based subscription
func realTimeSearchSubscription(ctx context.Context, input []*model.SearchInput, ch chan []*SearchResult) (<-chan []*SearchResult, error) {
	manager := notification.GetNotificationManager()

	// Create subscription filter from search input
	var notificationFilter notification.NotificationFilter
	var err error

	if len(input) > 0 {
		// Use the first SearchInput to create the filter
		notificationFilter, err = notification.CreateSubscriptionFilter(ctx, input[0])
		if err != nil {
			klog.Errorf("Failed to create notification filter: %v", err)
			// Fall back to polling if filter creation fails
			return pollingSearchSubscription(ctx, input, ch)
		}
	} else {
		// Create a default filter for all changes
		notificationFilter = notification.NewFilterBuilder().
			WithOperations("INSERT", "UPDATE", "DELETE").
			Build()
	}

	// Generate unique subscription ID
	subscriptionID := fmt.Sprintf("search-subscription-%d", time.Now().UnixNano())

	klog.V(2).Infof("Creating real-time search subscription %s with filter: %s",
		subscriptionID, notification.FilterString(notificationFilter))

	// Subscribe to notifications
	subscription, err := manager.Subscribe(subscriptionID, notificationFilter, nil)
	if err != nil {
		klog.Errorf("Failed to create notification subscription: %v", err)
		// Fall back to polling if subscription fails
		return pollingSearchSubscription(ctx, input, ch)
	}

	go func() {
		defer func() {
			close(ch)
			// Clean up subscription when done
			if err := manager.Unsubscribe(subscriptionID); err != nil {
				klog.Errorf("Failed to unsubscribe from notifications: %v", err)
			}
			klog.V(2).Infof("Cleaned up real-time search subscription %s", subscriptionID)
		}()

		timeout := time.After(time.Duration(config.Cfg.SubscriptionRefreshTimeout) * time.Millisecond)

		// Send initial search results
		klog.V(3).Info("Sending initial search results for real-time subscription")
		searchResult, err := Search(ctx, input)
		if err != nil {
			klog.Errorf("Error in initial search for real-time subscription: %s", err)
		} else {
			select {
			case <-ctx.Done():
				return
			case <-timeout:
				return
			case ch <- searchResult:
				// Initial results sent
			}
		}

		eventCount := 0
		for {
			select {
			case <-ctx.Done():
				klog.V(3).Info("Real-time search subscription context cancelled")
				return

			case <-timeout:
				klog.V(3).Info("Real-time search subscription timeout reached")
				return

			case notificationPayload, ok := <-subscription.Channel:
				if !ok {
					klog.V(3).Info("Real-time search subscription notification channel closed")
					return
				}

				klog.V(4).Infof("Received notification for: %s Action:[%s] kind:[%s] name:[%s] namespace:[%s] cluster:[%s]",
					subscriptionID, notificationPayload.Operation, notificationPayload.NewData["kind"],
					notificationPayload.NewData["name"], notificationPayload.NewData["namespace"], notificationPayload.Cluster)

				// Send updated results
				select {
				case <-ctx.Done():
					return
				case <-timeout:
					return
				case ch <- searchResult:
					klog.V(4).Infof("Sent updated search results for subscription %s", subscriptionID)
				default:
					eventCount++
					klog.V(4).Infof("Received %d events for subscription %s", eventCount, subscriptionID)
					continue
				}
			}
		}
	}()

	return ch, nil
}
