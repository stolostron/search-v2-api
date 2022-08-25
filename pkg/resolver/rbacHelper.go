package resolver

import (
	"context"
	"sort"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
	"github.com/stolostron/search-v2-api/pkg/rbac"
)

// function to loop through resources and build the where clause
// Resolves to something similar to:
//    ((apigroup='' AND kind='') OR (apigroup='' AND kind='') OR ... )
func matchApigroupKind(resources []rbac.Resource) exp.ExpressionList {
	var whereCsDs exp.ExpressionList // Stores the where clause for cluster scoped resources

	for i, clusterRes := range resources {
		whereOrDs := []exp.Expression{goqu.COALESCE(goqu.L(`data->>?`, "apigroup"), "").Eq(clusterRes.Apigroup),
			goqu.L(`data->>?`, "kind_plural").Eq(clusterRes.Kind)}

		// Using this workaround to build the AND-OR combination query in goqu.
		// Otherwise, by default goqu will AND everything
		// (apigroup='' AND kind='') OR (apigroup='' AND kind='')
		if i == 0 {
			whereCsDs = goqu.And(whereOrDs...) // First time, AND all conditions
		} else {
			//Next time onwards, perform OR with the existing conditions
			whereCsDs = goqu.Or(whereCsDs, goqu.And(whereOrDs...))
		}
	}
	return whereCsDs
}
func matchClusterScopedResources(csRes []rbac.Resource) exp.ExpressionList {
	if len(csRes) > 0 {
		return goqu.And(goqu.COALESCE(goqu.L(`data->>?`, "namespace"), "").Eq(""),
			matchApigroupKind(csRes))

	}
	return exp.NewExpressionList(0, nil)
}

func matchNamespacedResources(nsResources map[string][]rbac.Resource) exp.ExpressionList {
	var whereNsDs []exp.Expression
	if len(nsResources) > 0 {
		whereNsDs = make([]exp.Expression, len(nsResources))
		namespaces := make([]string, len(nsResources))
		i := 0
		for namespace := range nsResources {
			namespaces[i] = namespace
			i++
		}
		sort.Strings(namespaces) //to make unit tests pass
		for nsCount, namespace := range namespaces {
			whereNsDs[nsCount] = goqu.And(goqu.L(`data->>?`, "namespace").Eq(namespace),
				matchApigroupKind(nsResources[namespace]))
		}
	}
	return goqu.Or(whereNsDs...)
}

func matchHubCluster() exp.BooleanExpression {
	//hub cluster
	return goqu.L(`data->>?`, "_hubClusterResource").Eq("true")
}

func matchManagedCluster(managedClusters []string) exp.BooleanExpression {
	//managed clusters
	return goqu.C("cluster").Eq(goqu.Any(pq.Array(managedClusters)))
}

func getUserAccessData(ctx context.Context) (*rbac.UserDataCache, error) {
	userData, userDataErr := rbac.CacheInst.GetUserData(ctx, nil)
	if userDataErr != nil {
		return nil, userDataErr
	}
	// Proceed if user's rbac data exists
	// Get a copy of the current user access if user data exists

	userAccess := &rbac.UserDataCache{
		CsResources:     userData.GetCsResources(),
		NsResources:     userData.GetNsResources(),
		ManagedClusters: userData.GetManagedClusters(),
	}
	return userAccess, nil
}
