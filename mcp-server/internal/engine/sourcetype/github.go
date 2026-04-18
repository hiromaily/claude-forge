package sourcetype

import (
	"net/url"
	"path"
	"regexp"

	"github.com/hiromaily/claude-forge/mcp-server/pkg/maputil"
)

var (
	reGitHubURL  = regexp.MustCompile(`^https://github\.com/[^/]+/[^/]+/issues/[0-9]+`)
	reGitHubBase = regexp.MustCompile(`^https://github\.com/`)
)

func init() { register(&GitHubHandler{}) }

type GitHubHandler struct{}

func (h *GitHubHandler) Type() string               { return "github_issue" }
func (h *GitHubHandler) Label() string              { return "GitHub issue" }
func (h *GitHubHandler) FieldPrefix() string        { return "github_" }
func (h *GitHubHandler) URLPattern() *regexp.Regexp  { return reGitHubURL }
func (h *GitHubHandler) BasePattern() *regexp.Regexp { return reGitHubBase }
func (h *GitHubHandler) SupportsClosingRef() bool    { return true }

func (h *GitHubHandler) InvalidURLMessage() string {
	return "Invalid GitHub URL format. Expected: https://github.com/{owner}/{repo}/issues/{number}"
}

func (h *GitHubHandler) ExtractSourceID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return path.Base(u.Path)
}

func (h *GitHubHandler) FetchConfig(sourceURL, sourceID string) *FetchConfig {
	return &FetchConfig{
		Type:    "github",
		Command: "gh issue view " + sourceURL + " --json title,body,labels",
		ResponseMapping: map[string]string{
			"title": "github_title", "body": "github_body", "labels": "github_labels",
		},
		Instruction: "fetch github issue fields before calling pipeline_init_with_context",
	}
}

func (h *GitHubHandler) PostConfig(sourceURL, sourceID, artifactPath string) *PostConfig {
	return &PostConfig{
		Command:    "gh issue comment " + sourceURL + " --body-file " + artifactPath,
		BodySource: artifactPath,
	}
}

func (h *GitHubHandler) ParseExternalContext(m map[string]any) ExternalFields {
	return ExternalFields{
		Title:  maputil.StringField(m, "github_title"),
		Body:   maputil.StringField(m, "github_body"),
		Labels: maputil.StringArray(m, "github_labels"),
	}
}
