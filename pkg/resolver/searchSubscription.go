// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"errors"
	"time"

	klog "k8s.io/klog/v2"

	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
)

func SearchSubscription(ctx context.Context, input []*model.SearchInput) (<-chan []*SearchResult, error) {
	ch := make(chan []*SearchResult)

	// if not enabled via feature flag -> return error message
	if !config.Cfg.Features.SubscriptionEnabled {
		klog.Infof("GraphQL subscription requests are disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
		ctx.Done()
		close(ch)
		return ch, errors.New("GraphQL subscription requests are disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
	}

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