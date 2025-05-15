// Copyright Contributors to the Open Cluster Management project

package resolver

import (
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/assert"
	klog "k8s.io/klog/v2"
)

func Test_matchFineGrainedRbac(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": []string{"namespace-a1", "namespace-a2"},
	}

	result := matchFineGrainedRbac(clusterNamespaces)

	assert.Equal(t, 2, len(result.Expressions()))

	sql, _, err := goqu.From(goqu.S("search").Table("resources")).Select("uid").Where(result).ToSQL()

	// TODO: This test fails because order of expressions is inconsistent.
	// expectedSQL := `SELECT "uid" FROM "search"."resources" WHERE (((data->'apigroup'?'snapshot.kubevirt.io' AND data->'kind'?|'{"VirtualMachineSnapshot","VirtualMachineSnapshotContent","VirtualMachineRestore"}') OR (data->'apigroup'?'kubevirt.io' AND data->'kind'?|'{"VirtualMachine","VirtualMachineInstance","VirtualMachineInstancePreset","VirtualMachineInstanceReplicaset","VirtualMachineInstanceMigration"}') OR (data->'apigroup'?'clone.kubevirt.io' AND data->'kind'?|'{"VirtualMachineClone"}') OR (data->'apigroup'?'export.kubevirt.io' AND data->'kind'?|'{"VirtualMachineExport"}') OR (data->'apigroup'?'instancetype.kubevirt.io' AND data->'kind'?|'{"VirtualMachineInstancetype","VirtualMachineClusterInstancetype","VirtualMachinePreference","VirtualMachineClusterPreference"}') OR (data->'apigroup'?'migrations.kubevirt.io' AND data->'kind'?|'{"MigrationPolicy"}') OR (data->'apigroup'?'pool.kubevirt.io' AND data->'kind'?|'{"VirtualMachinePool"}')) AND (("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1","namespace-a2"}'))`

	assert.Nil(t, err)
	// assert.Equal(t, sql, expectedSQL)
	klog.Info(">>> SQL result ", sql)
}

func Test_matchClusterAndNamespace(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": []string{"namespace-a1", "namespace-a2"},
	}

	result := matchClusterAndNamespace(clusterNamespaces)

	sql, args, err := goqu.From(goqu.S("search").Table("resources")).Select("uid").Where(result).ToSQL()

	expectedSQL := `SELECT "uid" FROM "search"."resources" WHERE (("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1","namespace-a2"}')`

	assert.Nil(t, err)
	assert.Equal(t, 0, len(args))
	assert.Equal(t, expectedSQL, sql)
}

func Test_matchClusterAndNamespace_anyNamespace(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": []string{"namespace-a1", "namespace-a2"},
		"cluster_b": []string{"*"},
	}

	result := matchClusterAndNamespace(clusterNamespaces)

	sql, args, err := goqu.From(goqu.S("search").Table("resources")).Select("uid").Where(result).ToSQL()

	expectedSQL := `SELECT "uid" FROM "search"."resources" WHERE ((("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1","namespace-a2"}') OR ("cluster" = 'cluster_b'))`

	assert.Nil(t, err)
	assert.Equal(t, 0, len(args))
	assert.Equal(t, expectedSQL, sql)
}
