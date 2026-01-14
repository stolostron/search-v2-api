// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

// Data needed to process a federated request.
type FederatedRequest struct {
	InRequestBody []byte
	Response      GraphQLPayload
}

var getFedConfig = getFederationConfig

func HandleFederatedRequest(w http.ResponseWriter, r *http.Request) {
	klog.V(1).Info("Received federated search request.")
	ctx := r.Context()
	receivedBody, err := io.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("Error reading federated request body: %s", err)
		sendResponse(w, &GraphQLPayload{
			Data:   Data{},
			Errors: []string{fmt.Errorf("error reading federated request body: %s", err).Error()},
		})
		return
	}
	klog.V(3).Infof("Federated search query: %s", string(receivedBody))

	fedRequest := FederatedRequest{
		InRequestBody: receivedBody,
		Response: GraphQLPayload{
			Data:   Data{},
			Errors: []string{},
		},
	}

	managedHubValues := retrieveManagedHubValues(receivedBody)
	klog.V(3).Infof("ManagedHub filter values in request: %v", managedHubValues)

	fedConfig := getFedConfig(ctx, r)
	numberOfRequests := 0
	wg := sync.WaitGroup{}
	for _, remoteService := range fedConfig {
		// If managedHubValues is not empty, check if remoteService.Name is in the list.
		if len(managedHubValues) > 0 {
			found := false
			for _, hub := range managedHubValues {
				if hub == remoteService.Name {
					found = true
					break
				}
			}
			if !found {
				klog.V(3).Infof("Skipping remote service %s as it's not in the managedHub filter.", remoteService.Name)
				continue
			}
		}
		numberOfRequests++
		wg.Add(1)
		go func(remoteService RemoteSearchService) {
			defer wg.Done()
			// Get the http client from pool.
			client := httpClientGetter()
			fedRequest.getFederatedResponse(remoteService, receivedBody, client)
			httpClientPool.Put(client) // Put the client back into the pool for reuse.
		}(remoteService)
	}
	klog.V(3).Infof("Sent %d federated requests, waiting for response.", numberOfRequests)
	wg.Wait()

	// Send JSON response to client.
	sendResponse(w, &fedRequest.Response)
}

// retrieveManagedHubValues retrieves the managedHub values from the receivedBody of the federated request.
func retrieveManagedHubValues(receivedBody []byte) []string {
	var reqBodyMap map[string]interface{}
	if err := json.Unmarshal(receivedBody, &reqBodyMap); err != nil {
		klog.Errorf("Error unmarshaling federated request body: %s", err)
		return []string{}
	}

	variables, ok := reqBodyMap["variables"].(map[string]interface{})
	if !ok {
		return []string{}
	}

	return extractManagedHubValues(extractFiltersFromVariables(variables))
}

// extractFiltersFromVariables extracts the filters list from variables using multiple paths.
func extractFiltersFromVariables(variables map[string]interface{}) []interface{} {
	// Try path 1: variables.query.filters (for searchSchema)
	if filters := getFiltersFromQuery(variables); filters != nil {
		return filters
	}

	// Try path 2: variables.input[0].filters (for searchResultItems)
	return getFiltersFromInput(variables)
}

// getFiltersFromQuery extracts filters from variables.query.filters path.
func getFiltersFromQuery(variables map[string]interface{}) []interface{} {
	query, ok := variables["query"].(map[string]interface{})
	if !ok {
		return nil
	}

	filters, ok := query["filters"].([]interface{})
	if !ok {
		return nil
	}

	return filters
}

// getFiltersFromInput extracts filters from variables.input[0].filters path.
func getFiltersFromInput(variables map[string]interface{}) []interface{} {
	input, ok := variables["input"].([]interface{})
	if !ok || len(input) == 0 {
		return nil
	}

	inputMap, ok := input[0].(map[string]interface{})
	if !ok {
		return nil
	}

	filters, ok := inputMap["filters"].([]interface{})
	if !ok {
		return nil
	}

	return filters
}

// extractManagedHubValues extracts managedHub values from a filters list.
func extractManagedHubValues(filtersList []interface{}) []string {
	managedHubValues := []string{}

	for _, filter := range filtersList {
		filterMap, ok := filter.(map[string]interface{})
		if !ok {
			continue
		}

		property, ok := filterMap["property"].(string)
		if !ok || property != "managedHub" {
			continue
		}

		values := extractStringValues(filterMap["values"])
		managedHubValues = append(managedHubValues, values...)
	}

	return managedHubValues
}

