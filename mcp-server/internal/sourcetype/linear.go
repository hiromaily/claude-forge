package sourcetype

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/maputil"
)

var (
	reLinearURL  = regexp.MustCompile(`^https://linear\.app/[^/]+/issue/[A-Z]+-[0-9]+`)
	reLinearBase = regexp.MustCompile(`^https://linear\.app/`)
)

func init() { register(&LinearHandler{}) }

type LinearHandler struct{}

func (h *LinearHandler) Type() string               { return "linear_issue" }
func (h *LinearHandler) Label() string              { return "Linear issue" }
func (h *LinearHandler) FieldPrefix() string        { return "linear_" }
func (h *LinearHandler) URLPattern() *regexp.Regexp  { return reLinearURL }
func (h *LinearHandler) BasePattern() *regexp.Regexp { return reLinearBase }
func (h *LinearHandler) SupportsClosingRef() bool    { return false }

func (h *LinearHandler) InvalidURLMessage() string {
	return "Invalid Linear URL format. Expected: https://linear.app/{org}/issue/{KEY}-{number}"
}

func (h *LinearHandler) ExtractSourceID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, seg := range segments {
		if seg == "issue" && i+1 < len(segments) {
			return segments[i+1]
		}
	}
	return ""
}

func (h *LinearHandler) FetchConfig(sourceURL, sourceID string) *FetchConfig {
	return &FetchConfig{
		Type:    "linear",
		MCPTool: "mcp__linear__get_issue",
		MCPParams: map[string]string{"issueId": sourceID},
		ResponseMapping: map[string]string{
			"title": "linear_title", "description": "linear_description",
			"priority": "linear_priority", "estimate": "linear_estimate",
			"labels": "linear_labels",
		},
		Instruction: "fetch linear issue fields before calling pipeline_init_with_context",
	}
}

func (h *LinearHandler) PostConfig(sourceURL, sourceID, artifactPath string) *PostConfig {
	return &PostConfig{
		MCPTool:    "mcp__linear__save_comment",
		MCPParams:  map[string]string{"issueId": sourceID},
		BodySource: artifactPath,
	}
}

func (h *LinearHandler) ParseExternalContext(m map[string]any) ExternalFields {
	return ExternalFields{
		Title:       maputil.StringFieldAlt(m, "linear_title", "title"),
		Body:        maputil.StringFieldAlt(m, "linear_description", "description"),
		Labels:      maputil.StringArray(m, "linear_labels"),
		StoryPoints: maputil.IntFieldAlt(m, "linear_estimate", "estimate"),
	}
}
