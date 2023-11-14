// Copyright Contributors to the Open Cluster Management project
package fedresolver

type SearchRelatedResult struct {
	Kind  string                   `json:"kind"`
	Count *int                     `json:"count"`
	Items []map[string]interface{} `json:"items"`
}
