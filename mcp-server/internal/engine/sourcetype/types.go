package sourcetype

// FetchConfig describes how to fetch external issue data.
type FetchConfig struct {
	Type            string            `json:"type"`
	MCPTool         string            `json:"mcp_tool,omitempty"`
	Command         string            `json:"command,omitempty"`
	MCPParams       map[string]string `json:"mcp_params,omitempty"`
	ResponseMapping map[string]string `json:"response_mapping"`
	Instruction     string            `json:"instruction"`
}

// PostConfig describes how to post a comment back to the source issue.
type PostConfig struct {
	MCPTool     string            `json:"mcp_tool,omitempty"`
	Command     string            `json:"command,omitempty"`
	MCPParams   map[string]string `json:"mcp_params,omitempty"`
	BodySource  string            `json:"body_source"`
	Instruction string            `json:"instruction,omitempty"`
}

// ExternalFields holds parsed issue fields in a service-neutral form.
type ExternalFields struct {
	Title       string
	Body        string
	Labels      []string
	IssueType   string
	StoryPoints int
}

// CombinedText returns a single string of all text fields for effort detection.
func (ef ExternalFields) CombinedText() string {
	return ef.Title + " " + ef.Body
}

// IsEmpty returns true when no fields are populated.
func (ef ExternalFields) IsEmpty() bool {
	return ef.Title == "" && ef.Body == "" && ef.IssueType == "" && len(ef.Labels) == 0 && ef.StoryPoints == 0
}
