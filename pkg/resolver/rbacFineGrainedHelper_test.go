// Copyright Contributors to the Open Cluster Management project

package resolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_matchGroupKind(t *testing.T) {

	clusterNamespaces := map[string][]string{
		"cluster-a": []string{"namespace-a1", "namespace-a2"},
	}

	result := matchFineGrainedRbac(clusterNamespaces)

	assert.Equal(t, 2, len(result.Expressions()))

	// TODO: Parse this exp list to a query and validate.

}
