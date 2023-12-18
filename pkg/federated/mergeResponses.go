package federated

import "k8s.io/klog/v2"

type SearchRelatedResult struct {
	Count int
	Items []interface{}
	Kind  string
}

type SearchResult struct {
	Count   int                   `json:"count,omitempty"`
	Items   []interface{}         `json:"items,omitempty"`
	Related []SearchRelatedResult `json:"related,omitempty"`
}

type DataResponse struct {
	Messages       []string       `json:"messages,omitempty"`
	Search         []SearchResult `json:"search,omitempty"`
	SearchComplete []string       `json:"searchComplete,omitempty"`
	SearchSchema   []string       `json:"searchSchema,omitempty"`
}

func (dr *DataResponse) mergeSearchSchema(s []interface{}) {
	klog.Info("Merge searchSchema results to federated response.")
	if dr.SearchSchema == nil {
		dr.SearchSchema = []string{}
	}
	// TODO: Remove duplicates.
	for _, v := range s {
		dr.SearchSchema = append(dr.SearchSchema, v.(string))
	}
}

func (dr *DataResponse) mergeSearchComplete(s []interface{}) {
	klog.Info("Merge searchComplete results to federated response.")
	if dr.SearchComplete == nil {
		dr.SearchComplete = []string{}
	}
	// TODO: Remove duplicates.
	for _, v := range s {
		dr.SearchComplete = append(dr.SearchComplete, v.(string))
	}
}

func (dr *DataResponse) mergeSearchResults(results []interface{}) {
	klog.Info("Merge searchResult to federated response.")
	if dr.Search == nil {
		dr.Search = make([]SearchResult, len(results))
		klog.Infof("Created SearchResults array with %d elements.", len(results))
	}

	for index, result := range results {
		result := result.(map[string]interface{})
		// Count
		dr.Search[index].Count = dr.Search[index].Count + int(result["count"].(float64))

		// Items
		for _, item := range result["items"].([]interface{}) {
			item.(map[string]interface{})["managedHub"] = "insert-managed-hub-name"
			dr.Search[index].Items = append(dr.Search[index].Items, item)
		}

		// TODO: Related
		// dr.Search[index].Related = append(dr.Search[index].Related, result["related"].([]interface{})...)
	}
}
