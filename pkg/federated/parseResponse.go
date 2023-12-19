package federated

import (
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"
)

// Used to parse the GraphQL response.
type GraphQLResponse struct {
	Data   DataResponse `json:"data"`
	Errors []error      `json:"errors,omitempty"`
}

type SearchRelatedResult struct {
	Count int                      `json:"count,omitempty"`
	Kind  string                   `json:"kind,omitempty"`
	Items []map[string]interface{} `json:"items,omitempty"`
}

type SearchResult struct {
	Count   int                      `json:"count,omitempty"`
	Items   []map[string]interface{} `json:"items,omitempty"`
	Related []SearchRelatedResult    `json:"related,omitempty"`
}

type DataResponse struct {
	Messages       []string       `json:"messages,omitempty"`
	Search         []SearchResult `json:"search,omitempty"`
	SearchComplete []string       `json:"searchComplete,omitempty"`
	SearchSchema   []string       `json:"searchSchema,omitempty"`
	GraphQLSchema  interface{}    `json:"__schema,omitempty"`
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

	klog.Infof("Parsing Response [errors] field: %+v", response.Errors)
	if len(response.Errors) > 0 {
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, response.Errors...)
	}

	klog.Infof("Parsing Response [data] field: %+v", response.Data)

	if response.Data.SearchSchema != nil {
		klog.Info("Found searchSchema in response.")
		fedRequest.Response.Data.mergeSearchSchema(response.Data.SearchSchema)
	}
	if response.Data.SearchComplete != nil {
		klog.Info("Found searchComplete in response.")
		fedRequest.Response.Data.mergeSearchComplete(response.Data.SearchComplete)
	}
	if response.Data.Search != nil {
		klog.Infof("Found SearchResults in response. %+v", response.Data.Search)
		fedRequest.Response.Data.mergeSearchResults(response.Data.Search)
	}
	if response.Data.Messages != nil {
		klog.Info("Found messages in response.")
		fedRequest.Response.Data.Messages = append(fedRequest.Response.Data.Messages, response.Data.Messages...)
	}
	// Needed to support GraphQL instrospection for clients like GraphQL Playground and Postman.
	// FUTURE: Validate that the schema on each search API instance is the same.
	if response.Data.GraphQLSchema != nil {
		klog.Info("Found schema in response.")
		fedRequest.Response.Data.GraphQLSchema = response.Data.GraphQLSchema
	}
}
