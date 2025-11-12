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

func WatchSubscription(ctx context.Context, input *model.SearchInput) (<-chan *model.Event, error) {
	receive := make(chan *model.Event)
	result := make(chan *model.Event)

	// if not enabled via feature flag -> return error message
	if !config.Cfg.Features.SubscriptionEnabled {
		klog.Infof("GraphQL subscription feature is disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
		ctx.Done()
		return result, errors.New("GraphQL subscription feature is disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
	}

	go func() {
		uid := uuid.New().String()[:8] // Random UID for logging purposes.
		database.RegisterSubscriptionAndListen(ctx, uid, receive)
		defer database.UnregisterSubscription(uid)

		defer func() {
			klog.V(3).Info("Closing result and receive channels.")
			close(result)
			close(receive)
		}()

		// Forward events from the subscription channel to the client channel
		for {
			select {
			case <-ctx.Done():
				klog.V(3).Infof("Subscription watch(%s) closed by client.", uid)
				return
			case event, ok := <-receive:
				if !ok {
					// Subscription channel was closed.
					klog.V(3).Infof("Subscription watch(%s) channel closed.", uid)
					return
				}
				// Filter and send event to client
				// TODO ======================================================
				//   1. Filter events based on the input filter. ACM-24574
				//   2. Filter events for user's RBAC permissions. ACM-26248
				// TODO ======================================================
				select {
				case result <- event:
					klog.V(3).Infof("Subscription watch(%s) sent event (UID: %s, Operation: %s) to client", uid, event.UID, event.Operation)
					continue
				case <-ctx.Done():
					klog.V(3).Infof("Subscription watch(%s) closed while sending event.", uid)
					return
				default:
					klog.V(3).Infof("Subscription watch(%s) channel buffer is full, dropping event.", uid)
					return
				}
			}
		}
	}()

	return result, nil
}
