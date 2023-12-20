// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"encoding/json"
	"fmt"
	"sync"

	"k8s.io/klog/v2"
)

// Used to parse the GraphQL payload.
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

type SearchSchema struct {
	AllProperties []string `json:"allProperties,omitempty"`
}

type Data struct {
	Messages       []string       `json:"messages,omitempty"`
	Search         []SearchResult `json:"searchResult,omitempty"` // FIXME: Hacked to solve aliasing issue from console.
	SearchComplete []string       `json:"searchComplete,omitempty"`
	SearchSchema   *SearchSchema  `json:"searchSchema,omitempty"`
	GraphQLSchema  interface{}    `json:"__schema,omitempty"`
	writeLock      sync.Mutex
}

type GraphQLPayload struct {
	Data   Data    `json:"data"`
	Errors []error `json:"errors,omitempty"`
}

// Parse the response from a remote search service.
func parseResponse(fedRequest *FederatedRequest, body []byte, hubName string) {
	var response GraphQLPayload
	err := json.Unmarshal(body, &response)

	if err != nil {
		klog.Errorf("Error parsing response: %s", err)
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, fmt.Errorf("Error parsing response: %s", err))
		return
	}

	if len(response.Errors) > 0 {
		fedRequest.Response.Errors = append(fedRequest.Response.Errors, response.Errors...)
	}

	if response.Data.SearchSchema != nil {
		klog.Info("Found SearchSchema in response.")
		fedRequest.Response.Data.mergeSearchSchema(response.Data.SearchSchema.AllProperties)
	}
	if len(response.Data.SearchComplete) > 0 {
		klog.Info("Found SearchComplete in response.")
		fedRequest.Response.Data.mergeSearchComplete(response.Data.SearchComplete)
	}
	if len(response.Data.Search) > 0 {
		klog.Infof("Found SearchResults in response. %+v", response.Data.Search)
		fedRequest.Response.Data.mergeSearchResults(hubName, response.Data.Search)
	}
	if len(response.Data.Messages) > 0 {
		klog.Info("Found messages in response.")
		fedRequest.Response.Data.mergeMessages(response.Data.Messages)
	}
	// Needed to support GraphQL instrospection for clients like GraphQL Playground and Postman.
	// FUTURE: Validate that the schema on each search API instance is the same.
	if response.Data.GraphQLSchema != nil {
		klog.Info("Found schema in response.")
		fedRequest.Response.Data.GraphQLSchema = response.Data.GraphQLSchema
	}
}
