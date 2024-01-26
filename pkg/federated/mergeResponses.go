// Copyright Contributors to the Open Cluster Management project
package federated

import "k8s.io/klog/v2"

func (d *Data) mergeSearchSchema(schemaProps []string) {
	klog.Info("Merge searchSchema results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.SearchSchema == nil {
		d.searchSchemaValues = make(map[string]interface{})
		d.SearchSchema = &SearchSchema{AllProperties: make([]string, 0)}
	}
	for _, prop := range schemaProps {
		if _, exists := d.searchSchemaValues[prop]; !exists {
			d.searchSchemaValues[prop] = struct{}{}
			d.SearchSchema.AllProperties = append(d.SearchSchema.AllProperties, prop)
		}
	}
}

func (d *Data) mergeSearchComplete(s []string) {
	klog.Info("Merge searchComplete results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.SearchComplete == nil {
		d.searchCompleteValues = make(map[string]interface{})
		d.SearchComplete = make([]string, 0)
	}

	for _, prop := range s {
		if _, exists := d.searchCompleteValues[prop]; !exists {
			d.searchCompleteValues[prop] = struct{}{}
			d.SearchComplete = append(d.SearchComplete, prop)
		}
	}

	// TODO: How to handle LIMIT ?
	// 		We would need to parse the incoming graphql query to determine the limit.
	//      This can be done but adds significant complexity.

	// TODO: How to handle SORT ?
	//      We would need to wait until all results are in before sorting, which will
	//      impact response time.
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
		for _, item := range result.Items {
			item["managedHub"] = hubName
			d.Search[index].Items = append(d.Search[index].Items, item)
		}
		// TODO: How to handle LIMIT ?
		// TODO: How to handle SORT ?

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
