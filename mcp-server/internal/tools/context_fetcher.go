// Package tools — external context parsing and request.md construction.
package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// externalContext holds parsed GitHub/Jira context fields.
type externalContext struct {
	// Source identifiers from pipeline_init result — used in request.md front matter.
	SourceURL string
	SourceID  string
	// TaskText is the original task text for text source type pipelines.
	// Populated from the top-level task_text MCP parameter (not from external_context map).
	TaskText string
	// GitHub fields
	GitHubLabels []string
	GitHubTitle  string
	GitHubBody   string
	// Jira fields
	JiraIssueType   string
	JiraStoryPoints int
	JiraSummary     string
	JiraDescription string
}

// parseExternalContext extracts GitHub/Jira context fields from the args map.
func parseExternalContext(args map[string]any) (externalContext, error) {
	var extCtx externalContext

	raw, ok := args["external_context"]
	if !ok || raw == nil {
		return extCtx, nil
	}

	// Round-trip through JSON to normalize types.
	data, err := json.Marshal(raw)
	if err != nil {
		return extCtx, fmt.Errorf("marshal external_context: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return extCtx, fmt.Errorf("unmarshal external_context: %w", err)
	}

	extCtx.SourceURL = stringField(m, "source_url")
	extCtx.SourceID = stringField(m, "source_id")
	extCtx.GitHubTitle = stringField(m, "github_title")
	extCtx.GitHubBody = stringField(m, "github_body")
	extCtx.JiraIssueType = stringFieldAlt(m, "jira_issue_type", "issue_type")
	extCtx.JiraSummary = stringFieldAlt(m, "jira_summary", "summary")
	extCtx.JiraDescription = stringFieldAlt(m, "jira_description", "description")

	// Parse github_labels (array of strings).
	if labelsRaw, ok := m["github_labels"]; ok {
		switch v := labelsRaw.(type) {
		case []any:
			for _, l := range v {
				if s, ok := l.(string); ok {
					extCtx.GitHubLabels = append(extCtx.GitHubLabels, s)
				}
			}
		case []string:
			extCtx.GitHubLabels = v
		}
	}

	// Parse jira_story_points (number), with "story_points" as fallback alias.
	if _, ok := m["jira_story_points"]; !ok {
		if sp, ok2 := m["story_points"]; ok2 {
			m["jira_story_points"] = sp
		}
	}
	if spRaw, ok := m["jira_story_points"]; ok {
		switch v := spRaw.(type) {
		case float64:
			extCtx.JiraStoryPoints = int(v)
		case int:
			extCtx.JiraStoryPoints = v
		case json.Number:
			n, _ := v.Int64()
			extCtx.JiraStoryPoints = int(n)
		}
	}

	return extCtx, nil
}

// stringField extracts a string value from a map by key.
func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// stringFieldAlt tries the primary key first, then falls back to the alt key.
// This allows callers to pass either "jira_summary" or "summary" as the field name.
func stringFieldAlt(m map[string]any, primary, alt string) string {
	if s := stringField(m, primary); s != "" {
		return s
	}
	return stringField(m, alt)
}

// boolField extracts a bool value from a map by key.
func boolField(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

// buildRequestMDWithBody constructs the request.md content.
// For text source type: uses the body parameter directly (enables both task_text passthrough
// and discussion-enriched body). For github_issue/jira_issue: ignores body and uses the
// GitHub/Jira fields as before.
func buildRequestMDWithBody(extCtx externalContext, body string) string {
	var sb strings.Builder

	// Determine source_type and body.
	sourceType := "text"
	var resolvedBody string

	switch {
	case extCtx.GitHubTitle != "" || extCtx.GitHubBody != "":
		sourceType = "github_issue"
		resolvedBody = strings.TrimSpace(extCtx.GitHubTitle + "\n\n" + extCtx.GitHubBody)
	case extCtx.JiraIssueType != "" || extCtx.JiraSummary != "" || extCtx.JiraDescription != "":
		sourceType = "jira_issue"
		resolvedBody = strings.TrimSpace(extCtx.JiraSummary + "\n\n" + extCtx.JiraDescription)
	default:
		// text source: use the body parameter directly.
		resolvedBody = body
	}

	sb.WriteString("---\n")
	sb.WriteString("source_type: ")
	sb.WriteString(sourceType)
	sb.WriteString("\n")
	if extCtx.SourceURL != "" {
		sb.WriteString("source_url: ")
		sb.WriteString(extCtx.SourceURL)
		sb.WriteString("\n")
	}
	if extCtx.SourceID != "" {
		sb.WriteString("source_id: ")
		sb.WriteString(extCtx.SourceID)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")

	if resolvedBody != "" {
		sb.WriteString("\n")
		sb.WriteString(resolvedBody)
		sb.WriteString("\n")
	}

	return sb.String()
}

// buildEnrichedRequestBody combines the original task text with the discussion answers
// in a structured markdown format.
func buildEnrichedRequestBody(taskText, discussionAnswers string) string {
	return fmt.Sprintf("%s\n\n## Discussion Answers\n\n%s", taskText, discussionAnswers)
}

// defaultDiscussionQuestions returns the set of generic clarification questions
// presented to the user when --discuss is active.
func defaultDiscussionQuestions() []string {
	return []string{
		"What is the expected outcome or definition of done for this task?",
		"Are there any constraints, non-goals, or out-of-scope items?",
		"Are there any specific implementation details, preferences, or context the agent should know?",
	}
}
