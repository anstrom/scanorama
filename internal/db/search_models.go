// Package db provides search result models for unified cross-entity search.
package db

// SearchResult represents a single search hit from any entity type.
type SearchResult struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

// SearchResults is the top-level response returned by the search endpoint.
type SearchResults struct {
	Results map[string][]SearchResult `json:"results"`
	Total   int                       `json:"total"`
}
