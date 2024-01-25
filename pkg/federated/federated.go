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
	// FUTURE: The fields below are for future use.
	// OutRequests   map[string]OutboundRequestLog
}

// FUTURE: Keep track of outbound requests.
// type OutboundRequestLog struct {
// 	RemoteService string
// 	SentTime      time.Time
// 	ReceivedTime  time.Time
// 	ResponseBody  []byte
// }

var getFedConfig = getFederationConfig

var httpClientGetter = GetHttpClient

func HandleFederatedRequest(w http.ResponseWriter, r *http.Request) {
	klog.Info("Resolving federated search query.")
	ctx := r.Context()
	receivedBody, err := io.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("Error reading request body: %s", err)
	}
	klog.Infof("Received federated search query: %s", string(receivedBody))

	fedRequest := FederatedRequest{
		InRequestBody: receivedBody,
		Response: GraphQLPayload{
			Data:   Data{},
			Errors: []string{},
		},
	}

	fedConfig := getFedConfig(ctx, r)
	klog.Infof("Sending federated query to %d remote services.", len(fedConfig))

	wg := sync.WaitGroup{}
	for _, remoteService := range fedConfig {
		wg.Add(1)
		go func(remoteService RemoteSearchService) {
			defer wg.Done()
			// Get the http client from pool.
			client := httpClientGetter(remoteService)
			fedRequest.getFederatedResponse(remoteService, receivedBody, client)
			httpClientPool.Put(client) // Put the client back into the pool for reuse
		}(remoteService)
	}
	klog.Info("Waiting for all remote services to respond.")
	wg.Wait()

	// Send JSON response to client.
	w.Header().Set("Content-Type", "application/json")
	result := json.NewEncoder(w).Encode(&fedRequest.Response)
	if result != nil {
		klog.Errorf("Error encoding federated response: %s", result)
	}
	klog.Info("Sent federated response to client.")
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
		klog.Errorf("Error reading federated response body: %s", err)
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("error reading federated response body: %s", err).Error())
		return
	}

	klog.Infof("Received federated response from %s: \n%s", remoteService.Name, string(body))
	parseResponse(fedRequest, body, remoteService.Name)
}
