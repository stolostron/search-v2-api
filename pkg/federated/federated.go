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
var httpClientGetter = GetHttpClient

func HandleFederatedRequest(w http.ResponseWriter, r *http.Request) {
	klog.Info("Received federated search request.")
	ctx := r.Context()
	receivedBody, err := io.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("Error reading request body: %s", err)
		sendResponse(w, &GraphQLPayload{
			Data:   Data{},
			Errors: []string{fmt.Errorf("error reading request body: %s", err).Error()},
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
	// Check if receivedBody has ManagedHub filter - if yes, get FedConfig only for select managedHubs
	clusterList, hubListErr := managedHubList(receivedBody)
	if hubListErr != nil {
		klog.Errorf("Error fetching managed hub list %s: %+v", string(receivedBody), hubListErr)
		sendResponse(w, &GraphQLPayload{
			Data:   Data{},
			Errors: []string{fmt.Errorf("error fetching managed hub list: %s", hubListErr).Error()},
		})
		return
	}
	fedConfig := getFedConfig(ctx, r, clusterList)

	klog.V(2).Infof("Sending federated query to %d remote services.", len(fedConfig))

	wg := sync.WaitGroup{}
	for _, remoteService := range fedConfig {
		wg.Add(1)
		go func(remoteService RemoteSearchService) {
			defer wg.Done()
			klog.V(5).Info("Sending federated request to ", remoteService.Name)
			// Get the http client from pool.
			client := httpClientGetter(remoteService)
			fedRequest.getFederatedResponse(remoteService, receivedBody, client)
			httpClientPool.Put(client) // Put the client back into the pool for reuse.
		}(remoteService)
	}
	klog.V(2).Info("Waiting for all federated requests to respond.")
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
	klog.Info("Sent federated response.")
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
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		klog.Errorf("Error reading federated response from %s: %s", remoteService.Name, err)
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("error reading federated response body: %s", err).Error())
		return
	}

	klog.V(2).Infof("Received response from %s:\n%s", remoteService.Name, string(body))
	parseResponse(fedRequest, body, remoteService.Name)
}
