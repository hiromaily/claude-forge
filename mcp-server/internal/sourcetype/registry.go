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
