// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"os"

	"k8s.io/klog/v2"
)

// TODO: Read the federation config from secret.

// Holds the data needed to connect to a remote search service.
type RemoteSearchService struct {
	Name    string
	URL     string
	Token   string
	TLSCert string
	TLSKey  string
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
		name1 = "default-hub-1"
	}
	remoteServices := []RemoteSearchService{
		{
			Name:  name1,
			URL:   api1,
			Token: token1,
			// TLSCert: tlsCert1,
			// TLSKey: tlsKey1,
		},
	}
	tlsCert1, tlsCertExists := os.LookupEnv("TLS_CRT1")
	tlsKey1, tlsKeyExists := os.LookupEnv("TLS_KEY1")
	if tlsCertExists && tlsKeyExists {
		remoteServices[0].TLSCert = tlsCert1
		remoteServices[0].TLSKey = tlsKey1
	}

	name2, name2exists := os.LookupEnv("NAME2")
	api2, api2exists := os.LookupEnv("API2")
	token2, token2exists := os.LookupEnv("TOKEN2")
	if !name2exists {
		name2 = "default-hub-2"
	}
	if api2exists && token2exists {
		remoteServices = append(remoteServices, RemoteSearchService{
			Name:  name2,
			URL:   api2,
			Token: token2,
			// TLSCert: tlsCert2,
			// TLSKey: tlsKey2,
		})
	}
	tlsCert2, tlsCert2Exists := os.LookupEnv("TLS_CRT2")
	tlsKey2, tlsKey2Exists := os.LookupEnv("TLS_KEY2")
	if tlsCert2Exists && tlsKey2Exists {
		remoteServices[1].TLSCert = tlsCert2
		remoteServices[1].TLSKey = tlsKey2
	}

	return remoteServices
}
