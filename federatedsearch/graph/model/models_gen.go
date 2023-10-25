// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

type SearchInput struct {
	// Boolean to check if query is from globalhub
	Globalhub *bool `json:"globalhub,omitempty"`
}

type SearchResult struct {
	// Resources matching the search query.
	Items []map[string]interface{} `json:"items,omitempty"`
}

func (SearchResult) IsEntity() {}