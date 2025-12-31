// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	// I want to get the values of managedhub from receivedBody and
	// check if the remoteService.Name is in the managedhub values.
	// If not, skip this remoteService.
	var reqBodyMap map[string]interface{}
	if err := json.Unmarshal(receivedBody, &reqBodyMap); err != nil {
		klog.Errorf("Error unmarshaling federated request body: %s", err)
	}
	// Navigate to the managedHub values in the request body.
	// this is sample federated search query
	// {"operationName":"searchSchema","variables":{"query":{"keywords":[],
	// "filters":[{"property":"managedHub","values":["local-cluster"]},
	// {"property":"cluster","values":["local-cluster"]}],"limit":10000}},
	// "query":"query searchSchema($query: SearchInput) {\n  searchSchema(query: $query)\n}"}
	managedHubValues := []string{}
	if variables, ok := reqBodyMap["variables"].(map[string]interface{}); ok {
		if query, ok := variables["query"].(map[string]interface{}); ok {
			if filters, ok := query["filters"].([]interface{}); ok {
				for _, filter := range filters {
					if filterMap, ok := filter.(map[string]interface{}); ok {
						if property, ok := filterMap["property"].(string); ok && property == "managedHub" {
							if values, ok := filterMap["values"].([]interface{}); ok {
								for _, value := range values {
									if strValue, ok := value.(string); ok {
										managedHubValues = append(managedHubValues, strValue)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	klog.V(3).Infof("ManagedHub filter values in request: %v", managedHubValues)

	fedConfig := getFedConfig(ctx, r)

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
		wg.Add(1)
		go func(remoteService RemoteSearchService) {
			defer wg.Done()
			// Get the http client from pool.
			client := httpClientGetter()
			fedRequest.getFederatedResponse(remoteService, receivedBody, client)
			httpClientPool.Put(client) // Put the client back into the pool for reuse.
		}(remoteService)
	}
	klog.V(3).Infof("Sent %d federated requests, waiting for response.", len(fedConfig))
	wg.Wait()

	// Send JSON response to client.
	sendResponse(w, &fedRequest.Response)
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

func (fedRequest *FederatedRequest) getFederatedResponse(remoteService RemoteSearchService,
	receivedBody []byte, client HTTPClient) {

	// Create the request.
	req, err := http.NewRequest("POST", remoteService.URL, bytes.NewBuffer(receivedBody))
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
