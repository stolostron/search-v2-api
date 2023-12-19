// Copyright Contributors to the Open Cluster Management project
package federated

import "k8s.io/klog/v2"

func (d *Data) mergeSearchSchema(schemaProps []string) {
	klog.Info("Merge searchSchema results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.SearchSchema == nil {
		d.SearchSchema = &SearchSchema{AllProperties: make([]string, 0)}
	}
	// TODO: Remove duplicates.
	d.SearchSchema.AllProperties = append(d.SearchSchema.AllProperties, schemaProps...)
}

func (d *Data) mergeSearchComplete(s []string) {
	klog.Info("Merge searchComplete results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.SearchComplete == nil {
		d.SearchComplete = make([]string, 0)
	}
	// TODO: Remove duplicates.
	// TODO: How to handle LIMIT ?
	// TODO: How to handle SORT ?
	d.SearchComplete = append(d.SearchComplete, s...)
}

func (d *Data) mergeSearchResults(hubName string, results []SearchResult) {
	klog.Info("Merge searchResult to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.Search == nil {
		d.Search = make([]SearchResult, len(results))
	}

	for index, result := range results {
		// Count
		d.Search[index].Count = d.Search[index].Count + int(result.Count)

		// Items
		// TODO: How to handle LIMIT ?
		for _, item := range result.Items {
			item["managedHub"] = hubName
			d.Search[index].Items = append(d.Search[index].Items, item)
		}

		// Related
		// TODO: Implement related.
		// d.Search[index].Related = append(d.Search[index].Related, result["related"].([]interface{})...)
	}
}

func (d *Data) mergeMessages(msgs []string) {
	klog.Info("Merge message results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.Messages == nil {
		d.Messages = make([]string, 0)
	}

	d.Messages = append(d.Messages, msgs...)
}
