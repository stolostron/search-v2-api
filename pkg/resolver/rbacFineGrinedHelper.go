// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
)

// TODO
// Currently hardcoding the Kubevirt apigroup and kinds. The next iteration should read the
// ClusterRole resource to extract this information.
var kubevirtResourcesMap = map[string][]string{
	"kubevirt.io": {"VirtualMachine", "VirtualMachineInstance", "VirtualMachineInstancePreset",
		"VirtualMachineInstanceReplicaset", "VirtualMachineInstanceMigration"},
	"clone.kubevirt.io":  {"VirtualMachineClone"},
	"export.kubevirt.io": {"VirtualMachineExport"},
	"instancetype.kubevirt.io": {"VirtualMachineInstancetype", "VirtualMachineClusterInstancetype",
		"VirtualMachinePreference", "VirtualMachineClusterPreference"},
	"migrations.kubevirt.io": {"MigrationPolicy"},
	"pool.kubevirt.io":       {"VirtualMachinePool"},
	"snapshot.kubevirt.io":   {"VirtualMachineSnapshot", "VirtualMachineSnapshotContent", "VirtualMachineRestore"},
}

// Match VirtualMachine resources using fine-grained RBAC.
// Resolves to:
// data->>'apigroup' = 'kubevirt.io' AND data->>'kind' = VirtualMachine
//
//	(( cluster = 'a' AND data->>'namespace' IN ['ns-1', 'ns-2', ...] )
//	OR ( cluster = 'b' AND data->>'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
func matchFineGrainedRbac(clusterNamespacesMap map[string][]string) exp.ExpressionList {

	result := goqu.And(
		matchGroupKind(kubevirtResourcesMap),
		matchClusterAndNamespace(clusterNamespacesMap))

	// klog.Info("Fine-grained RBAC query: ", result)

	return result
}

// Match apiGroup + kind for fine-grained RBAC.
// Resolves to:
// ((data->>'apigroup' = 'kubevirt.io' AND data->>'kind' = VirtualMachine)
// OR data->>'apigroup' = 'snapshots.kubevirt.io' OR ...)
func matchGroupKind(groupKind map[string][]string) exp.ExpressionList {
	result := exp.NewExpressionList(exp.ExpressionListType(exp.OrType))

	for group, kinds := range groupKind {
		if len(kinds) == 0 {
			result = result.Append(goqu.L("???", goqu.L(`data->?`, "apigroup"), goqu.Literal("?"), group))
		} else {
			next := goqu.And(
				goqu.L("???", goqu.L(`data->?`, "apigroup"), goqu.Literal("?|"), pq.Array([]string{group})),
				goqu.L("???", goqu.L(`data->?`, "kind"), goqu.Literal("?|"), pq.Array(kinds)))
			result = result.Append(next)

		}
	}
	return result
}

// Match cluster + namespaces for  fine-grained RBAC.
// Resolves to:
// (( cluster = 'a' AND data->>'namespace' IN ['ns-1', 'ns-2', ...] )
// OR ( cluster = 'b' AND data->>'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
func matchClusterAndNamespace(clusterNamespacesMap map[string][]string) exp.ExpressionList {
	result := exp.NewExpressionList(exp.ExpressionListType(exp.OrType))

	for cluster, namespaces := range clusterNamespacesMap {
		if len(namespaces) == 1 && namespaces[0] == "Any" {
			result = result.Append(goqu.C("cluster").Eq(cluster))
		} else {
			next := goqu.And(
				goqu.C("cluster").Eq(cluster),
				goqu.L("???", goqu.L(`data->?`, "namespace"), goqu.Literal("?|"), pq.Array(namespaces)))

			result = result.Append(next)
		}
	}
	return result
}
