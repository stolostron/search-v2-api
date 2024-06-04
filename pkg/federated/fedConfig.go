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

var (
	routesGvr = schemav1.GroupVersionResource{
		Group:    "route.openshift.io",
		Version:  "v1",
		Resource: "routes",
	}
	managedClustersGvr = schemav1.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}
)

func getFederationConfig(ctx context.Context, request *http.Request) []RemoteSearchService {
	cacheDuration := time.Duration(config.Cfg.Federation.ConfigCacheTTL) * time.Millisecond
	if cachedFedConfig.lastUpdated.IsZero() || cachedFedConfig.lastUpdated.Add(cacheDuration).Before(time.Now()) {
		klog.Infof("Refreshing federation config.")
		cachedFedConfig.fedConfig = getFederationConfigFromSecret(ctx, request)
		cachedFedConfig.lastUpdated = time.Now()
	} else {
		klog.Infof("Using cached federation config.")
	}

	logFederationConfig(cachedFedConfig.fedConfig)
	return cachedFedConfig.fedConfig
}

func getLocalSearchApiConfig(request *http.Request) RemoteSearchService {
	namespace := "open-cluster-management" // FIXME: Namespace.
	url := fmt.Sprintf("https://search-search-api.%s.svc:4010/searchapi/graphql", namespace)

	if config.Cfg.DevelopmentMode {
		klog.Warningf("Running in DevelopmentMode. Using local self-signed certificate.")
		url = "https://localhost:4010/searchapi/graphql"
	}

	return RemoteSearchService{
		Name:     config.Cfg.Federation.GlobalHubName,
		URL:      url,
		Token:    strings.ReplaceAll(request.Header.Get("Authorization"), "Bearer ", ""),
		CABundle: []byte{}, // addded later in client.go
	}
}

// Read the secret search-global-token on each managed hub namespace to get the token and certificates.
func getFederationConfigFromSecret(ctx context.Context, request *http.Request) []RemoteSearchService {
	result := []RemoteSearchService{}
	resultLock := sync.Mutex{}
	wg := sync.WaitGroup{}

	// Add the local search-api on the global hub.
	result = append(result, getLocalSearchApiConfig(request))

	// Add the managed hubs.
	client := config.KubeClient()
	dynamicClient := config.GetDynamicClient()

	// The kube-root-ca.crt has the CA bundle to verify the TLS connection to the cluster-proxy-user route in the global hub.
	kubeRootCA, err := client.CoreV1().ConfigMaps("openshift-service-ca").Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error getting the kube-root-ca.crt: %s", err)
	}

	// Get the cluster-proxy-user route on the global hub. We use this to proxy the requests to the managed hubs.
	routes, err := dynamicClient.Resource(routesGvr).List(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=cluster-proxy-addon-user",
	})
	if err != nil {
		klog.Errorf("Error getting the routes list: %s", err)
	}
	clusterProxyRoute := routes.Items[0].UnstructuredContent()["spec"].(map[string]interface{})["host"].(string)

	managedClusters, err := dynamicClient.Resource(managedClustersGvr).List(ctx, metav1.ListOptions{})
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
				klog.V(5).Infof("Managed cluster [%s] is not a managed hub.	Skipping.", hubName)
				continue
			}

			// Using cluster-proxy on global hub to access the search-api on managed hubs.
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

				namespace := "open-cluster-management" // FIXME: namespace is hardcoded to open-cluster-management.
				result = append(result, RemoteSearchService{
					Name: hubName,
					URL: fmt.Sprintf(
						"https://%s/%s/api/v1/namespaces/%s/services/search-search-api:4010/proxy-service/searchapi/graphql",
						clusterProxyRoute, hubName, namespace),
					Token:    string(secret.Data["token"]),
					CABundle: []byte(kubeRootCA.Data["ca.crt"]),
				})
			}(hubName)
		}
	}
	wg.Wait() // Wait for all managed hub configs to be retrieved.

	return result
}

func logFederationConfig(fedConfig []RemoteSearchService) {
	configStr := ""
	for _, service := range fedConfig {
		configStr += fmt.Sprintf("{ Name: %s , URL: %s }\n", service.Name, service.URL)
	}
	klog.Infof("Federation config:\n %s", configStr)
}
