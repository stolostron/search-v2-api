// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
	clusterviewv1alpha1 "github.com/stolostron/cluster-lifecycle-api/clusterview/v1alpha1"
	"k8s.io/klog/v2"
)

// Fine-grained RBAC for UserPermissions.
// Resolves to:
// (cluster = 'a' OR (cluster = 'b' AND data->>'namespace' IN ['ns-1', 'ns-2', ...] )
// OR ( cluster = 'c' AND data->>'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
// AND
// (( data->'apigroup' ? 'kubevirt.io' AND data->'kind_plural' IN ['VirtualMachine', 'VirtualMachineInstance', ...] )
// OR data->'apigroup' ? 'snapshots.kubevirt.io' OR ...)
func matchFineGrainedRbac(userPermissionList clusterviewv1alpha1.UserPermissionList) exp.ExpressionList {
	result := goqu.Or()

	for _, userPermission := range userPermissionList.Items {
		result = result.Append(
			goqu.And(
				matchClusterAndNamespaces(userPermission),
				matchApiGroupAndKind(userPermission)))
	}

	if klog.V(1).Enabled() { // TODO: Change to V(5) before merging.
		sql, _, err := goqu.From("t").Where(result).ToSQL()
		klog.Info(">>>> Fine-grained RBAC WHERE clause:\n", sql, "\n error: ", err)
	}
	return result
}

// Builds the part of the query that matches the location (cluster and namespaces) of a resource.
// Resolves to:
// (cluster = 'a' OR (cluster = 'b' AND data->>'namespace' IN ['ns-1', 'ns-2', ...] )
// OR ( cluster = 'c' AND data->>'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
func matchClusterAndNamespaces(userPermission clusterviewv1alpha1.UserPermission) exp.ExpressionList {
	result := goqu.Or()

	clusterScopedNames := make([]string, 0)
	for _, binding := range userPermission.Status.Bindings {
		if binding.Scope == "cluster" ||
			(len(binding.Namespaces) == 1 && binding.Namespaces[0] == "*") {
			// Collect the cluster name. Query gets built after all bindings are processed.
			clusterScopedNames = append(clusterScopedNames, binding.Cluster)
		} else {
			// Matches the location (cluster + namespace).
			result = result.Append(goqu.And(
				goqu.C("cluster").Eq(binding.Cluster),
				goqu.L("data->???", "namespace", goqu.L("?|"), pq.Array(binding.Namespaces))))
		}
	}
	if len(clusterScopedNames) > 0 {
		result = result.Append(goqu.C("cluster").In(clusterScopedNames))
	}

	return result
}

// Builds the part of the query that matches the API group and kind of a resource.
// Resolves to:
// (( data->'apigroup' ? 'kubevirt.io' AND data->'kind_plural' IN ['VirtualMachine', 'VirtualMachineInstance', ...] )
// OR data->'apigroup' ? 'snapshots.kubevirt.io' OR ...)
// Cases:
//  1. apigroup is empty string.
//  2. apigroup is an array of strings.
//  3. apigroup has a wilcard (*).
//  4. kind is an array of strings.
//  5. kind has a wildcard (*).
//  6. both apigroup and kind have a wildcard (*).
//  7. both apigroup and kind are arrays of strings.
func matchApiGroupAndKind(userPermission clusterviewv1alpha1.UserPermission) exp.ExpressionList {
	result := goqu.Or()
	for _, rule := range userPermission.Status.ClusterRoleDefinition.Rules {
		for _, verb := range rule.Verbs {
			if verb == "list" || verb == "*" {
				if len(rule.Resources) == 1 && rule.Resources[0] == "*" {
					result = result.Append(goqu.L("data->???", "apigroup", goqu.L("?"), rule.APIGroups[0]))
				} else {
					result = result.Append(goqu.And(
						goqu.L("data->???", "apigroup", goqu.L("?"), rule.APIGroups[0]),
						goqu.L("data->???", "kind_plural", goqu.L("?|"), pq.Array(rule.Resources)),
					))
				}
			}
		}
	}

	if klog.V(1).Enabled() { // TODO: Change to V(7) before merging.
		sql, _, err := goqu.From("t").Where(result).ToSQL()
		klog.Infof(">> UserPermission [%s] WHERE clause (apigroup and kind):\n %s\n error: %v",
			userPermission.Name, sql, err)
	}
	return result
}
