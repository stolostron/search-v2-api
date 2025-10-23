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
		"cluster-a": {"namespace-a1", "namespace-a2"},
	}

	result := matchFineGrainedRbac(clusterNamespaces)

	// matchFineGrainedRbac returns OR at top level with two branches:
	// 1. Namespace objects (matchNamespaceObject)
	// 2. Namespaced resources (AND of matchGroupKind and matchClusterAndNamespace)

	assert.Equal(t, 2, len(result.Expressions()), "Should have 2 top-level OR expressions")

	// We can't easily verify the exact SQL string due to inconsistent ordering,
	// but we can verify the structure is correct by checking it produces valid SQL
	sql, _, err := goqu.From("test").Where(result).ToSQL()
	assert.Nil(t, err)
	assert.Contains(t, sql, "data?'apigroup' IS NOT TRUE")
	assert.Contains(t, sql, "data->'kind'?'Namespace'")
	assert.Contains(t, sql, "data->'name'?|")
	assert.Contains(t, sql, "data->'namespace'?|")
	assert.Contains(t, sql, "kubevirt.io")
}

func Test_matchClusterAndNamespace(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": {"namespace-a1", "namespace-a2"},
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
		"cluster-a": {"namespace-a1", "namespace-a2"},
		"cluster-b": {"*"},
	}

	result := matchClusterAndNamespace(clusterNamespaces)

	expressionString := buildExpressionStringFrom(result)

	expectedExpression := `("cluster" IN ('cluster-b')) OR (("cluster" = 'cluster-a') AND data->'namespace'?|'{"namespace-a1","namespace-a2"}')`

	assert.Equal(t, expectedExpression, expressionString)
}

func Test_matchNamespaceObject(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": {"namespace-a1", "namespace-a2"},
	}

	result := matchNamespaceObject(clusterNamespaces)

	sql, args, err := goqu.From("t").Where(result).ToSQL()

	expectedSQL := `SELECT * FROM "t" WHERE ((data?'apigroup' IS NOT TRUE) AND data->'kind'?'Namespace' AND (("cluster" = 'cluster-a') AND data->'name'?|'{"namespace-a1","namespace-a2"}'))`

	assert.Nil(t, err)
	assert.Equal(t, 0, len(args))
	assert.Equal(t, expectedSQL, sql)
}

func Test_matchNamespaceObject_anyNamespace(t *testing.T) {
	clusterNamespaces := map[string][]string{
		"cluster-a": {"namespace-a1", "namespace-a2"},
		"cluster-b": {"*"},
		"cluster-c": {"*"},
	}

	result := matchNamespaceObject(clusterNamespaces)

	sql, args, err := goqu.From("t").Where(result).ToSQL()

	assert.Nil(t, err)
	assert.Equal(t, 0, len(args))

	// Should contain the basic Namespace matching
	assert.Contains(t, sql, "data?'apigroup' IS NOT TRUE")
	assert.Contains(t, sql, "data->'kind'?'Namespace'")

	// Should match cluster-a with specific namespaces
	assert.Contains(t, sql, "cluster-a")
	assert.Contains(t, sql, "data->'name'?|")

	// Should match clusters with all namespaces using IN clause
	assert.Contains(t, sql, "\"cluster\" IN ('cluster-")
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