// extractStringValues extracts string values from an interface{} slice.
func extractStringValues(valuesInterface interface{}) []string {
	values, ok := valuesInterface.([]interface{})
	if !ok {
		return []string{}
	}

	stringValues := []string{}
	for _, value := range values {
		strValue, ok := value.(string)
		if ok {
			stringValues = append(stringValues, strValue)
		}
	}

	return stringValues
}

// Send GraphQL/JSON response to client.
func sendResponse(w http.ResponseWriter, response *GraphQLPayload) {
	w.Header().Set("Content-Type", "application/json")

	result := json.NewEncoder(w).Encode(response)
	if result != nil {
		klog.Errorf("Error encoding federated response: %s", result)
	}
	klog.V(3).Info("Responded to federated request.")
}

// modifyRequestBodyForVersion modifies the request body to support older API versions.
// For versions containing "2.13", it removes the query parameter from the GraphQL query string
// in searchSchema requests since older versions don't support filtering searchSchema results.
func modifyRequestBodyForVersion(remoteService RemoteSearchService, receivedBody []byte) []byte {
	// Check if the version includes "2.13"
	if !strings.HasPrefix(remoteService.Version, "2.13") {
		klog.V(5).Infof("Version %s doesn't require modification, using original request body", remoteService.Version)
		return receivedBody
	}

	klog.V(3).Infof("Modifying request body for version %s (2.13 detected)", remoteService.Version)

	// Parse the request body
	var reqBodyMap map[string]interface{}
	if err := json.Unmarshal(receivedBody, &reqBodyMap); err != nil {
		klog.Errorf("Error unmarshaling request body for version modification: %s", err)
		return receivedBody
	}

	// Check if this is a searchSchema query
	operationName, ok := reqBodyMap["operationName"].(string)
	if !ok || operationName != "searchSchema" {
		klog.V(5).Info("Not a searchSchema query, no modification needed")
		return receivedBody
	}

	// Modify the GraphQL query string to remove the $query parameter
	if queryStr, ok := reqBodyMap["query"].(string); ok {
		// Replace the parameterized query with a simple query without parameters
		// From: "query searchSchema($query: SearchInput) { searchSchema(query: $query) }"
		// To: "query searchSchema { searchSchema }"
		if strings.Contains(queryStr, "$query") {
			klog.V(3).Info("Removing $query parameter from GraphQL query string for 2.13 compatibility")
			// Simple replacement for searchSchema query
			modifiedQuery := strings.ReplaceAll(queryStr, "($query: SearchInput)", "")
			modifiedQuery = strings.ReplaceAll(modifiedQuery, "(query: $query)", "")
			reqBodyMap["query"] = modifiedQuery
		}
	}

	// Marshal the modified body back to JSON
	modifiedBody, err := json.Marshal(reqBodyMap)
	if err != nil {
		klog.Errorf("Error marshaling modified request body: %s", err)
		return receivedBody
	}

	klog.V(3).Infof("Modified request body for 2.13: %s", string(modifiedBody))
	return modifiedBody
}

func (fedRequest *FederatedRequest) getFederatedResponse(remoteService RemoteSearchService,
	receivedBody []byte, client HTTPClient) {

	// modify the receivedBody to support older versions of search-api
	modifiedBody := modifyRequestBodyForVersion(remoteService, receivedBody)

	// Create the request.
	req, err := http.NewRequest("POST", remoteService.URL, bytes.NewBuffer(modifiedBody))
	if err != nil {
		klog.Errorf("Error creating federated request: %s", err)
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("error creating federated request: %s", err).Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", remoteService.Token))

	// Send the request.
	resp, err := client.Do(req)
	if err != nil {
		klog.Errorf("Error sending federated request: %s", err)
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("error sending federated request: %s", err).Error())
		return
	}

	// Read and process the response.
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.Errorf("Error reading federated response from %s: %s", remoteService.Name, err)
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("error reading federated response body: %s", err).Error())
		return
	}

	klog.V(3).Infof("Received response from %s:\n%s", remoteService.Name, string(body))
	parseResponse(fedRequest, body, remoteService.Name)
}
