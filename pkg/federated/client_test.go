package federated

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLocalHttpClient(t *testing.T) {

	c := getLocalHttpClient()

	assert.NotNil(t, c)
}
