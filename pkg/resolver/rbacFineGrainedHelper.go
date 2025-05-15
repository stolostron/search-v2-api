// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
)

// TODO: Allow to expand this hard-coded list with an env variable.
// Currently hardcoding the Kubevirt apigroup and kinds. A future iteration should update this
// to read the ClusterRole resource to extract this information.
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

// Match resources using fine-grained RBAC.
// Resolves to:
// data->'apigroup' ? 'kubevirt.io' AND data->>'kind' ? 'VirtualMachine' OR ...
//	AND (( cluster = 'a' AND data->'namespace' IN ['ns-1', 'ns-2', ...] )
//	OR ( cluster = 'b' AND data->'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
func matchFineGrainedRbac(clusterNamespacesMap map[string][]string) exp.ExpressionList {
	result := goqu.And(
		matchGroupKind(kubevirtResourcesMap),
		matchClusterAndNamespace(clusterNamespacesMap))

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
	result := exp.NewExpressionList(exp.ExpressionListType(exp.OrType))

	for cluster, namespaces := range clusterNamespacesMap {
		// TODO: Pending PR to change any to *
		// https://github.com/stolostron/multicloud-operators-foundation/pull/980
		if len(namespaces) == 1 && (namespaces[0] == "any" || namespaces[0] == "*") {
			result = result.Append(goqu.C("cluster").Eq(cluster))
		} else {
			result = result.Append(
				goqu.And(
					goqu.C("cluster").Eq(cluster),
					goqu.L("data->???", "namespace", goqu.L("?|"), pq.Array(namespaces))),
			)
		}
	}
	return result
}
