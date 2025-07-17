// Copyright Contributors to the Open Cluster Management project
package notification

import (
	"context"
	"fmt"
	"strings"

	"github.com/stolostron/search-v2-api/graph/model"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"k8s.io/klog/v2"
)

// FilterBuilder helps create notification filters
type FilterBuilder struct {
	filter NotificationFilter
}

// NewFilterBuilder creates a new filter builder
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		filter: NotificationFilter{
			Kinds:      []string{},
			Namespaces: []string{},
			Clusters:   []string{},
			Labels:     make(map[string]string),
			Properties: make(map[string]interface{}),
			Operations: []string{},
		},
	}
}

// WithKinds adds resource kinds to filter
func (b *FilterBuilder) WithKinds(kinds ...string) *FilterBuilder {
	b.filter.Kinds = append(b.filter.Kinds, kinds...)
	return b
}

// WithNamespaces adds namespaces to filter
func (b *FilterBuilder) WithNamespaces(namespaces ...string) *FilterBuilder {
	b.filter.Namespaces = append(b.filter.Namespaces, namespaces...)
	return b
}

// WithClusters adds clusters to filter
func (b *FilterBuilder) WithClusters(clusters ...string) *FilterBuilder {
	b.filter.Clusters = append(b.filter.Clusters, clusters...)
	return b
}

// WithLabel adds a label filter
func (b *FilterBuilder) WithLabel(key, value string) *FilterBuilder {
	if b.filter.Labels == nil {
		b.filter.Labels = make(map[string]string)
	}
	b.filter.Labels[key] = value
	return b
}

// WithProperty adds a property filter
func (b *FilterBuilder) WithProperty(key string, value interface{}) *FilterBuilder {
	if b.filter.Properties == nil {
		b.filter.Properties = make(map[string]interface{})
	}
	b.filter.Properties[key] = value
	return b
}

// WithOperations adds operation types to filter (INSERT, UPDATE, DELETE)
func (b *FilterBuilder) WithOperations(operations ...string) *FilterBuilder {
	b.filter.Operations = append(b.filter.Operations, operations...)
	return b
}

// Build returns the constructed filter
func (b *FilterBuilder) Build() NotificationFilter {
	return b.filter
}

// FilterFromSearchInput creates a notification filter from a GraphQL SearchInput
func FilterFromSearchInput(searchInput *model.SearchInput) NotificationFilter {
	builder := NewFilterBuilder()

	// Default to all operations if not specified
	builder.WithOperations("INSERT", "UPDATE", "DELETE")

	if searchInput == nil {
		return builder.Build()
	}

	// Extract filters from SearchInput
	if searchInput.Filters != nil {
		for _, filter := range searchInput.Filters {
			switch strings.ToLower(filter.Property) {
			case "kind":
				kinds := make([]string, len(filter.Values))
				for i, v := range filter.Values {
					if v != nil {
						kinds[i] = *v
					}
				}
				builder.WithKinds(kinds...)

			case "namespace":
				namespaces := make([]string, len(filter.Values))
				for i, v := range filter.Values {
					if v != nil {
						namespaces[i] = *v
					}
				}
				builder.WithNamespaces(namespaces...)

			case "cluster":
				clusters := make([]string, len(filter.Values))
				for i, v := range filter.Values {
					if v != nil {
						clusters[i] = *v
					}
				}
				builder.WithClusters(clusters...)

			default:
				// Handle other properties
				if len(filter.Values) > 0 && filter.Values[0] != nil {
					builder.WithProperty(filter.Property, *filter.Values[0])
				}
			}
		}
	}

	return builder.Build()
}

