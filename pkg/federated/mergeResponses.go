package federated

import "k8s.io/klog/v2"

func (dr *DataResponse) mergeSearchSchema(s []string) {
	klog.Info("Merge searchSchema results to federated response.")
	if dr.SearchSchema == nil {
		dr.SearchSchema = []string{}
	}
	// TODO: Remove duplicates.
	dr.SearchSchema = append(dr.SearchSchema, s...)
}

func (dr *DataResponse) mergeSearchComplete(s []string) {
	klog.Info("Merge searchComplete results to federated response.")
	if dr.SearchComplete == nil {
		dr.SearchComplete = []string{}
	}
	// TODO: Remove duplicates.
	dr.SearchComplete = append(dr.SearchComplete, s...)
}

func (dr *DataResponse) mergeSearchResults(results []SearchResult) {
	klog.Info("Merge searchResult to federated response.")
	if dr.Search == nil {
		dr.Search = make([]SearchResult, len(results))
		klog.Infof("Created SearchResults array with %d elements.", len(results))
	}

	for index, result := range results {
		// Count
		dr.Search[index].Count = dr.Search[index].Count + int(result.Count)

		// Items
		for _, item := range result.Items {
			item["managedHub"] = "insert-managed-hub-name" // TODO: Replace with managed hub name.
			dr.Search[index].Items = append(dr.Search[index].Items, item)
		}

		// Related
		// dr.Search[index].Related = append(dr.Search[index].Related, result["related"].([]interface{})...)
	}
}
