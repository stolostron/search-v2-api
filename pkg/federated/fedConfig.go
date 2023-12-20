// Copyright Contributors to the Open Cluster Management project
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
	name1, nameExists := os.LookupEnv("NAME1")
	api1, apiExists := os.LookupEnv("API1")
	token1, tokenExists := os.LookupEnv("TOKEN1")
	if !apiExists || !tokenExists {
		klog.Errorf("Error reading API1 and TOKEN1 from environment. API1: %s TOKEN1:%s", api1, token1)
		return nil
	}
	if !nameExists {
		name1 = "default-hub-name"
	}
	remoteServices := []RemoteSearchService{
		{
			Name:  name1,
			URL:   api1,
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
