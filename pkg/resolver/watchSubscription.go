// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"errors"
	"strconv"
	"time"

	klog "k8s.io/klog/v2"

	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
)

func WatchSubscription(ctx context.Context, input *model.SearchInput) (<-chan *model.Event, error) {
	ch := make(chan *model.Event)

	// if not enabled via feature flag -> return error message
	if !config.Cfg.Features.SubscriptionEnabled {
		klog.Infof("GraphQL subscription requests are disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
		ctx.Done()
		close(ch)
		return ch, errors.New("GraphQL subscription requests are disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
	}

	// TODO: Get event from database.
	go func() {
		i := 0
		for {
			i++
			select {
			case <-ctx.Done():
				klog.V(3).Info("Watch subscription closed.")
				return
			default:
				klog.V(1).Info("Sending mock event for watch subscription")
				// TODO: Get event from database
				// Sending mock event for now

				ch <- &model.Event{
					UID:       "0000-" + strconv.Itoa(i),
					Operation: "CREATE",
					Data: map[string]interface{}{
						"name":       "test-" + strconv.Itoa(i),
						"namespace":  "default",
						"kind":       "MockedKind",
						"apiVersion": "v1",
						"apiGroup":   "mock.io",
					},
					Timestamp: time.Now().Format(time.RFC3339),
				}

				if i%5 == 0 {
					ch <- &model.Event{
						UID:       "0000-" + strconv.Itoa(i-1),
						Operation: "DELETE",
						Data: map[string]interface{}{
							"name":       "test-" + strconv.Itoa(i-1),
							"namespace":  "default",
							"kind":       "MockedKind",
							"apiVersion": "v1",
							"apiGroup":   "mock.io",
						},
						Timestamp: time.Now().Format(time.RFC3339),
					}
				}
			}
			time.Sleep(time.Duration(1000 * time.Millisecond))
		}
	}()

	return ch, nil
}
