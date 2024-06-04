package federated

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
