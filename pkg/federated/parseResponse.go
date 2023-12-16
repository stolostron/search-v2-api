package federated

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"
)

type GraphQLResponse struct {
	Data   map[string]interface{} `json:"data"`
	Errors []interface{}          `json:"errors"`
}

func parseResponse(requestContext *RequestContext, body []byte) {
	var response GraphQLResponse
	err := json.Unmarshal(body, &response)

	if err != nil {
		klog.Errorf("Error parsing response: %s", err)
		requestContext.Errors = append(requestContext.Errors, fmt.Errorf("Error parsing response: %s", err))
		return
	}

	klog.Infof("Parsing Response: %+v", response)

	for key, value := range response.Data {

		switch key {
		case "searchSchema":
			klog.Info("Found searchSchema in response.")
			allProps := value.(map[string]interface{})["allProperties"]
			requestContext.Data.mergeSearchSchema(allProps.([]interface{}))

		case "searchComplete":
			klog.Info("Found searchComplete in response.")
			requestContext.Data.mergeSearchComplete(value.([]interface{}))

		case "messages":
			klog.Info("Found messages in response.")
			// requestContext.Data.Messages = append(requestContext.Data.Messages, value.([]interface{})...)

		case "search":
			klog.Infof("Found SearchResults in response. %+v", value)
			searchResults := value.([]interface{})
			requestContext.Data.mergeSearchResults(searchResults)

		default:
			klog.Infof("Found unknown key in results: %s", key)

		}
	}

}