// ApplyRBACToFilter applies RBAC constraints to a notification filter
func ApplyRBACToFilter(ctx context.Context, filter NotificationFilter) (NotificationFilter, error) {
	userData, err := rbac.GetCache().GetUserData(ctx)
	if err != nil {
		return filter, fmt.Errorf("failed to get user RBAC data: %w", err)
	}

	// If user is cluster admin, return filter as-is
	if userData.IsClusterAdmin {
		klog.V(3).Info("User is cluster admin, no RBAC filtering applied to notifications")
		return filter, nil
	}

	klog.V(3).Info("Applying RBAC constraints to notification filter")

	// Apply cluster filtering
	if userData.ManagedClusters != nil && len(userData.ManagedClusters) > 0 {
		allowedClusters := make([]string, 0, len(userData.ManagedClusters))
		for cluster := range userData.ManagedClusters {
			allowedClusters = append(allowedClusters, cluster)
		}

		// If filter already has clusters, intersect with allowed clusters
		if len(filter.Clusters) > 0 {
			intersectedClusters := []string{}
			for _, filterCluster := range filter.Clusters {
				for _, allowedCluster := range allowedClusters {
					if filterCluster == allowedCluster {
						intersectedClusters = append(intersectedClusters, filterCluster)
						break
					}
				}
			}
			filter.Clusters = intersectedClusters
		} else {
			filter.Clusters = allowedClusters
		}
	}

	// Apply namespace filtering for hub cluster resources
	if userData.NsResources != nil && len(userData.NsResources) > 0 {
		allowedNamespaces := make([]string, 0)
		for namespace := range userData.NsResources {
			// Add namespace if not already present
			found := false
			for _, existing := range allowedNamespaces {
				if existing == namespace {
					found = true
					break
				}
			}
			if !found {
				allowedNamespaces = append(allowedNamespaces, namespace)
			}
		}

		// If filter already has namespaces, intersect with allowed namespaces
		if len(filter.Namespaces) > 0 {
			intersectedNamespaces := []string{}
			for _, filterNamespace := range filter.Namespaces {
				for _, allowedNamespace := range allowedNamespaces {
					if filterNamespace == allowedNamespace {
						intersectedNamespaces = append(intersectedNamespaces, filterNamespace)
						break
					}
				}
			}
			filter.Namespaces = intersectedNamespaces
		}
		// Note: We don't automatically add all allowed namespaces if none specified
		// This allows users to get notifications for all resources they have access to
	}

	return filter, nil
}

// ValidateFilter checks if a notification filter is valid
func ValidateFilter(filter NotificationFilter) error {
	// Validate operations
	validOperations := map[string]bool{
		"INSERT": true,
		"UPDATE": true,
		"DELETE": true,
	}

	for _, op := range filter.Operations {
		if !validOperations[op] {
			return fmt.Errorf("invalid operation: %s", op)
		}
	}

	// Validate that at least one filtering criteria is provided if this is for a specific subscription
	hasFilters := len(filter.Kinds) > 0 ||
		len(filter.Namespaces) > 0 ||
		len(filter.Clusters) > 0 ||
		len(filter.Labels) > 0 ||
		len(filter.Properties) > 0

	if !hasFilters {
		klog.V(3).Info("No specific filters provided, will receive all notifications")
	}

	return nil
}

// CreateSubscriptionFilter creates a complete filter for a subscription, applying RBAC
func CreateSubscriptionFilter(ctx context.Context, searchInput *model.SearchInput) (NotificationFilter, error) {
	// Create base filter from search input
	filter := FilterFromSearchInput(searchInput)

	// Apply RBAC constraints
	rbacFilter, err := ApplyRBACToFilter(ctx, filter)
	if err != nil {
		return NotificationFilter{}, fmt.Errorf("failed to apply RBAC to filter: %w", err)
	}

	// Validate the final filter
	if err := ValidateFilter(rbacFilter); err != nil {
		return NotificationFilter{}, fmt.Errorf("invalid filter: %w", err)
	}

	return rbacFilter, nil
}

// FilterString returns a string representation of the filter for logging
func FilterString(filter NotificationFilter) string {
	var parts []string

	if len(filter.Operations) > 0 {
		parts = append(parts, fmt.Sprintf("operations:%v", filter.Operations))
	}
	if len(filter.Kinds) > 0 {
		parts = append(parts, fmt.Sprintf("kinds:%v", filter.Kinds))
	}
	if len(filter.Namespaces) > 0 {
		parts = append(parts, fmt.Sprintf("namespaces:%v", filter.Namespaces))
	}
	if len(filter.Clusters) > 0 {
		parts = append(parts, fmt.Sprintf("clusters:%v", filter.Clusters))
	}
	if len(filter.Labels) > 0 {
		parts = append(parts, fmt.Sprintf("labels:%v", filter.Labels))
	}
	if len(filter.Properties) > 0 {
		parts = append(parts, fmt.Sprintf("properties:%v", filter.Properties))
	}

	if len(parts) == 0 {
		return "no-filters"
	}

	return strings.Join(parts, ",")
}
