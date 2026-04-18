package sourcetype

import (
	"fmt"
	"strings"
)

var handlers []Handler

func register(h Handler) {
	handlers = append(handlers, h)
}

func Get(sourceType string) Handler {
	for _, h := range handlers {
		if h.Type() == sourceType {
			return h
		}
	}
	return nil
}

func All() []Handler {
	return handlers
}

func IsURLSource(sourceType string) bool {
	return Get(sourceType) != nil
}

// ClassifyByFieldPrefix infers the source type from field name prefixes in a map.
// Returns the source type of the first handler whose FieldPrefix matches a key,
// or "" if no match. Used as a backward-compatibility fallback when source_url
// is not provided but prefixed fields (github_*, jira_*, linear_*) are.
func ClassifyByFieldPrefix(m map[string]any) string {
	for _, h := range handlers {
		prefix := h.FieldPrefix()
		for key := range m {
			if len(key) > len(prefix) && key[:len(prefix)] == prefix {
				return h.Type()
			}
		}
	}
	return ""
}

func ClassifyURL(rawURL string) (string, error) {
	for _, h := range handlers {
		if h.BasePattern().MatchString(rawURL) {
			if h.URLPattern().MatchString(rawURL) {
				return h.Type(), nil
			}
			return "", fmt.Errorf("ERROR: %s", h.InvalidURLMessage())
		}
	}
	var supported []string
	for _, h := range handlers {
		supported = append(supported, h.InvalidURLMessage())
	}
	return "", fmt.Errorf("ERROR: Unrecognised URL format. Supported formats: %s",
		strings.Join(supported, ", "))
}
