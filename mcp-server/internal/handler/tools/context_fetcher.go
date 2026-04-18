// external context parsing and request.md construction.

package tools

import (
	"fmt"
	"strings"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/sourcetype"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/maputil"
)

// externalContext holds parsed source context fields in a service-neutral form.
type externalContext struct {
	SourceURL  string
	SourceID   string
	SourceType string
	TaskText   string
	Fields     sourcetype.ExternalFields
}

// IsTextSource returns true when no external fields are populated.
// Used by handleFirstCall to decide whether --discuss is applicable.
func (ec externalContext) IsTextSource() bool {
	return ec.Fields.IsEmpty()
}

// parseExternalContext extracts source context fields from the args map,
// delegating to the appropriate sourcetype handler for field extraction.
func parseExternalContext(args map[string]any, sourceType string) (externalContext, error) {
	var extCtx externalContext
	extCtx.SourceType = sourceType

	raw, ok := args["external_context"]
	if !ok || raw == nil {
		return extCtx, nil
	}

	m, ok := raw.(map[string]any)
	if !ok {
		return extCtx, fmt.Errorf("external_context must be an object, got %T", raw)
	}

	extCtx.SourceURL = maputil.StringField(m, "source_url")
	extCtx.SourceID = maputil.StringField(m, "source_id")

	h := sourcetype.Get(sourceType)
	if h != nil {
		extCtx.Fields = h.ParseExternalContext(m)
	}

	return extCtx, nil
}

// detectSourceTypeFromFields infers the source type from the field name prefixes
// in external_context. This is a backward-compatibility fallback for cases where
// source_url is not provided but prefixed fields (github_*, jira_*, linear_*) are.
func detectSourceTypeFromFields(args map[string]any) string {
	ec, ok := args["external_context"].(map[string]any)
	if !ok {
		return ""
	}
	return sourcetype.ClassifyByFieldPrefix(ec)
}

// buildRequestMDWithBody constructs the request.md content.
// For URL source types: uses the external fields. For text: uses the body parameter directly.
func buildRequestMDWithBody(extCtx externalContext, body string) string {
	var sb strings.Builder

	sourceType := "text"
	var resolvedBody string

	if !extCtx.Fields.IsEmpty() && extCtx.SourceType != "" {
		sourceType = extCtx.SourceType
		resolvedBody = strings.TrimSpace(extCtx.Fields.Title + "\n\n" + extCtx.Fields.Body)
	} else {
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
