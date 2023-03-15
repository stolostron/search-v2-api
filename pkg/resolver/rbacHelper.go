// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"encoding/json"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	v1 "k8s.io/api/authentication/v1"
	"k8s.io/klog/v2"
)

// function to loop through resources and build the where clause
// Resolves to something similar to:
//    ((apigroup='' AND kind='') OR (apigroup='' AND kind='') OR ... )
func matchApigroupKind(resources []rbac.Resource) exp.ExpressionList {
	var whereCsDs exp.ExpressionList // Stores the where clause for cluster scoped resources

	for i, clusterRes := range resources {
		whereOrDs := []exp.Expression{}
		//add apigroup filter
		if clusterRes.Apigroup != "*" { // if all apigroups are allowed, this filter is not needed
			var isApiGrp exp.LiteralExpression
			if clusterRes.Apigroup == "" { // if apigroup is empty
				isApiGrp = goqu.L("NOT(???)", goqu.C("data"), goqu.Literal("?"), "apigroup")
			} else {
				isApiGrp = goqu.L("???", goqu.L(`data->?`, "apigroup"),
					goqu.Literal("?"), clusterRes.Apigroup)
			}
			whereOrDs = append(whereOrDs, isApiGrp) //data->'apigroup'?'storage.k8s.io'
		}
		//add kind filter
		if clusterRes.Kind != "*" { // if all kinds are allowed, this filter is not needed
			whereOrDs = append(whereOrDs, goqu.L("???", goqu.L(`data->?`, "kind_plural"),
				goqu.Literal("?"), clusterRes.Kind))
		}
		// special case: if both apigroup and kind are stars - all resources are allowed
		if clusterRes.Apigroup == "*" && clusterRes.Kind == "*" {
			// no clauses are needed as everything is allowed - return an empty clause
			return goqu.Or()
		}
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

// Match cluster-scoped resources, which are identified by not having the namespace property.
// Resolves to something like:
//   (AND data->>'namespace' = '')
func matchClusterScopedResources(csRes []rbac.Resource, userInfo v1.UserInfo) exp.ExpressionList {
	if len(csRes) == 0 {
		return goqu.Or() // return empty clause

		// user has access to all cluster scoped resources
	} else if len(csRes) == 1 && csRes[0].Apigroup == "*" && csRes[0].Kind == "*" {
		klog.V(5).Infof(
			"User %s with UID %s has access to all clusterscoped resources. Excluding cluster scoped filters",
			userInfo.Username, userInfo.UID)
		return goqu.Or() // return empty clause

	} else {
		//cluster scoped resources do not have namespace set. So, add the condition below to check that.
		return goqu.And(goqu.L("NOT(???)", goqu.C("data"), goqu.Literal("?"), "namespace"), //NOT("data"?'namespace')
			matchApigroupKind(csRes))
	}
}

// For each namespace, match the authorized resources (apigroup + kind)
// Resolves to some similar to:
//    (namespace = 'a' AND ((apigroup='' AND kind='') OR (apigroup='' AND kind='') OR ... ) OR
//    (namespace = 'b' AND ( ... ) OR (namespace = 'c' AND ( ... ) OR ...
func matchNamespacedResources(nsResources map[string][]rbac.Resource, userInfo v1.UserInfo) exp.ExpressionList {
	var whereNsDs []exp.Expression
	namespaces := getKeys(nsResources)
	if len(nsResources) < 1 { // no namespace scoped resources for user
		klog.V(5).Infof("User %s with UID %s has no access to namespace scoped resources.",
			userInfo.Username, userInfo.UID)
		return goqu.Or(whereNsDs...)

	} else if len(nsResources) == 1 && namespaces[0] == "*" { // user has access to all namespaces
		klog.V(5).Infof("User %s with UID %s has access to all namespaces. Excluding individual namespace filters",
			userInfo.Username, userInfo.UID)
		return goqu.Or() // return empty clause

	} else {
		var unMarshalErr error

		//consolidate namespace resources
		consolidateNsList, keys, jsonMarshalErr := consolidateNsResources(nsResources)
		whereNsDs = make([]exp.Expression, len(consolidateNsList))
		if jsonMarshalErr == nil {
			klog.V(2).Info("Using consolidated namespace list")
			for count, resources := range keys {
				namespaces := consolidateNsList[resources]
				resList := []rbac.Resource{}
				unMarshalErr = json.Unmarshal([]byte(resources), &resList)
				if unMarshalErr == nil {
					whereNsDs[count] = goqu.And(goqu.L("???", goqu.L(`data->?`, "namespace"),
						goqu.Literal("?|"), pq.Array(namespaces)),
						matchApigroupKind(resList))
				} else {
					break // use non-consolidated namespace list
				}
			}
		}
		// if consolidating namespaces, doesn't work, proceed as usual without consolidation
		if jsonMarshalErr != nil || unMarshalErr != nil {
			klog.V(2).Info("Using non-consolidated namespace list")
			whereNsDs = make([]exp.Expression, len(nsResources))
			for nsCount, namespace := range namespaces {
				whereNsDs[nsCount] = goqu.And(goqu.L("???", goqu.L(`data->?`, "namespace"),
					goqu.Literal("?"), namespace),
					matchApigroupKind(nsResources[namespace]))
			}
		}

		return goqu.Or(whereNsDs...)
	}
}

// Consolidate namespace resources by resource groups as key and namespaces as values
// Returns map with resource groups
// array with keys of the map - to preserve order for testing
// error if any, while marshaling the resource groups
func consolidateNsResources(nsResources map[string][]rbac.Resource) (map[string][]string, []string, error) {
	m := map[string][]string{}

	for ns, resources := range nsResources {
		b, err := json.Marshal(resources)
		if err == nil {
			if _, found := m[string(b)]; found {
				m[string(b)] = append(m[string(b)], ns)
			} else {
				m[string(b)] = []string{ns}
			}
		} else {
			klog.Info("Error marshaling resources:", err)
			return nil, nil, err
		}
	}

	klog.V(4).Infof("RBAC consolidation reduced from %d namespaces/s to %d namespace group/s.", len(nsResources), len(m))
	return m, getKeys(m), nil
}

// Match cluster scoped and namespace scoped resources from the hub.
// These are identified by containing the property _hubClusterResource=true
// Resolves to:
// (data->>'_hubClusterResource' = true)
// AND ((namespace=null AND apigroup AND kind) OR
// 		(namespace AND apiproup AND kind))
func matchHubCluster(userrbac *rbac.UserData, userInfo v1.UserInfo) exp.ExpressionList {
	if len(userrbac.CsResources) == 0 && len(userrbac.NsResources) == 0 {
		// Do not match hub cluster if user doesn't have access to cluster scoped or namespace scoped resources on hub
		return goqu.And()
	} else {
		// hub cluster rbac clause
		return goqu.And(
			goqu.L("???", goqu.C("data"), goqu.Literal("?"), "_hubClusterResource"), // "data"?'_hubClusterResource'
			goqu.Or(
				matchClusterScopedResources(userrbac.CsResources, userInfo), // (namespace=null AND apigroup AND kind)
				matchNamespacedResources(userrbac.NsResources, userInfo),    // (namespace AND apiproup AND kind)
			),
		)
	}
}

// Match resources from the managed clusters.
// Resolves to:
//    ( cluster IN ['a', 'b', ...] )
func matchManagedCluster(managedClusters []string) exp.BooleanExpression {
	//managed clusters
	return goqu.C("cluster").Eq(goqu.Any(pq.Array(managedClusters)))
}
