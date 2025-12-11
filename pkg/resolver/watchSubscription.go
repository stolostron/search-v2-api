// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	klog "k8s.io/klog/v2"

	"github.com/google/uuid"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
)

// eventMatchesFilters checks if an event matches the search input filters.
// Returns true if the event should be sent to the client, false otherwise.
func eventMatchesFilters(event *model.Event, input *model.SearchInput) bool {
	// If no filters are specified, send all events
	if input == nil {
		return true
	}

	// Get the event data to check (use NewData for INSERT/UPDATE, OldData for DELETE)
	eventData := event.NewData
	if eventData == nil {
		eventData = event.OldData
	}

	// If no data to check, skip the event
	if eventData == nil {
		return false
	}

	// Check keyword filters (AND operation - all keywords must match)
	if len(input.Keywords) > 0 {
		for _, keyword := range input.Keywords {
			if keyword == nil {
				continue
			}
			keywordLower := strings.ToLower(*keyword)
			found := false

			// Search for keyword in any field value
			for _, value := range eventData {
				if strValue, ok := value.(string); ok {
					if strings.Contains(strings.ToLower(strValue), keywordLower) {
						found = true
						break
					}
				}
			}

			// If keyword not found in any field, event doesn't match
			if !found {
				return false
			}
		}
	}

	// Check property filters (AND operation - all filters must match)
	if len(input.Filters) > 0 {
		for _, filter := range input.Filters {
			if filter == nil || filter.Property == "" {
				continue
			}

			property := filter.Property
			propertyValue, exists := eventData[property]

			// If property doesn't exist in event data, filter doesn't match
			if !exists {
				return false
			}

			// If filter has no values, it's invalid - reject the event
			if len(filter.Values) == 0 {
				return false
			}

			// Convert property value to string for comparison
			propertyValueStr := ""
			if strValue, ok := propertyValue.(string); ok {
				propertyValueStr = strValue
			} else {
				// Try to convert other types to string
				propertyValueStr = strings.ToLower(fmt.Sprintf("%v", propertyValue))
			}

			// Check if property value matches any of the filter values (OR operation)
			matched := false
			for _, filterValue := range filter.Values {
				if filterValue == nil {
					continue
				}

				if propertyValueStr == *filterValue {
					matched = true
					break
				}
			}

			// If none of the filter values matched, event doesn't match
			if !matched {
				return false
			}
		}
	}

	// All filters matched
	return true
}

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

	// Get WebSocket connection ID from the context. If not found, generate a new one.
	subID, ok := ctx.Value(config.WSContextKeyConnectionID).(string)
	if !ok {
		subID = uuid.New().String()[:8]
		klog.Errorf("Failed to get WebSocket connection ID from context. Generating a new one: %s", subID)
	}

	go func() {
		database.RegisterSubscription(ctx, subID, receiver)

		defer func() {
			klog.V(2).Infof("Closed subscription watch(%s).", subID)
			database.UnregisterSubscription(subID)
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
				// Filter event based on the input filters
				if !eventMatchesFilters(event, input) {
					klog.V(4).Infof("Subscription watch(%s) event did not match filters (UID: %s, Operation: %s)", subID, event.UID, event.Operation)
					continue
				}

				// TODO: Filter events for user's RBAC permissions. ACM-26248

				// Send filtered event to client
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
