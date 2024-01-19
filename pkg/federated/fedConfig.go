// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"context"
	"fmt"
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
	// TODO: Cache the federated configuration result to reduce the overhead.
	return result
}

// Read the secret search-global-token on each managed hub namespace to get the route token and certificates.
func getFederationConfigFromSecret() []RemoteSearchService {
	result := []RemoteSearchService{}

	// Add the global-hub (self) first.
	client := config.KubeClient()
	secret, err := client.CoreV1().Secrets("open-cluster-management").Get(context.TODO(), "search-global-token", metav1.GetOptions{})

	if err != nil {
		klog.Errorf("Error getting secret list: %s", err)
	} else {
		result = append(result, RemoteSearchService{
			Name:  config.Cfg.GlobalHubName,
			URL:   "https://localhost:4010/searchapi/graphql",
			Token: string(secret.Data["token"]),
		})
	}

	if len(result) == 0 {
		klog.Warningf("Unable to search on global-hub resources. No secret found for search-global-token service account.")
	}

	// Add the managed hubs.
	dynamicClient := config.GetDynamicClient()
	gvr := schemav1.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}
	managedClusters, err := dynamicClient.Resource(gvr).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Error getting the managed clusters list: %s", err)
	} else {
		// Filter managed hubs.
		// A managed hub is a managed cluster that has the RHACM operator installed.
		// oc get mcl -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'
		for _, managedCluster := range managedClusters.Items {
			hubName := managedCluster.GetName()
			isManagedHub := false
			clusterClaims := managedCluster.UnstructuredContent()["status"].(map[string]interface{})["clusterClaims"].([]interface{})
			for _, clusterClaim := range clusterClaims {
				if clusterClaim.(map[string]interface{})["name"] == "hub.open-cluster-management.io" && clusterClaim.(map[string]interface{})["value"] != "NotInstalled" {
					isManagedHub = true
					break
				}
			}
			if !isManagedHub {
				klog.Infof("Skipping managed cluster [%s] because it is not a managed hub.", hubName)
				continue
			}

			// Get the search-api URL.
			hubUrl := managedCluster.UnstructuredContent()["spec"].(map[string]interface{})["managedClusterClientConfigs"].([]interface{})[0].(map[string]interface{})["url"].(string)
			url := strings.ReplaceAll(hubUrl, "https://api", "https://search-global-hub-open-cluster-management.apps")
			url = strings.ReplaceAll(url, ":6443", "/searchapi/graphql")

			// Get the search-api token.
			// TODO: Move to async function.
			secret, err := client.CoreV1().Secrets(hubName).Get(context.TODO(), "search-global", metav1.GetOptions{})
			if err != nil {
				klog.Errorf("Error getting token for managed hub [%s]: %s", hubName, err)
				continue
			}
			result = append(result, RemoteSearchService{
				Name:  hubName,
				URL:   url,
				Token: string(secret.Data["token"]),
				// TLSCert: string(secret.Data["ca.crt"]),
			})
		}
	}
	logFederationConfig(result)

	return result
}

func logFederationConfig(fedConfig []RemoteSearchService) {
	configStr := ""
	for _, service := range fedConfig {
		configStr += fmt.Sprintf("{ Name: %s URL: %s Token: [yes] TLSCert: [yes/no] }\n", service.Name, service.URL)
	}
	klog.Infof("Federation config:\n %s", configStr)
}
