// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"errors"

	klog "k8s.io/klog/v2"

	"github.com/google/uuid"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
)

// WatchSubscriptions implements the GraphQL watch subscription resolver.
func WatchSubscription(ctx context.Context, input *model.SearchInput) (<-chan *model.Event, error) {
	result := make(chan *model.Event)   // Channel to send events to the client.
	receiver := make(chan *model.Event) // Channel to receive events from the database.

	// Check if the feature flag is enabled. If not, return an error.
	if !config.Cfg.Features.SubscriptionEnabled {
		klog.Info("GraphQL subscription feature is disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
		ctx.Done()
		return result, errors.New("GraphQL subscription feature is disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
	}

	go func() {
		// Get WebSocket connection ID from the context
		subID, ok := ctx.Value("ws-connection-id").(string)
		if !ok {
			// FIXME: Should get the subscription ID from the context.
			subID = uuid.New().String()[:8]
			klog.Errorf("FIXME:Failed to get WebSocket connection ID from context. Generating a new one: %s", subID)
		}

		database.RegisterSubscription(ctx, subID, receiver)
		defer database.UnregisterSubscription(subID)

		defer func() {
			klog.V(2).Infof("Closed subscription watch(%s).", subID)
			close(result)
			close(receiver)
		}()

		// Receive events from the database (receiver), filter, and send to the client (result).
		for {
			select {
			case <-ctx.Done():
				klog.V(3).Infof("Subscription watch(%s) closed by client.", subID)
				return
			case event, ok := <-receiver:
				// If the receiver channel is closed, return.
				if !ok {
					klog.V(3).Infof("Subscription watch(%s) channel closed.", subID)
					return
				}
				// Filter event and send to client.
				// TODO ======================================================
				//   1. Filter events based on the input filter. ACM-24574
				//   2. Filter events for user's RBAC permissions. ACM-26248
				// TODO ======================================================
				select {
				case result <- event:
					klog.V(3).Infof("Subscription watch(%s) sent event (UID: %s, Operation: %s) to client", subID, event.UID, event.Operation)
					continue
				case <-ctx.Done():
					klog.V(3).Infof("Subscription watch(%s) closed while sending event.", subID)
					return
				default:
					klog.V(3).Infof("Subscription watch(%s) channel buffer is full, dropping event.", subID)
					return
				}
			}
		}
	}()

	return result, nil
}
