// Copyright Contributors to the Open Cluster Management project
package federated

import "k8s.io/klog/v2"

func (d *Data) mergeSearchSchema(schemaProps []string) {
	klog.Info("Merge searchSchema results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()
	if d.SearchSchema.AllProperties == nil {
		d.SearchSchema.AllProperties = []string{}
	}
	// TODO: Remove duplicates.
	d.SearchSchema.AllProperties = append(d.SearchSchema.AllProperties, schemaProps...)
}

func (d *Data) mergeSearchComplete(s []string) {
	klog.Info("Merge searchComplete results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()
	if d.SearchComplete == nil {
		d.SearchComplete = []string{}
	}
	// TODO: Remove duplicates.
	d.SearchComplete = append(d.SearchComplete, s...)
}

func (d *Data) mergeSearchResults(hubName string, results []SearchResult) {
	klog.Info("Merge searchResult to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()
	if d.Search == nil {
		d.Search = make([]SearchResult, len(results))
		klog.Infof("Created SearchResults array with %d elements.", len(results))
	}

	for index, result := range results {
		// Count
		d.Search[index].Count = d.Search[index].Count + int(result.Count)

		// Items
		for _, item := range result.Items {
			item["managedHub"] = hubName
			d.Search[index].Items = append(d.Search[index].Items, item)
		}

		// TODO: Related
		// d.Search[index].Related = append(d.Search[index].Related, result["related"].([]interface{})...)
	}
}
