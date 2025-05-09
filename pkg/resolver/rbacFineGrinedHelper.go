// Copyright Contributors to the Open Cluster Management project
package resolver

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
	"k8s.io/klog/v2"
)

// Match VirtualMachine resources using fine-grained RBAC.
// Resolves to:
// data->>'apigroup' = 'kubevirt.io' AND
//
//	(( cluster = 'a' AND data->>'namespace' IN ['ns-1', 'ns-2', ...] )
//	OR ( cluster = 'b' AND data->>'namespace' IN ['ns-3', 'ns-4', ...] ) OR ...)
func matchFineGrainedRbac(ns map[string][]string) exp.ExpressionList {
	// managed cluster + namespace

	groupKind := goqu.L("???", goqu.L(`data->?`, "apigroup"), goqu.Literal("?|"), pq.Array([]string{"kubevirt.io"}))

	var result exp.ExpressionList
	for key, val := range ns {
		namespaces := goqu.And(
			goqu.C("cluster").Eq(key),
			goqu.L("???", goqu.L(`data->?`, "namespace"), goqu.Literal("?|"), pq.Array(val)))

		result = goqu.Or(result, namespaces)
	}

	result = goqu.And(groupKind, result)

	klog.Info("Rbac query: ", result)

	return result
}
