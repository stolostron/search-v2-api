package federated

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Data needed to process a federated request.
type FederatedRequest struct {
	InRequestBody []byte
	Response      FederatedResponse
	// Fields below are for future use.
	// Sent     []string     `json:"sent"`
	// Received []string     `json:"received"`
	// OutRequests   map[string]OutboundRequestLog
}

type FederatedResponse struct {
	Data   DataResponse `json:"data"`
	Errors []error      `json:"errors,omitempty"`
}

// FUTURE: Keep track of outbound requests.
// type OutboundRequestLog struct {
// 	RemoteService string
// 	SentTime      time.Time
// 	ReceivedTime  time.Time
// 	ResponseBody  []byte
// }

func HandleFederatedRequest(w http.ResponseWriter, r *http.Request) {
	klog.Info("Resolving federated search query.")

	receivedBody, err := io.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("Error reading request body: %s", err)
	}
	klog.Infof("Received federated search query: %s", string(receivedBody))

	fedRequest := FederatedRequest{
		InRequestBody: receivedBody,
		Response: FederatedResponse{
			Data:   DataResponse{},
			Errors: []error{},
		},
	}

	fedConfig := getFederationConfig()
	klog.Infof("Sending federated query to %d remote services.", len(fedConfig))

	wg := sync.WaitGroup{}
	for _, remoteService := range fedConfig {
		wg.Add(1)
		go func(remoteService RemoteSearchService) {
			defer wg.Done()
			// Create http client. TODO: move to a pool to share this client.
			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // TODO: Use TLS verification.
				},
				Timeout: time.Second * 30,
			}

			// Create the request.
			req, err := http.NewRequest("POST", remoteService.URL, bytes.NewBuffer(receivedBody))
			if err != nil {
				klog.Errorf("Error creating federated request: %s", err)
				fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("Error creating federated request: %s", err))
				return
			}

			// Set the request headers.
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", remoteService.Token))

			// Send the request.
			resp, err := client.Do(req)
			if err != nil {
				klog.Errorf("Error sending federated request: %s", err)
				fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("Error sending federated request: %s", err))
				return
			}

			// Process the response.
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				klog.Errorf("Error reading federated response body: %s", err)
				fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("Error reading federated response body: %s", err))
				return
			}

			klog.Infof("Received federated response from %s: \n%s", remoteService.Name, string(body))
			parseResponse(&fedRequest, body)

		}(remoteService)
	}

	klog.Info("Waiting for all remote services to respond.")
	wg.Wait()

	klog.Info("Merging the federated responses.")

	klog.Info("Sending federated response to client.")
	response := json.NewEncoder(w).Encode(fedRequest.Response)

	fmt.Fprint(w, response)
}
