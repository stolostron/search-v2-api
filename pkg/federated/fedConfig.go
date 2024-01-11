// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"context"
	"strings"

	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schemav1 "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// Holds the data needed to connect to a remote search service.
type RemoteSearchService struct {
	Name    string
	URL     string
	Token   string
	TLSCert string
	TLSKey  string
}

func getFederationConfig() []RemoteSearchService {
	result := getFederationConfigFromSecret()
	return result
}

func getFederationConfigFromSecret() []RemoteSearchService {
	result := []RemoteSearchService{}

	// Add the global-hub
	// TODO: This needs to be more efficient.
	client := config.KubeClient()
	secretList, err := client.CoreV1().Secrets("open-cluster-management").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Error getting secret list: %s", err)
	}
	for _, secret := range secretList.Items {
		if strings.HasPrefix(secret.GetName(), "search-global-token") {
			result = append(result, RemoteSearchService{
				Name:  "global-hub", // TODO: Should this be configurable?
				URL:   "https://localhost:4010/searchapi/graphql",
				Token: string(secret.Data["token"]),
			})
		}
	}
	if len(result) == 0 {
		klog.Warningf("Unable to search on global-hub resources. No secret found for search-global-token service account.")
	}

	// Add the managed-hubs
	dynamicClient := config.GetDynamicClient()
	gvr := schemav1.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}
	managedClusters, err := dynamicClient.Resource(gvr).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Error getting managed clusters list: %s", err)
	} else {
		// TODO: Filter managed hubs only.
		for _, managedCluster := range managedClusters.Items {
			hubName := managedCluster.GetName()
			hubUrl := managedCluster.UnstructuredContent()["spec"].(map[string]interface{})["managedClusterClientConfigs"].([]interface{})[0].(map[string]interface{})["url"].(string)

			secret, err := client.CoreV1().Secrets(hubName).Get(context.TODO(), "search-global", metav1.GetOptions{})

			if err != nil {
				klog.Errorf("Error getting token for managed hub [%s]: %s", hubName, err)
				continue
			}
			url := strings.ReplaceAll(hubUrl, "https://api", "https://search-global-hub-open-cluster-management.apps")
			url = strings.ReplaceAll(url, ":6443", "/searchapi/graphql")
			result = append(result, RemoteSearchService{
				Name:  hubName,
				URL:   url,
				Token: string(secret.Data["token"]),
				// TLSCert: string(secret.Data["ca.crt"]),
			})
		}
	}
	klog.Infof("Federation config:\n %+v", result)

	return result
}
