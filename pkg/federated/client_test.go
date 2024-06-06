package federated

import (
	"testing"

	"github.com/stretchr/testify/assert"
)


func TestGetHttpClient(t *testing.T) {
	mockService := RemoteSearchService{
		Name: "test",
	}

	c := GetHttpClient(mockService)

	assert.NotNil(t, c)
}
