// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schemav1 "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

// Holds the data needed to connect to a remote search service.
type RemoteSearchService struct {
	Name     string
	URL      string
	Token    string
	CABundle []byte
}

type fedConfigCache struct {
	lastUpdated time.Time
	fedConfig   []RemoteSearchService
}

var cachedFedConfig = fedConfigCache{
	lastUpdated: time.Time{},
	fedConfig:   []RemoteSearchService{},
}

func getFederationConfig(ctx context.Context, request *http.Request) []RemoteSearchService {
	cacheDuration := time.Duration(config.Cfg.Federation.ConfigCacheTTL) * time.Millisecond
	if cachedFedConfig.lastUpdated.IsZero() || cachedFedConfig.lastUpdated.Add(cacheDuration).Before(time.Now()) {
		klog.Infof("Refreshing federation config.")
		cachedFedConfig.fedConfig = getFederationConfigFromSecret(ctx)
		cachedFedConfig.lastUpdated = time.Now()
	} else {
		klog.Infof("Using cached federation config.")
	}

	client := config.KubeClient()
	ca, err := client.CoreV1().ConfigMaps("open-cluster-management").Get(ctx, "search-ca-crt", metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error getting search-ca-crt: %s", err)
	}
	// Add the global-hub (self) first.
	local := RemoteSearchService{
		Name:     config.Cfg.Federation.GlobalHubName,
		URL:      "https://localhost:4010/searchapi/graphql",
		Token:    strings.ReplaceAll(request.Header.Get("Authorization"), "Bearer ", ""),
		CABundle: []byte(ca.Data["service-ca.crt"]),
	}

	result := append(cachedFedConfig.fedConfig, local)
	return result
}

// Read the secret search-global-token on each managed hub namespace to get the route token and certificates.
func getFederationConfigFromSecret(ctx context.Context) []RemoteSearchService {
	result := []RemoteSearchService{}
	resultLock := sync.Mutex{}
	wg := sync.WaitGroup{}

	// Add the managed hubs.
	client := config.KubeClient()
	dynamicClient := config.GetDynamicClient()
	gvr := schemav1.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}
	managedClusters, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
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

			// Using cluster-proxy on global hub to access the search-api on managed hubs.
			searchApiURL := "https://cluster-proxy-user.apps.sno-4xlarge-414-6rqfc.dev07.red-chesterfield.com/" // FIXME: get from cluster-proxy route.

			// Get the ManagedServiceAccount token and ca.crt.
			wg.Add(1)
			go func(hubName string) {
				defer wg.Done()
				secret, err := client.CoreV1().Secrets(hubName).Get(ctx, "search-global", metav1.GetOptions{})
				if err != nil {
					klog.Errorf("Error getting token for managed hub [%s]: %s", hubName, err)
					return
				}
				resultLock.Lock()
				defer resultLock.Unlock()

				if err != nil {
					klog.Errorf("Error decoding ca.crt for managed hub [%s]: %s", hubName, err)
					return
				}
				result = append(result, RemoteSearchService{
					Name:     hubName,
					URL:      searchApiURL + hubName + "/api/v1/namespaces/open-cluster-management/services/search-search-api:4010/proxy-service/searchapi/graphql",
					Token:    string(secret.Data["token"]),
					CABundle: secret.Data["ca.crt"],
				})
			}(hubName)
		}
	}
	wg.Wait() // Wait for all managed hub configs to be retrieved.
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
