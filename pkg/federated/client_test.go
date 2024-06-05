package federated

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLocalHttpClient(t *testing.T) {

	c := getLocalHttpClient()

	assert.NotNil(t, c)
}

func TestGetHttpClient(t *testing.T) {
	mockService := RemoteSearchService{
		Name: "test",
	}

	c := GetHttpClient(mockService)

	assert.NotNil(t, c)
}
