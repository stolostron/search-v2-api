// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	klog "k8s.io/klog/v2"

	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/config"
	db "github.com/stolostron/search-v2-api/pkg/database"
	"github.com/stolostron/search-v2-api/pkg/rbac"
)

// NotificationEvent represents a database notification event
type NotificationEvent struct {
	Operation string                 `json:"operation"` // INSERT, UPDATE, DELETE
	OldRow    map[string]interface{} `json:"old_row,omitempty"`
	NewRow    map[string]interface{} `json:"new_row,omitempty"`
}

func SearchSubscription(ctx context.Context, input []*model.SearchInput) (<-chan []*SearchResult, error) {
	ch := make(chan []*SearchResult)

	// if not enabled via feature flag -> return error message
	if !config.Cfg.Features.SubscriptionEnabled {
		klog.Infof("GraphQL subscription requests are disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
		ctx.Done()
		close(ch)
		return ch, errors.New("GraphQL subscription requests are disabled. To enable set env variable FEATURE_SUBSCRIPTION=true")
	}

	// Ensure database triggers are set up
	if err := setupDatabaseTriggers(ctx); err != nil {
		klog.Errorf("Failed to setup database triggers: %s", err)
		close(ch)
		return ch, err
	}

	// Start the notification listener
	go func() {
		defer close(ch)

		// Get user data for RBAC filtering
		userData, userDataErr := rbac.GetCache().GetUserData(ctx)
		if userDataErr != nil {
			klog.Errorf("Error getting user data for subscription: %s", userDataErr)
			return
		}

		// Get property types for filtering
		propTypes, err := getPropertyType(ctx, false)
		if err != nil {
			klog.Warningf("Error creating datatype map. Error: [%s]", err)
		}

		// Create connection for LISTEN
		conn, err := db.GetConnPool(ctx).Acquire(ctx)
		if err != nil {
			klog.Errorf("Failed to acquire database connection for subscription: %s", err)
			return
		}
		defer conn.Release()

		// Start listening for notifications
		_, err = conn.Exec(ctx, "LISTEN search_resource_changes")
		if err != nil {
			klog.Errorf("Failed to start listening for database notifications: %s", err)
			return
		}

		klog.V(3).Info("Started listening for search resource changes")

		timeout := time.After(time.Duration(config.Cfg.SubscriptionRefreshTimeout) * time.Millisecond)

		for {
			select {
			case <-ctx.Done():
				klog.V(3).Info("Search subscription closed due to context cancellation")
				return

			case <-timeout:
				klog.V(3).Info("Subscription timeout reached. Closing connection.")
				return

			default:
				// Wait for notification with a short timeout
				notification, err := conn.Conn().WaitForNotification(ctx)
				if err != nil {
					// Check if this is a timeout or context cancellation
					if ctx.Err() != nil {
						return
					}
					// For other errors, log and continue
					klog.V(5).Infof("No notification received or error: %s", err)
					time.Sleep(100 * time.Millisecond) // Small delay to prevent busy waiting
					continue
				}

				if notification != nil {
					klog.V(4).Infof("Received database notification: %s", notification.Payload)

					// Process the notification
					results, err := processNotification(ctx, notification.Payload, input, userData, propTypes)
					if err != nil {
						klog.Errorf("Error processing notification: %s", err)
						continue
					}

					if len(results) > 0 {
						// Send results to the subscription channel
						select {
						case ch <- results:
							klog.V(4).Info("Sent notification results to subscription channel")
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()

	// Return the channel
	return ch, nil
}

// setupDatabaseTriggers ensures that the necessary database triggers are in place
func setupDatabaseTriggers(ctx context.Context) error {
	pool := db.GetConnPool(ctx)
	if pool == nil {
		return fmt.Errorf("unable to get database connection pool")
	}

	// Create the trigger function if it doesn't exist
	triggerFunction := `
		CREATE OR REPLACE FUNCTION notify_search_resource_change()
		RETURNS trigger AS $$
		DECLARE
			notification_payload json;
		BEGIN
			-- Build the notification payload based on the operation
			IF TG_OP = 'DELETE' then
				notification_payload = json_build_object(
					'operation', TG_OP,
					'old_row', json_build_object(
						'uid', OLD.uid,
						'cluster', OLD.cluster,
						'data', OLD.data
					)
				);
			ELSIF TG_OP = 'INSERT' then
				notification_payload = json_build_object(
					'operation', TG_OP,
					'new_row', json_build_object(
						'uid', NEW.uid,
						'cluster', NEW.cluster,
						'data', NEW.data
					)
				);
			ELSIF TG_OP = 'UPDATE' then
				notification_payload = json_build_object(
					'operation', TG_OP,
					'old_row', json_build_object(
						'uid', OLD.uid,
						'cluster', OLD.cluster,
						'data', OLD.data
					),
					'new_row', json_build_object(
						'uid', NEW.uid,
						'cluster', NEW.cluster,
						'data', NEW.data
					)
				);
			END IF;
			
			-- Send the notification
			PERFORM pg_notify('search_resource_changes', notification_payload::text);
			
			-- Return the appropriate row
			IF TG_OP = 'DELETE' THEN
				RETURN OLD;
			ELSE
				RETURN NEW;
			END IF;
		END;
		$$ LANGUAGE plpgsql;
	`

	_, err := pool.Exec(ctx, triggerFunction)
	if err != nil {
		return fmt.Errorf("failed to create trigger function: %w", err)
	}

	// Create the trigger if it doesn't exist
	trigger := `
		DO $$
		BEGIN
			-- Check if trigger exists and drop it if it does
			IF EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'search_resource_change_trigger') THEN
				DROP TRIGGER search_resource_change_trigger ON search.resources;
			END IF;
			
			-- Create the trigger
			CREATE TRIGGER search_resource_change_trigger
				AFTER INSERT OR UPDATE OR DELETE ON search.resources
				FOR EACH ROW EXECUTE FUNCTION notify_search_resource_change();
		END $$;
	`

	_, err = pool.Exec(ctx, trigger)
	if err != nil {
		return fmt.Errorf("failed to create trigger: %w", err)
	}

	klog.V(3).Info("Database triggers setup completed")
	return nil
}

// processNotification processes a database notification and returns matching search results
func processNotification(ctx context.Context, payload string, inputs []*model.SearchInput, userData rbac.UserData, propTypes map[string]string) ([]*SearchResult, error) {
	// Parse the notification payload
	var event NotificationEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil, fmt.Errorf("failed to parse notification payload: %w", err)
	}

	var results []*SearchResult

	// Process each search input to see if the notification matches
	for _, input := range inputs {
		// Check if the notification matches this search input
		matches, matchedRow, err := doesNotificationMatchSearch(event, input, userData, propTypes)
		if err != nil {
			klog.Errorf("Error checking notification match: %s", err)
			continue
		}

		if matches && matchedRow != nil {
			// Create a SearchResult with the matching data
			searchResult := &SearchResult{
				input:     input,
				pool:      db.GetConnPool(ctx),
				userData:  userData,
				context:   ctx,
				propTypes: propTypes,
			}

			// Set the UIDs for the matched row
			if uid, ok := matchedRow["uid"].(string); ok {
				searchResult.uids = []*string{&uid}
			}

			results = append(results, searchResult)
		}
	}

	return results, nil
}

// doesNotificationMatchSearch checks if a notification matches a search input criteria
func doesNotificationMatchSearch(event NotificationEvent, input *model.SearchInput, userData rbac.UserData, propTypes map[string]string) (bool, map[string]interface{}, error) {
	// Determine which row to check based on operation
	var rowToCheck map[string]interface{}
	switch event.Operation {
	case "INSERT", "UPDATE":
		rowToCheck = event.NewRow
	case "DELETE":
		rowToCheck = event.OldRow
	default:
		return false, nil, fmt.Errorf("unknown operation: %s", event.Operation)
	}

	if rowToCheck == nil {
		return false, nil, fmt.Errorf("no row data in notification")
	}

	// Check RBAC permissions first
	if !checkRowRBAC(rowToCheck, userData) {
		return false, nil, nil
	}

	// Check if the row matches the search criteria
	matches, err := checkRowMatchesSearch(rowToCheck, input, propTypes)
	if err != nil {
		return false, nil, err
	}

	if matches {
		return true, rowToCheck, nil
	}

	return false, nil, nil
}

// checkRowRBAC checks if the user has RBAC permissions for the given row
func checkRowRBAC(row map[string]interface{}, userData rbac.UserData) bool {
	// If user is cluster admin, they can see everything
	if userData.IsClusterAdmin {
		return true
	}

	// Extract cluster and data from row
	cluster, ok := row["cluster"].(string)
	if !ok {
		return false
	}

	data, ok := row["data"].(map[string]interface{})
	if !ok {
		return false
	}

	// Check managed cluster access
	if userData.ManagedClusters != nil {
		if _, hasAccess := userData.ManagedClusters[cluster]; hasAccess {
			return true
		}
	}

	// For hub cluster resources, check more detailed RBAC
	if cluster == config.Cfg.HubName || data["_hubClusterResource"] != nil {
		// Check namespace-scoped resources
		if namespace, ok := data["namespace"].(string); ok {
			if userData.NsResources != nil {
				if nsResources, hasNamespace := userData.NsResources[namespace]; hasNamespace {
					return checkResourceAccess(data, nsResources)
				}
			}
		} else {
			// Check cluster-scoped resources
			if userData.CsResources != nil {
				return checkResourceAccess(data, userData.CsResources)
			}
		}
	}

	return false
}

// checkResourceAccess checks if the user has access to the specific resource type
func checkResourceAccess(data map[string]interface{}, allowedResources []rbac.Resource) bool {
	kind, ok := data["kind_plural"].(string)
	if !ok {
		if k, ok := data["kind"].(string); ok {
			kind = strings.ToLower(k) + "s" // Simple pluralization
		} else {
			return false
		}
	}

	apigroup, _ := data["apigroup"].(string)

	for _, resource := range allowedResources {
		if resource.Kind == kind && resource.Apigroup == apigroup {
			return true
		}
	}

	return false
}

// checkRowMatchesSearch checks if a row matches the search input criteria
func checkRowMatchesSearch(row map[string]interface{}, input *model.SearchInput, propTypes map[string]string) (bool, error) {
	data, ok := row["data"].(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("invalid row data format")
	}

	cluster, ok := row["cluster"].(string)
	if !ok {
		return false, fmt.Errorf("invalid cluster format")
	}

	// Create a combined row with cluster and data fields flattened
	combinedRow := make(map[string]interface{})
	combinedRow["cluster"] = cluster
	for key, value := range data {
		combinedRow[key] = value
	}

	// Check keyword matches
	if len(input.Keywords) > 0 {
		if !checkKeywordMatch(combinedRow, input.Keywords) {
			return false, nil
		}
	}

	// Check filter matches
	if len(input.Filters) > 0 {
		if !checkFilterMatches(combinedRow, input.Filters, propTypes) {
			return false, nil
		}
	}

	return true, nil
}

// checkKeywordMatch checks if any keywords match the row data
func checkKeywordMatch(row map[string]interface{}, keywords []*string) bool {
	if len(keywords) == 0 {
		return true
	}

	// Convert row to JSON string for text search
	jsonData, err := json.Marshal(row)
	if err != nil {
		return false
	}
	rowText := strings.ToLower(string(jsonData))

	// All keywords must match (AND operation)
	for _, keyword := range keywords {
		if keyword != nil {
			if !strings.Contains(rowText, strings.ToLower(*keyword)) {
				return false
			}
		}
	}

	return true
}

// checkFilterMatches checks if all filters match the row data
func checkFilterMatches(row map[string]interface{}, filters []*model.SearchFilter, propTypes map[string]string) bool {
	// All filters must match (AND operation)
	for _, filter := range filters {
		if !checkSingleFilterMatch(row, filter, propTypes) {
			return false
		}
	}
	return true
}

// checkSingleFilterMatch checks if a single filter matches the row data
func checkSingleFilterMatch(row map[string]interface{}, filter *model.SearchFilter, propTypes map[string]string) bool {
	if len(filter.Values) == 0 {
		return true
	}

	// Get the property value from the row
	var propertyValue interface{}
	var exists bool

	if filter.Property == "cluster" {
		propertyValue, exists = row["cluster"]
	} else {
		propertyValue, exists = row[filter.Property]
	}

	if !exists {
		return false
	}

	// Convert property value to string for comparison
	var propertyStr string
	switch v := propertyValue.(type) {
	case string:
		propertyStr = v
	case float64:
		propertyStr = fmt.Sprintf("%.0f", v)
	case bool:
		propertyStr = fmt.Sprintf("%t", v)
	default:
		// For complex types, convert to JSON
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return false
		}
		propertyStr = string(jsonBytes)
	}

	// Check if any filter value matches (OR operation)
	for _, filterValue := range filter.Values {
		if filterValue != nil {
			if matchesFilterValue(propertyStr, *filterValue, filter.Property, propTypes) {
				return true
			}
		}
	}

	return false
}

// matchesFilterValue checks if a property value matches a filter value
func matchesFilterValue(propertyValue, filterValue, property string, propTypes map[string]string) bool {
	// Handle special case for kind - case insensitive matching
	if property == "kind" {
		return strings.EqualFold(propertyValue, filterValue)
	}

	// Parse operator from filter value
	operator, operand := getOperatorFromString(filterValue)

	switch operator {
	case "=":
		return propertyValue == operand
	case "!=":
		return propertyValue != operand
	case "*":
		// Wildcard match
		pattern := strings.ReplaceAll(operand, "*", ".*")
		matched, _ := regexp.MatchString(pattern, propertyValue)
		return matched
	default:
		// For now, default to exact match
		return propertyValue == filterValue
	}
}