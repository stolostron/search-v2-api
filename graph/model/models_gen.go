// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

// A message is used to communicate conditions detected while executing a query on the server.
type Message struct {
	// Unique identifier to be used by clients to process the message independently of locale or gramatical changes.
	ID string `json:"id"`
	// Message type.
	// **Values:** information, warning, error.
	Kind *string `json:"kind"`
	// Message text.
	Description *string `json:"description"`
}

// Defines a key/value to filter results.
// When multiple values are provided for a property, it is interpreted as an OR operation.
type SearchFilter struct {
	// Name of the property (or key).
	Property string `json:"property"`
	// Values for the property. Multiple values per property are interpreted as an OR operation.
	Values []*string `json:"values"`
}

// Input options to the search query.
type SearchInput struct {
	// List of strings to match resources.
	// Will match resources containing any of the keywords in any text field.
	// When multiple keywords are provided, it is interpreted as an AND operation.
	// Matches are case insensitive.
	Keywords []*string `json:"keywords"`
	// List of SearchFilter, which is a key(property) and values.
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
