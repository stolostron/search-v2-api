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
// (( cluster = 'a' AND data->>'namespace' IN ['ns-1', 'ns-2', ...] )
// OR ( cluster = 'b' AND data->>'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
// AND
// (( data->'apigroup' ? 'kubevirt.io' AND data->'kind_plural' IN ['VirtualMachine', 'VirtualMachineInstance', ...] )
// OR data->'apigroup' ? 'snapshots.kubevirt.io' OR ...)
func matchFineGrainedRbac(userPermissionList clusterviewv1alpha1.UserPermissionList) exp.ExpressionList {
	result := goqu.Or()
	klog.Info("UserPermissionList items: ", len(userPermissionList.Items))
	for _, userPermission := range userPermissionList.Items {
		locations := goqu.Or()
		// This part matches the cluster and namespaces.
		// Resolves to:
		// (( cluster = 'a' AND data->>'namespace' IN ['ns-1', 'ns-2', ...] )
		// OR ( cluster = 'b' AND data->>'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
		for _, binding := range userPermission.Status.Bindings {
			if len(binding.Namespaces) == 1 && binding.Namespaces[0] == "*" {
				locations = locations.Append(goqu.C("cluster").Eq(binding.Cluster))
			} else {
				locations = locations.Append(goqu.And(
					goqu.C("cluster").Eq(binding.Cluster),
					goqu.L("data->???", "namespace", goqu.L("?|"), pq.Array(binding.Namespaces))))
			}
		}

		resources := goqu.Or()
		// This part matches the API group and kind.
		// Resolves to:
		// (( data->'apigroup' ? 'kubevirt.io' AND data->'kind_plural' IN ['VirtualMachine', 'VirtualMachineInstance', ...] )
		// OR data->'apigroup' ? 'snapshots.kubevirt.io' OR ...)
		for _, rule := range userPermission.Status.ClusterRoleDefinition.Rules {
			for _, verb := range rule.Verbs {
				if verb == "list" || verb == "*" {
					if len(rule.Resources) == 1 && rule.Resources[0] == "*" {
						resources = resources.Append(goqu.L("data->???", "apigroup", goqu.L("?"), rule.APIGroups[0]))
					} else {
						resources = resources.Append(goqu.And(
							goqu.L("data->???", "apigroup", goqu.L("?"), rule.APIGroups[0]),
							goqu.L("data->???", "kind_plural", goqu.L("?|"), pq.Array(rule.Resources)),
						))
					}
					// TODO: APIGroup is a list.
					continue
				}
			}
		}

		result = result.Append(goqu.And(locations, resources))
	}

	// if klog.V(4).Enabled() {
	sql, _, err := goqu.From("t").Where(result).ToSQL()
	klog.Info("\n >>>>Fine-grained RBAC clause is: ", sql, " error: ", err)
	// }
	return result
}
