// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	Name    string
	URL     string
	Token   string
	Version string
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
	getKubeClient    = config.KubeClient       // Allows for mocking client in tests.
	getDynamicClient = config.GetDynamicClient // Allows for mocking client in tests.
	routesGvr        = schemav1.GroupVersionResource{
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

func getLocalSearchApiConfig(ctx context.Context, request *http.Request) RemoteSearchService {
	url := fmt.Sprintf("https://search-search-api.%s.svc:4010/searchapi/graphql", config.Cfg.PodNamespace)

	if config.Cfg.DevelopmentMode {
		klog.Warningf("Running in DevelopmentMode. Using local self-signed certificate.")
		url = "https://localhost:4010/searchapi/graphql"
		tlsCert, err := os.ReadFile("sslcert/tls.crt") // Local self-signed certificate.
		if err != nil {
			klog.Errorf("Error reading local self-signed certificate: %s", err)
			klog.Info("Use 'make setup' to generate the local self-signed certificate.")
		}
		ok := tr.TLSClientConfig.RootCAs.AppendCertsFromPEM(tlsCert)
		if !ok {
			klog.Errorf("Error appending local self-signed CA bundle.")
		}
	} else {
		client := getKubeClient()
		caBundleConfigMap, err := client.CoreV1().ConfigMaps(config.Cfg.PodNamespace).
			Get(ctx, "search-ca-crt", metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Error getting the search-ca-crt configmap: %s", err)
		}
		ok := tr.TLSClientConfig.RootCAs.AppendCertsFromPEM([]byte(caBundleConfigMap.Data["service-ca.crt"]))
		if !ok {
			klog.Errorf("Error appending CA bundle for local service.")
		}
	}

	return RemoteSearchService{
		Name:  config.Cfg.Federation.GlobalHubName,
		URL:   url,
		Token: strings.ReplaceAll(request.Header.Get("Authorization"), "Bearer ", ""),
	}
}

// Builds a map of managed hub name to ACM install namespace.
// Use the local search-api to query the MultiClusterHub resource on the managed hubs.
// The MultiClusterHub resource namespace is the namespace where ACM is installed.
func getACMInstallNamespaces(localService RemoteSearchService) map[string]string {
	result := map[string]string{}
	client := httpClientGetter()

	// Build request to local search-api.
	reqBody := []byte(`{
		"query":"query Search ($input: [SearchInput]) { searchResult: search (input: $input) { items }}",
		"variables":{"input":[{"keywords":[],"filters":[{"property":"kind","values":["MultiClusterHub"]}] }]}}`)
	req, err := http.NewRequest("POST", localService.URL, bytes.NewBuffer(reqBody))
	if err != nil {
		klog.Errorf("Error creating request to get ACM install namespaces: %s", err)
		return result
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", localService.Token))
	req.Header.Set("Content-Type", "application/json")

	// Send the request to the local search-api.
	resp, err := client.Do(req)
	if err != nil {
		klog.Errorf("Error getting ACM install namespaces: %s", err)
		return result
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Read the response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.Errorf("Error reading response body: %s", err)
		return result
	}

	// Unmarshal the response.
	response := GraphQLPayload{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		klog.Errorf("Error unmarshalling search result: %s", err)
	}

	// Build the map of managed hub name to ACM install namespace.
	for _, item := range response.Data.Search[0].Items {
		result[item["cluster"].(string)] = item["namespace"].(string)
	}

	return result
}

// The kube-root-ca.crt has the CA bundle to verify the TLS connection to the cluster-proxy-user route in the global hub.
func addCABundleForClusterProxy(ctx context.Context) {
	client := getKubeClient()

	kubeRootCA, err := client.CoreV1().ConfigMaps("openshift-service-ca").Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error getting the kube-root-ca.crt: %s", err)
	}
	ok := tr.TLSClientConfig.RootCAs.AppendCertsFromPEM([]byte(kubeRootCA.Data["ca.crt"]))
	if !ok {
		klog.Error("Error appending CA bundle for remote client (cluster-proxy).")
	}
}

// Build the list of managed hubs the URL and token to access.
// The token to access each managed hub is created by the managedserviceaccount and saved in the
// secret search-global-token on each managed huc namespace.
func getFederationConfigFromSecret(ctx context.Context, request *http.Request) []RemoteSearchService {
	result := []RemoteSearchService{}
	resultLock := sync.Mutex{}
	wg := sync.WaitGroup{}
	client := getKubeClient()
	dynamicClient := getDynamicClient()

	// Add the local search-api on the global hub.
	localSearchApi := getLocalSearchApiConfig(ctx, request)
	result = append(result, localSearchApi)

	// Get the namespace where ACM is installed in each managed hub.
	acmInstallNamespaceMap := getACMInstallNamespaces(localSearchApi)

	// Get the cluster-proxy-user route on the global hub. We use this to proxy the requests to the managed hubs.
	routes, err := dynamicClient.Resource(routesGvr).List(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=cluster-proxy-addon-user",
	})
	if err != nil {
		klog.Errorf("Error getting the routes list: %s", err)
	}
	clusterProxyRoute := routes.Items[0].UnstructuredContent()["spec"].(map[string]interface{})["host"].(string)

	// Add the CA bundle for the cluster-proxy-user route.
	addCABundleForClusterProxy(ctx)

	// Build the list of managed hubs.
	managedClusters, err := dynamicClient.Resource(managedClustersGvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Error getting the managed clusters list: %s", err)
	} else {
		// Filter managed hubs. A managed hub is a managed cluster that has the RHACM operator installed.
		// oc get mcl -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'
		for _, managedCluster := range managedClusters.Items {
			hubName := managedCluster.GetName()
			isManagedHub := false
			// skip the local cluster
			if managedCluster.GetLabels() != nil && managedCluster.GetLabels()["local-cluster"] == "true" {
				klog.V(5).Info("Skipping local cluster.", "name", managedCluster.GetName())
				continue
			}
			// store ACM version info
			version := "unknown"
			clusterClaims := managedCluster.UnstructuredContent()["status"].(map[string]interface{})["clusterClaims"].([]interface{})
			for _, clusterClaim := range clusterClaims {
				if clusterClaim.(map[string]interface{})["name"] == "hub.open-cluster-management.io" && clusterClaim.(map[string]interface{})["value"] != "NotInstalled" {
					isManagedHub = true
				}
				if clusterClaim.(map[string]interface{})["name"] == "version.open-cluster-management.io" {
					version = clusterClaim.(map[string]interface{})["value"].(string)
				}
			}
			if !isManagedHub {
				klog.V(5).Infof("Managed cluster [%s] is not a managed hub.	Skipping.", hubName)
				continue
			}

			// Get the token to access the managed hub.
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

				result = append(result, RemoteSearchService{
					Name: hubName,
					URL: fmt.Sprintf(
						"https://%s/%s/api/v1/namespaces/%s/services/search-search-api:4010/proxy-service/searchapi/graphql",
						clusterProxyRoute, hubName, acmInstallNamespaceMap[hubName]),
					Token:   string(secret.Data["token"]),
					Version: version,
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
		configStr += fmt.Sprintf("{ Name: %s , URL: %s, Version: %s }\n", service.Name, service.URL, service.Version)
	}
	klog.Infof("Using federation config:\n %s", configStr)
}
