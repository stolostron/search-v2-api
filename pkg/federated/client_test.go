package federated

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetHttpClient(t *testing.T) {

	c := GetHttpClient()

	assert.NotNil(t, c)
}
