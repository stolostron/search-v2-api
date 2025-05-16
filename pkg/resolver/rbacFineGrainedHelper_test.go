// Copyright Contributors to the Open Cluster Management project

package resolver

import (
	"sort"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	ex "github.com/doug-martin/goqu/v9/exp"

	"github.com/stretchr/testify/assert"
)

func Test_matchFineGrainedRbac(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": []string{"namespace-a1", "namespace-a2"},
	}

	result := matchFineGrainedRbac(clusterNamespaces)

	// NOTE: The commented assertion would be much simpler, but the test fails intermittently because
	// the order of the expressions is inconsistent.
	//
	// sql, _, _ := goqu.From("test").Where(result).ToSQL()
	// expected := `SELECT * FROM test WHERE (((data->'apigroup'?'snapshot.kubevirt.io' AND data->'kind'?|'{"VirtualMachineSnapshot","VirtualMachineSnapshotContent","VirtualMachineRestore"}') OR (data->'apigroup'?'kubevirt.io' AND data->'kind'?|'{"VirtualMachine","VirtualMachineInstance","VirtualMachineInstancePreset","VirtualMachineInstanceReplicaset","VirtualMachineInstanceMigration"}') OR (data->'apigroup'?'clone.kubevirt.io' AND data->'kind'?|'{"VirtualMachineClone"}') OR (data->'apigroup'?'export.kubevirt.io' AND data->'kind'?|'{"VirtualMachineExport"}') OR (data->'apigroup'?'instancetype.kubevirt.io' AND data->'kind'?|'{"VirtualMachineInstancetype","VirtualMachineClusterInstancetype","VirtualMachinePreference","VirtualMachineClusterPreference"}') OR (data->'apigroup'?'migrations.kubevirt.io' AND data->'kind'?|'{"MigrationPolicy"}') OR (data->'apigroup'?'pool.kubevirt.io' AND data->'kind'?|'{"VirtualMachinePool"}')) AND (("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1","namespace-a2"}'))`
	// assert.Equal(t, sql, expected)

	expStrings := make([]string, 0)
	for _, exp := range result.Expressions() {
		expString := buildExpressionStringFrom((exp.Expression().(ex.ExpressionList)))
		expStrings = append(expStrings, expString)
	}

	sort.Strings(expStrings)
	expressionString := strings.Join(expStrings, " AND ")
	expectedExpression := `(("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1","namespace-a2"}') AND (data->'apigroup'?'clone.kubevirt.io' AND data->'kind'?|'{"VirtualMachineClone"}') OR (data->'apigroup'?'export.kubevirt.io' AND data->'kind'?|'{"VirtualMachineExport"}') OR (data->'apigroup'?'instancetype.kubevirt.io' AND data->'kind'?|'{"VirtualMachineClusterInstancetype","VirtualMachineClusterPreference","VirtualMachineInstancetype","VirtualMachinePreference"}') OR (data->'apigroup'?'kubevirt.io' AND data->'kind'?|'{"VirtualMachine","VirtualMachineInstance","VirtualMachineInstanceMigration","VirtualMachineInstancePreset","VirtualMachineInstanceReplicaset"}') OR (data->'apigroup'?'migrations.kubevirt.io' AND data->'kind'?|'{"MigrationPolicy"}') OR (data->'apigroup'?'pool.kubevirt.io' AND data->'kind'?|'{"VirtualMachinePool"}') OR (data->'apigroup'?'snapshot.kubevirt.io' AND data->'kind'?|'{"VirtualMachineRestore","VirtualMachineSnapshot","VirtualMachineSnapshotContent"}')`

	assert.Equal(t, expectedExpression, expressionString)
}

func Test_matchClusterAndNamespace(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": []string{"namespace-a1", "namespace-a2"},
	}

	result := matchClusterAndNamespace(clusterNamespaces)

	sql, args, err := goqu.From("t").Where(result).ToSQL()

	expectedSQL := `SELECT * FROM "t" WHERE (("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1","namespace-a2"}')`

	assert.Nil(t, err)
	assert.Equal(t, 0, len(args))
	assert.Equal(t, expectedSQL, sql)
}

func Test_matchClusterAndNamespace_anyNamespace(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": []string{"namespace-a1", "namespace-a2"},
		"cluster-b": []string{"*"},
	}

	result := matchClusterAndNamespace(clusterNamespaces)

	expressionString := buildExpressionStringFrom(result)

	expectedExpression := `("cluster" = 'cluster-b') OR (("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1","namespace-a2"}')`

	assert.Equal(t, expectedExpression, expressionString)
}

// NOTE: This is needed because goqu ToSQL() doesn't return the expressions in a consistent order.
func buildExpressionStringFrom(expressions ex.ExpressionList) string {
	expStrings := make([]string, 0)
	for _, exp := range expressions.Expressions() {
		sql, _, _ := goqu.From("t").Where(exp).ToSQL()
		expStrings = append(expStrings, sql[24:]) // Removes [SELECT * FROM "t" WHERE ]
	}
	sort.Strings(expStrings)
	return strings.Join(expStrings, " OR ") // Use exp.Type() to make generic.
}
