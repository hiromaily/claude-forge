package sourcetype

import "regexp"

// Handler defines the contract for a source type integration.
type Handler interface {
	Type() string
	Label() string
	URLPattern() *regexp.Regexp
	BasePattern() *regexp.Regexp
	InvalidURLMessage() string
	ExtractSourceID(rawURL string) string
	FetchConfig(sourceURL, sourceID string) *FetchConfig
	PostConfig(sourceURL, sourceID, artifactPath string) *PostConfig
	ParseExternalContext(m map[string]any) ExternalFields
	SupportsClosingRef() bool

	// FieldPrefix returns the key prefix used in external_context maps
	// (e.g., "github_", "jira_", "linear_"). Used by ClassifyByFieldPrefix.
	FieldPrefix() string
}
