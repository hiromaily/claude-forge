package sourcetype

import (
	"net/url"
	"path"
	"regexp"

	"github.com/hiromaily/claude-forge/mcp-server/pkg/maputil"
)

var (
	reJiraURL  = regexp.MustCompile(`^https://[^/]+\.atlassian\.net/browse/[A-Z]+-[0-9]+`)
	reJiraBase = regexp.MustCompile(`^https://[^/]+\.atlassian\.net/`)
)

func init() { register(&JiraHandler{}) }

type JiraHandler struct{}

func (h *JiraHandler) Type() string                { return "jira_issue" }
func (h *JiraHandler) Label() string               { return "Jira issue" }
func (h *JiraHandler) FieldPrefix() string         { return "jira_" }
func (h *JiraHandler) URLPattern() *regexp.Regexp  { return reJiraURL }
func (h *JiraHandler) BasePattern() *regexp.Regexp { return reJiraBase }
func (h *JiraHandler) SupportsClosingRef() bool    { return false }

func (h *JiraHandler) InvalidURLMessage() string {
	return "Invalid Jira URL format. Expected: https://{org}.atlassian.net/browse/{KEY}-{number}"
}

func (h *JiraHandler) ExtractSourceID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return path.Base(u.Path)
}

func (h *JiraHandler) FetchConfig(sourceURL, sourceID string) *FetchConfig {
	return &FetchConfig{
		Type: "jira",
		ResponseMapping: map[string]string{
			"summary": "jira_summary", "description": "jira_description",
			"issue_type": "jira_issue_type", "story_points": "jira_story_points",
		},
		Instruction: "fetch jira issue fields (summary, description, issuetype, story_points) before calling pipeline_init_with_context. Use Atlassian MCP tools if available, or Jira REST API with $JIRA_USER:$JIRA_TOKEN credentials.",
	}
}

func (h *JiraHandler) PostConfig(sourceURL, sourceID, artifactPath string) *PostConfig {
	return &PostConfig{
		BodySource:  artifactPath,
		Instruction: "Post the contents of " + artifactPath + " as a comment to " + sourceURL + ". Use Atlassian MCP tools if available, or convert the markdown to ADF and POST via Jira REST API with $JIRA_USER:$JIRA_TOKEN.",
	}
}

func (h *JiraHandler) ParseExternalContext(m map[string]any) ExternalFields {
	return ExternalFields{
		Title:       maputil.StringFieldAlt(m, "jira_summary", "summary"),
		Body:        maputil.StringFieldAlt(m, "jira_description", "description"),
		IssueType:   maputil.StringFieldAlt(m, "jira_issue_type", "issue_type"),
		StoryPoints: maputil.IntFieldAlt(m, "jira_story_points", "story_points"),
	}
}
