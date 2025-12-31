// Copyright Contributors to the Open Cluster Management project
package federated

import (
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"
)

func (d *Data) mergeSearchSchema(schemaProps []string) {
	klog.V(1).Info("Merge [searchSchema] results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.SearchSchema == nil {
		d.searchSchemaValues = make(map[string]interface{})
		d.SearchSchema = &SearchSchema{AllProperties: make([]string, 0)}
	}
	// Add "managedHub" as a filter option in global Search - ACM-10019
	if !slices.Contains(schemaProps, "managedHub") {
		schemaProps = append([]string{"managedHub"}, schemaProps[:]...)
	}
	for _, prop := range schemaProps {
		if _, exists := d.searchSchemaValues[prop]; !exists {
			d.searchSchemaValues[prop] = struct{}{}
			d.SearchSchema.AllProperties = append(d.SearchSchema.AllProperties, prop)
		}
	}
}

func (d *Data) mergeSearchComplete(s []string) {
	klog.V(1).Info("Merge [searchComplete] results to federated response.")
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
	klog.V(1).Info("Merge [searchResult] to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.Search == nil {
		d.Search = make([]SearchResult, len(results))
	}

	for index, result := range results {
		// Preserve __typename from the first response
		if d.Search[index].TypeName == "" && result.TypeName != "" {
			d.Search[index].TypeName = result.TypeName
		}

		// Count
		d.Search[index].Count = d.Search[index].Count + int(result.Count)

		// Items
		// TODO: Handle SORT and LIMIT for Items.
		for _, item := range result.Items {
			item["managedHub"] = hubName
			d.Search[index].Items = append(d.Search[index].Items, item)
		}

		// Related
		if d.Search[index].Related == nil {
			d.Search[index].Related = make([]SearchRelatedResult, 0)
		}
		if len(result.Related) > 0 {
			for _, related := range result.Related {
				for _, item := range related.Items {
					item["managedHub"] = hubName
				}
			}
			d.Search[index].Related = d.appendRelatedResults(d.Search[index].Related, result.Related)
		}
	}
}

func (d *Data) mergeMessages(msgs []string) {
	klog.V(1).Info("Merge [message] results to federated response.")
	d.writeLock.Lock()
	defer d.writeLock.Unlock()

	if d.Messages == nil {
		d.Messages = make([]string, 0)
	}

	d.Messages = append(d.Messages, msgs...)
}

func (d *Data) appendRelatedResults(mergedItems, newItems []SearchRelatedResult) []SearchRelatedResult {
	klog.V(1).Info("Merge [related] to federated response.")

	if len(mergedItems) == 0 {
		return newItems
	}

	// Related results are grouped by kind.
	for _, newItem := range newItems {
		found := false
		kind := newItem.Kind

		// If the kind is found, merge the newItems with the mergedItems.
		for index, mergedItem := range mergedItems {
			if mergedItem.Kind == kind {
				// Preserve __typename from the first response if not already set
				if mergedItems[index].TypeName == "" && newItem.TypeName != "" {
					mergedItems[index].TypeName = newItem.TypeName
				}
				mergedItems[index].Count = mergedItem.Count + newItem.Count
				mergedItems[index].Items = append(mergedItem.Items, newItem.Items...)
				found = true
				break
			}
		}

		// If the kind was not found, add it to the mergedItems.
		if !found {
			mergedItems = append(mergedItems, newItem)
		}
	}
	return mergedItems
}
