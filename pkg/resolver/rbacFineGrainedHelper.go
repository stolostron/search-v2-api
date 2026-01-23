// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"fmt"
	"slices"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
	clusterviewv1alpha1 "github.com/stolostron/cluster-lifecycle-api/clusterview/v1alpha1"
	"k8s.io/klog/v2"
)

/* DELETE THIS COMMENTED CODE BEFORE MERGE.
 * I'm leaving the original code here because it makes the diff look cleaner for review.
 *
 *

// TODO: Allow to expand this hard-coded list with an env variable.
// Currently hardcoding the Kubevirt apigroup and kinds. A future iteration should update this
// to read the ClusterRole resource to extract this information.
var kubevirtResourcesMap = map[string][]string{
	"kubevirt.io": {"VirtualMachine", "VirtualMachineInstance", "VirtualMachineInstanceMigration",
		"VirtualMachineInstancePreset", "VirtualMachineInstanceReplicaset"},
	"clone.kubevirt.io":  {"VirtualMachineClone"},
	"export.kubevirt.io": {"VirtualMachineExport"},
	"instancetype.kubevirt.io": {"VirtualMachineClusterInstancetype", "VirtualMachineClusterPreference",
		"VirtualMachineInstancetype", "VirtualMachinePreference"},
	"migrations.kubevirt.io": {"MigrationPolicy"},
	"pool.kubevirt.io":       {"VirtualMachinePool"},
	"snapshot.kubevirt.io":   {"VirtualMachineRestore", "VirtualMachineSnapshot", "VirtualMachineSnapshotContent"},
}

// Match resources using fine-grained RBAC.
// Resolves to:
// data->'apigroup' ? 'kubevirt.io' AND data->>'kind' ? 'VirtualMachine' OR ...
//
//	AND (( cluster = 'a' AND data->'namespace' IN ['ns-1', 'ns-2', ...] )
//	OR ( cluster = 'b' AND data->'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
func matchFineGrainedRbac(clusterNamespacesMap map[string][]string) exp.ExpressionList {
	result := goqu.Or(
		// Match the Namespace objects.
		matchNamespaceObject(clusterNamespacesMap),
		// Match namespaced resources.
		goqu.And(
			matchGroupKind(kubevirtResourcesMap),
			matchClusterAndNamespace(clusterNamespacesMap),
		),
	)

	if klog.V(4).Enabled() {
		sql, _, err := goqu.From("t").Where(result).ToSQL()
		klog.V(4).Info("Fine-grained RBAC clause is: ", sql, " error: ", err)
	}
	return result
}

// Match apiGroup + kind for fine-grained RBAC.
// Resolves to:
// ((data->'apigroup' ? 'kubevirt.io' AND data->'kind' ? 'VirtualMachine')
// OR data->'apigroup' ? 'snapshots.kubevirt.io' OR ...)
func matchGroupKind(groupKind map[string][]string) exp.ExpressionList {
	result := exp.NewExpressionList(exp.ExpressionListType(exp.OrType))

	for group, kinds := range groupKind {
		if len(kinds) == 0 {
			result = result.Append(goqu.L("data->???", "apigroup", goqu.L("?"), group))
		} else {
			result = result.Append(
				goqu.And(
					goqu.L("data->???", "apigroup", goqu.L("?"), group),
					goqu.L("data->???", "kind", goqu.L("?|"), pq.Array(kinds))),
			)
		}
	}
	return result
}

// Match cluster + namespaces for fine-grained RBAC.
// Resolves to:
// (( cluster = 'a' AND data->>'namespace' IN ['ns-1', 'ns-2', ...] )
// OR ( cluster = 'b' AND data->>'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
func matchClusterAndNamespace(clusterNamespacesMap map[string][]string) exp.ExpressionList {
	result := goqu.Or()
	clustersWithAllNamespaces := []string{}
	for cluster, namespaces := range clusterNamespacesMap {
		if len(namespaces) == 1 && namespaces[0] == "*" {
			clustersWithAllNamespaces = append(clustersWithAllNamespaces, cluster)
		} else {
			result = result.Append(
				goqu.And(
					goqu.C("cluster").Eq(cluster),
					goqu.L("data->???", "namespace", goqu.L("?|"), pq.Array(namespaces))),
			)
		}
	}
	if len(clustersWithAllNamespaces) > 0 {
		result = result.Append(goqu.C("cluster").In(clustersWithAllNamespaces))
	}
	return result
}

// Match Namespace objects.
// Resolves to:
// data?'apigroup' IS NOT TRUE) AND data->'kind'?'Namespace'
// AND (( cluster = 'a' AND data->>'name' IN ['ns-1', 'ns-2', ...] )
// OR ( cluster = 'b' AND data->>'name' IN ['ns-3', 'ns-4', ...] ) OR ...)
func matchNamespaceObject(clusterNamespacesMap map[string][]string) exp.ExpressionList {
	result := goqu.And(
		goqu.L("data??", goqu.L("?"), "apigroup").IsNotTrue(),
		goqu.L("data->???", "kind", goqu.L("?"), "Namespace"))

	match := goqu.Or()
	clustersWithAllNamespaces := []string{}

	for cluster, namespaces := range clusterNamespacesMap {
		if len(namespaces) == 1 && namespaces[0] == "*" {
			// Save cluster names to build the clause later.
			clustersWithAllNamespaces = append(clustersWithAllNamespaces, cluster)
		} else {
			// Match Namespaces by name.
			match = match.Append(goqu.And(
				goqu.C("cluster").Eq(cluster),
				goqu.L("data->???", "name", goqu.L("?|"), pq.Array(namespaces))),
			)
		}
	}
	if len(clustersWithAllNamespaces) > 0 {
		match = match.Append(
			goqu.C("cluster").In(clustersWithAllNamespaces),
		)
	}

	result = result.Append(match)

	return result
}
DELETE THIS COMMENTED CODE BEFORE MERGE. */

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

	if klog.V(4).Enabled() {
		logExpression("Fine-grained RBAC WHERE expression:\n", result)
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

	if klog.V(2).Enabled() {
		logExpression(fmt.Sprintf("UserPermission [%s]. Expression for [cluster AND namespace]:\n",
			userPermission.Name), result)
	}

	return result
}

