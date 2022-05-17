// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

// A message is used to communicate conditions detected while executing a query on the server.
type Message struct {
	// Unique identifier. This can be used by clients to process the message independently of localization or gramatical changes.
	ID string `json:"id"`
	// Message type (information, warning, error).
	// **Values:** information, warning, error.
	Kind *string `json:"kind"`
	// Message text.
	Description *string `json:"description"`
}

// Defines a key/value to filter results.
// When multiple values are provided for a property, it's interpreted as an OR operation.
type SearchFilter struct {
	// Defines the property or key.
	Property string `json:"property"`
	// Defines the values for a property. Multiple values are interpreted as an OR operation.
	Values []*string `json:"values"`
}

// Input options to the search query.
type SearchInput struct {
	// List of strings to match resources.
	// Will match resources containiny any of the keywords in any text field.
	// When multiple keywords are provided, it is interpreted as an AND operation.
	// Matches are case insensitive.
	Keywords []*string `json:"keywords"`
	// List of SearchFilter, which is a key(properrty) and values.
	// When multiple filters are provided, results will match all fiters (AND operation).
	Filters []*SearchFilter `json:"filters"`
	// Max number of results returned by the query.
	// **Default is** 10,000
	Limit *int `json:"limit"`
	// Filter relationships to the specified kinds.
	// If empty, all relationships will be included.
	// This filter is used with the 'related' field on SearchResult.
	RelatedKinds []*string `json:"relatedKinds"`
}

// Data required to save a user search query.
type UserSearch struct {
	// Unique identifier of the saved search query.
	ID *string `json:"id"`
	// Name of the saved search query.
	Name *string `json:"name"`
	// Description of the saved search query.
	Description *string `json:"description"`
	// The search query in text format.
	// Example:
	// - `kind:pod,deployment namespace:default bar foo`
	SearchText *string `json:"searchText"`
}
