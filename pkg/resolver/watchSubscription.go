// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"errors"
	"fmt"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"strings"

	klog "k8s.io/klog/v2"

	"github.com/google/uuid"
	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/database"
)

// matchAnyLabel returns true if any of the label filters matches the event labels.
// Equivalent to an OR operation.
func matchAnyLabel(eventLabels map[string]interface{}, labelFilters []*string) bool {
	for _, labelFilter := range labelFilters {
		keyValue := strings.Split(*labelFilter, "=")
		// Filter validated before as containing key=value pairs.
		labelKey := keyValue[0]
		labelValue := keyValue[1]
		if value, exists := eventLabels[labelKey]; exists && value == labelValue {
			return true
		}
	}
	return false
}

// eventMatchesAllFilters Returns true if the event matches all the search input filters.
// Equivalent to an AND operation.
func eventMatchesAllFilters(event *model.Event, input *model.SearchInput) bool {
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
		klog.Warningf("Event data is nil for event (UID: %s, Operation: %s)", event.UID, event.Operation)
		return false
	}

	// Check keywords (AND operation - all keywords must match)
	for _, keyword := range input.Keywords {
		if keyword == nil {
			continue
		}
		keywordLower := strings.ToLower(*keyword)
		found := false

		// Search for keyword in any field value
		strValue := ""
		for _, value := range eventData {
			if v, ok := value.(string); ok {
				strValue = v
			} else {
				strValue = fmt.Sprintf("%v", value)
			}
			if strings.Contains(strings.ToLower(strValue), keywordLower) {
				found = true
				break
			}
		}

		// If keyword not found in any field, event doesn't match
		if !found {
			return false
		}
	}

	// Check property filters (AND operation - all filters must match)
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

		// Check if label filter matches
		if property == "label" {
			if !matchAnyLabel(propertyValue.(map[string]interface{}), filter.Values) {
				return false
			}
			continue
		}

		// Convert property value to string for comparison
		propertyValueStr := ""
		if strValue, ok := propertyValue.(string); ok {
			propertyValueStr = strValue
		} else {
			// Try to convert other types to string
			propertyValueStr = fmt.Sprintf("%v", propertyValue)
		}

		// Check if property value matches any of the filter values (OR operation)
		matched := false
		for _, filterValue := range filter.Values {
			if filterValue == nil {
				continue
			}
			// Special case: Kind is compared case-insensitive to match the search behavior.
			if property == "kind" {
				if strings.EqualFold(propertyValueStr, *filterValue) {
					matched = true
					break
				}
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
	// All filters matched
	return true
}

func getEventDataFields(eventData map[string]any) (string, string, string, string, bool) {
	var eventNamespace, eventKind, eventApigroup, eventCluster string
	eventIsHubClusterResource := false
	if _, ok := eventData["namespace"]; ok {
		eventNamespace = eventData["namespace"].(string)
	}
	if _, ok := eventData["kind_plural"]; ok {
		eventKind = eventData["kind_plural"].(string)
	}
	if _, ok := eventData["apigroup"]; ok {
		eventApigroup = eventData["apigroup"].(string)
	}
	if _, ok := eventData["cluster"]; ok {
		eventCluster = eventData["cluster"].(string)
	}
	if _, ok := eventData["_hubClusterResource"]; ok {
		eventIsHubClusterResource = true
	}
	return eventNamespace, eventKind, eventApigroup, eventCluster, eventIsHubClusterResource
}

// eventMatchesRbac validates that user has permission to see event
func eventMatchesRbac(ctx context.Context, event *model.Event) bool {
	eventData := event.NewData
	if eventData == nil {
		eventData = event.OldData
	}

	// if no data to check, skip the event
	if eventData == nil {
		klog.Warningf("Event data is nil for event (UID: %s, Operation: %s)", event.UID, event.Operation)
		return false
	}

	eventNamespace, eventKind, eventApigroup, eventCluster, eventIsHubClusterResource := getEventDataFields(eventData)

	if eventIsHubClusterResource {
		return checkAndCache(ctx, eventApigroup, eventKind, eventNamespace, eventCluster, eventIsHubClusterResource) // "oc auth can-i watch <eventApigroup>.<eventKind> [-n <eventNamespace>] --as=<user>"
	} else if config.Cfg.Features.FineGrainedRbac {
		return false // TODO: by Jorge :)
	} else {
		return checkAndCache(ctx, "", "", "", eventCluster, eventIsHubClusterResource) // "oc auth can-i create managedclusterview -n <eventCluster> --as=<user>
	}
}

func checkAndCache(ctx context.Context, eventApigroup, eventKind, eventNamespace, eventCluster string, eventIsHubClusterResource bool) bool {
	userWatchData, userWatchDataErr := rbac.GetWatchCache().GetUserWatchData(ctx)
	if userWatchDataErr != nil {
		klog.Errorf("Failed to get user watch data to verify permissions to view event. Not authorizing with error: %v", userWatchDataErr)
		return false
	}

	if eventIsHubClusterResource {
		return userWatchData.CheckPermissionAndCache(ctx, "watch", eventApigroup, eventKind, eventNamespace)
	} else {
		return userWatchData.CheckPermissionAndCache(ctx, "create", "view.open-cluster-management.io", "managedclusterviews", eventCluster)
	}
}

// validateInputFilters validates the input filters.
func validateInputFilters(input *model.SearchInput) error {
	if input != nil && len(input.Filters) > 0 {
		for _, filter := range input.Filters {
			if filter == nil || filter.Property == "" {
				return fmt.Errorf("invalid filter. Property is required. Filter %+v", *filter)
			}
			// Validate label filter values are key=value pairs.
			if filter.Property == "label" {
				for _, value := range filter.Values {
					keyValue := strings.Split(*value, "=")
					if len(keyValue) != 2 {
						return fmt.Errorf("invalid filter. Value must be a key=value pair. {Property: %s Values: %s} ",
							filter.Property, *value)
					}
				}
			}
			if len(filter.Values) == 0 {
				return fmt.Errorf("invalid filter. Values are required. {Property: %s Values: %+v} ",
					filter.Property, filter.Values)
			}
			for _, value := range filter.Values {
				if value == nil || *value == "" {
					return fmt.Errorf("invalid filter. Value is required. Filter %+v", *filter)
				}
				// NOTE: The limitations below are only while we implement the feature.
				// They will be removed once the feature is fully implemented.
				if strings.Contains(*value, "*") {
					return fmt.Errorf("invalid filter. Wildcards are not yet supported. Property: %s Value: %s",
						filter.Property, *value)
				}
				if strings.HasPrefix(*value, "!") ||
					strings.HasPrefix(*value, "!=") ||
					strings.HasPrefix(*value, ">") ||
					strings.HasPrefix(*value, ">=") ||
					strings.HasPrefix(*value, "<") ||
					strings.HasPrefix(*value, "<=") {
					return fmt.Errorf("invalid filter. Operators !,!=,>,>=,<,<= are not yet supported. {Property: %s Value: %s} ",
						filter.Property, *value)
				}
			}
		}
	}
	return nil
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

	// Validate the input filters.
	if err := validateInputFilters(input); err != nil {
		return result, err
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
				if !eventMatchesAllFilters(event, input) {
					klog.V(4).Infof("Subscription watch(%s) event did not match filters (UID: %s, Operation: %s)",
						subID, event.UID, event.Operation)
					continue
				}

				// Filter events based on user RBAC
				if !eventMatchesRbac(ctx, event) {
					klog.V(4).Infof("Subscription watch(%s) event did not match RBAC filters (UID: %s, Operation: %s)",
						subID, event.UID, event.Operation)
					continue
				}

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
