package federated

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"
)

// Used to parse the incoming GraphQL response.
type GraphQLResponse struct {
	Data   map[string]interface{} `json:"data"`
	Errors []interface{}          `json:"errors"`
}

// Parse the response from a remote search service.
func parseResponse(fedRequest *FederatedRequest, body []byte) {
	var response GraphQLResponse
	err := json.Unmarshal(body, &response)

	if err != nil {
		klog.Errorf("Error parsing response: %s", err)
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("Error parsing response: %s", err))
		return
	}

	klog.Infof("Parsing Response: %+v", response)

	for key, value := range response.Data {

		switch key {
		case "searchSchema":
			klog.Info("Found searchSchema in response.")
			allProps := value.(map[string]interface{})["allProperties"]
			fedRequest.Response.Data.mergeSearchSchema(allProps.([]interface{}))

		case "searchComplete":
			klog.Info("Found searchComplete in response.")
			fedRequest.Response.Data.mergeSearchComplete(value.([]interface{}))

		case "messages":
			klog.Info("Found messages in response.")
			// fedRequest.Data.Messages = append(fedRequest.Data.Messages, value.([]interface{})...)

		case "search":
			klog.Infof("Found SearchResults in response. %+v", value)
			searchResults := value.([]interface{})
			fedRequest.Response.Data.mergeSearchResults(searchResults)

		default:
			klog.Infof("Found unknown key in results: %s", key)

		}
	}

}
