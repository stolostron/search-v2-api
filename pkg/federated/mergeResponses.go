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

	// TODO: Handle SORT and LIMIT for SearchComplete.
	// LIMIT
	//   We would need to parse the incoming graphql query to determine the limit.
	//   This can be done but adds significant complexity.
	// SORT
	//   SORT function would need to wait until all results are in before sorting,
	//   which will impact response time.
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
		// TODO: Handle SORT and LIMIT for Items.

		// Related
		if d.Search[index].Related == nil {
			d.Search[index].Related = make([]SearchRelatedResult, 0)
		}
		// d.mergeRelatedResults(d.Search[index].Related, result.Related)
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

func (d *Data) mergeRelatedResults(mergedItems, newItems []SearchRelatedResult) {
	klog.Info("Merge related results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	// Related results are grouped by kind.
	for _, newItem := range newItems {
		kind := newItem.Kind
		for _, mergedItem := range mergedItems {
			if mergedItem.Kind == kind {
				// Merge the new items.
				mergedItem.Count = mergedItem.Count + newItem.Count
				for _, item := range newItem.Items {
					item["managedHub"] = "TODO_ADD_HUB_NAME_HERE" // TODO: Add hub name to related items.
					mergedItem.Items = append(mergedItem.Items, item)
				}
				return
			}
		}
	}
}
