package federated

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetFederatedConfig_fromCache(t *testing.T) {
	cachedFedConfig = fedConfigCache{
		lastUpdated: time.Now(),
		fedConfig: []RemoteSearchService{
			{
				Name:     "mock-cache-name",
				URL:      "https://mock-cache-url",
				Token:    "mock-cache-token",
				CABundle: []byte{},
			},
		},
	}

	mockRequest := &http.Request{}
	ctx := context.Background()
	result := getFederationConfig(ctx, mockRequest)

	assert.Equal(t, 1, len(result))
	assert.Equal(t, "mock-cache-name", result[0].Name)
	assert.Equal(t, "https://mock-cache-url", result[0].URL)
	assert.Equal(t, "mock-cache-token", result[0].Token)
}

func TestGetLocalSearchApiConfig(t *testing.T) {
	mockRequest := &http.Request{
		Header: map[string][]string{
			"Authorization": {"Bearer mock-token"},
		},
	}
	result := getLocalSearchApiConfig(mockRequest)

	assert.Equal(t, result.Name, "global-hub")
	assert.Equal(t, result.URL, "https://search-search-api.open-cluster-management.svc:4010/searchapi/graphql")
	assert.Equal(t, result.Token, "mock-token")
	assert.Equal(t, result.CABundle, []byte{})
}

// func TestGetFederationConfigFromSecret(t *testing.T) {
// 	mockRequest := &http.Request{
// 		Header: map[string][]string{
// 			"Authorization": {"Bearer mock-token"},
// 		},
// 	}
// 	ctx := context.Background()
// 	result := getFederationConfigFromSecret(ctx, mockRequest)

// 	t.Log("Result: ", result)
// 	assert.Equal(t, 2, len(result))
// }