// Builds the part of the query that matches the API group and kind of a resource.
// Resolves to:
// (( data->'apigroup' ? 'kubevirt.io' AND data->'kind_plural' IN ['VirtualMachine', 'VirtualMachineInstance', ...] )
// OR data->'apigroup' ? 'snapshots.kubevirt.io' OR ...)
// Cases:
//  1. both apigroup and kind have a wildcard (*).
//  2. both apigroup and kind are arrays of strings.
//  3. apigroup has a wilcard (*).
//  4. apigroup is empty string.
//  5. apigroup is an array of strings.
//  6. kind has a wildcard (*).
//  7. kind is an array of strings.
func matchApiGroupAndKind(userPermission clusterviewv1alpha1.UserPermission) exp.ExpressionList {
	result := goqu.Or()
	for _, rule := range userPermission.Status.ClusterRoleDefinition.Rules {
		// Rule must have the verb "list" or "*".
		if !slices.Contains(rule.Verbs, "list") && !slices.Contains(rule.Verbs, "*") {
			continue
		}

		// Check for wildcards
		wildcardAPIGroup := slices.Contains(rule.APIGroups, "*")
		wildcardKind := slices.Contains(rule.Resources, "*")

		// Match any resource (apigroup="*" and kind="*")
		if wildcardAPIGroup && wildcardKind {
			// Ignore all other rules because this rule matches any resource.
			result = goqu.Or(goqu.L("1=1"))
			return result
		}

		var apigroupExp exp.Expression
		if !wildcardAPIGroup {
			if len(rule.APIGroups) == 1 {
				apigroupExp = goqu.L("data->???", "apigroup", goqu.L("?"), rule.APIGroups[0])
			} else if len(rule.APIGroups) > 1 {
				apigroupExp = goqu.L("data->???", "apigroup", goqu.L("?|"), pq.Array(rule.APIGroups))
			}
		}

		var kindExp exp.Expression
		if !wildcardKind {
			// Remove sub-resources. Database only has resources.
			resources := make([]string, 0)
			for _, resource := range rule.Resources {
				if !strings.Contains(resource, "/") {
					resources = append(resources, resource)
				}
			}
			if len(resources) == 1 {
				kindExp = goqu.L("data->???", "kind_plural", goqu.L("?"), resources[0])
			} else if len(resources) > 1 {
				kindExp = goqu.L("data->???", "kind_plural", goqu.L("?|"), pq.Array(resources))
			}
		}

		// Combine expressions (apigroup AND kind).
		if wildcardAPIGroup && kindExp != nil {
			result = result.Append(kindExp)
		} else if wildcardKind && apigroupExp != nil {
			result = result.Append(apigroupExp)
		} else if apigroupExp != nil && kindExp != nil {
			result = result.Append(goqu.And(apigroupExp, kindExp))
		} else {
			klog.Warningf("Unexpected case building fine-grained RBAC apigroup and kind expressions: apigroupExp=%v, kindExp=%v", apigroupExp, kindExp)
		}
	}

	if klog.V(6).Enabled() {
		logExpression(fmt.Sprintf("UserPermission [%s]. Expression for [apigroup AND kind]:\n",
			userPermission.Name), result)
	}
	return result
}

func logExpression(message string, exp exp.Expression) {
	sql, _, err := goqu.From("t").Where(exp).ToSQL()
	if err != nil {
		klog.Errorf("Error logging expression: %v", err)
		return
	}
	sql = strings.ReplaceAll(sql, "SELECT * FROM \"t\" WHERE ", "")
	klog.V(1).Infof("%s %s", message, sql)
}
