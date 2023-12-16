package federated

import (
	"os"

	"k8s.io/klog/v2"
)

// TODO: Read the federation config from secret.

// Holds the data needed to connect to a remote search service.
type RemoteSearchService struct {
	Name  string
	URL   string
	Token string
	// ClientCertificate
	// ClientKey
	// CA
}

func getFederationConfig() []RemoteSearchService {
	token1, exists := os.LookupEnv("TOKEN1")
	if !exists {
		klog.Errorf("Error reading TOKEN1 from environment. %+v %s", exists, token1)
		return nil
	}
	remoteServices := []RemoteSearchService{
		{
			Name:  "jorge-mh-a",
			URL:   "https://search-api-open-cluster-management.apps.sno-413-tnlvk.dev07.red-chesterfield.com/searchapi/graphql",
			Token: token1,
		},
	}

	token2, t2exists := os.LookupEnv("TOKEN2")
	if t2exists {
		remoteServices = append(remoteServices, RemoteSearchService{
			Name:  "jorge-mh-b",
			URL:   "https://search-api-open-cluster-management.apps.sno-413-fnrz8.dev07.red-chesterfield.com/searchapi/graphql",
			Token: token2,
		})
	}

	return remoteServices
}
