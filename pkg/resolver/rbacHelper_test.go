// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"context"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	clusterviewv1alpha1 "github.com/stolostron/cluster-lifecycle-api/clusterview/v1alpha1"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/authentication/v1"
)

// Test that users with no permissions get blocked (not allowed to see everything)
func TestMatchHubClusterRbacNoPermissions(t *testing.T) {
	userInfo := v1.UserInfo{
		Username: "testuser",
		UID:      "test-uid",
	}

	// User with NO permissions
	userData := rbac.UserData{
		CsResources:    []rbac.Resource{},
		NsResources:    map[string][]rbac.Resource{},
		IsClusterAdmin: false,
	}

	result := matchHubClusterRbac(userData, userInfo)

	// Build a test query to verify the WHERE clause blocks access
	sql, _, err := goqu.From("search.resources").
		Where(
			goqu.L("data->'kind'?('RoleBinding')"),
			result,
		).
		ToSQL()

	assert.NoError(t, err)

	// The SQL should contain FALSE to block all access
	assert.Contains(t, sql, "FALSE", "Expected explicit FALSE in WHERE clause for users with no permissions")

	// Verify it's not an empty WHERE clause that allows everything
	assert.NotEqual(t, `SELECT * FROM "search"."resources" WHERE data->'kind'?('RoleBinding')`, sql,
		"WHERE clause should NOT be ignored - must contain RBAC restriction")
}

// Test that the entire buildRbacWhereClause blocks access for users with no permissions
func TestBuildRbacWhereClauseNoPermissions(t *testing.T) {
	ctx := context.Background()
	userInfo := v1.UserInfo{
		Username: "testuser",
		UID:      "test-uid",
	}

	// User with NO permissions and fine-grained RBAC enabled but no UserPermissions
	userData := rbac.UserData{
		CsResources:     []rbac.Resource{},
		NsResources:     map[string][]rbac.Resource{},
		ManagedClusters: map[string]struct{}{},
		IsClusterAdmin:  false,
		UserPermissions: clusterviewv1alpha1.UserPermissionList{Items: []clusterviewv1alpha1.UserPermission{}},
	}

	result := buildRbacWhereClause(ctx, userData, userInfo)

	// Build a test query
	sql, _, err := goqu.From("search.resources").
		Where(
			goqu.L("data->'kind'?('RoleBinding')"),
			result,
		).
		ToSQL()

	assert.NoError(t, err)

	// Should contain FALSE to block access
	assert.Contains(t, sql, "FALSE", "Expected explicit FALSE for users with no permissions")
}

// Test matchClusterScopedResources with no permissions
func TestMatchClusterScopedResourcesEmpty(t *testing.T) {
	userInfo := v1.UserInfo{
		Username: "testuser",
		UID:      "test-uid",
	}

	result := matchClusterScopedResources([]rbac.Resource{}, userInfo)

	sql, _, err := goqu.From("search.resources").
		Where(result).
		ToSQL()

	assert.NoError(t, err)
	assert.Contains(t, sql, "FALSE", "Expected FALSE for empty cluster-scoped resources")
}

// Test matchClusterScopedResources with wildcard permissions
func TestMatchClusterScopedResourcesWildcard(t *testing.T) {
	userInfo := v1.UserInfo{
		Username: "testuser",
		UID:      "test-uid",
	}

	// User has access to ALL cluster-scoped resources
	result := matchClusterScopedResources([]rbac.Resource{
		{Apigroup: "*", Kind: "*"},
	}, userInfo)

	sql, _, err := goqu.From("search.resources").
		Where(result).
		ToSQL()

	assert.NoError(t, err)

	// Should NOT contain restrictive filters when user has wildcard access
	// The query should be relatively simple (no complex apigroup/kind filters)
	assert.NotContains(t, sql, "FALSE", "Should not block when user has wildcard access")

	// Basic sanity check - query should be executable
	assert.True(t, strings.HasPrefix(sql, "SELECT"), "Should produce valid SQL")
}

// Test matchNamespacedResources with no permissions
func TestMatchNamespacedResourcesEmpty(t *testing.T) {
	userInfo := v1.UserInfo{
		Username: "testuser",
		UID:      "test-uid",
	}

	result := matchNamespacedResources(map[string][]rbac.Resource{}, userInfo)

	sql, _, err := goqu.From("search.resources").
		Where(result).
		ToSQL()

	assert.NoError(t, err)

	// Empty namespace resources should result in no matches (not match everything)
	// An empty OR() should produce minimal/no results
	assert.NotContains(t, sql, "data->'namespace'", "Should not have namespace filters when no permissions")
}

// Test that users with specific permissions get proper filters
func TestMatchHubClusterRbacWithPermissions(t *testing.T) {
	userInfo := v1.UserInfo{
		Username: "testuser",
		UID:      "test-uid",
	}

	// User with specific permissions
	userData := rbac.UserData{
		CsResources: []rbac.Resource{
			{Apigroup: "rbac.authorization.k8s.io", Kind: "clusterroles"},
		},
		NsResources: map[string][]rbac.Resource{
			"default": {
				{Apigroup: "apps", Kind: "deployments"},
			},
		},
		IsClusterAdmin: false,
	}

	result := matchHubClusterRbac(userData, userInfo)

	sql, _, err := goqu.From("search.resources").
		Where(result).
		ToSQL()

	assert.NoError(t, err)

	// Should contain _hubClusterResource filter
	assert.Contains(t, sql, "_hubClusterResource", "Should check for hub cluster resources")

	// Should NOT contain FALSE (user has permissions)
	assert.NotContains(t, sql, "FALSE", "Should not block when user has valid permissions")

	// Should contain filters for the permitted resources
	assert.Contains(t, sql, "clusterroles", "Should filter for permitted cluster-scoped resources")
}
