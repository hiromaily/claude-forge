# Source Type Registry + maputil Refactoring Design

## Problem

1. **DRY violations**: `parseStoryPoints`/`parseEstimate` are near-identical. Label array parsing is duplicated. The marshal-unmarshal round-trip pattern appears 4+ times.
2. **Source type knowledge scattered**: Adding a new source type requires changes in 8 locations across 5 packages with no compile-time enforcement.
3. **No shared utility for map field extraction**: `stringField`, `boolField`, etc. are private to `tools` but needed by the new `sourcetype` package.

## Goal

- Extract `maputil` package for generic map field helpers
- Create `sourcetype` package with Handler interface and registry
- Eliminate all source-type switch statements from `tools` and `orchestrator`
- Achieve compile-time enforcement: forgetting to implement a Handler method is a build error

## Design

### 1. `internal/maputil` Package

Extract generic map extraction helpers from `context_fetcher.go`.

```go
// internal/maputil/fields.go
package maputil

func StringField(m map[string]any, key string) string
func StringFieldAlt(m map[string]any, primary, alt string) string
func BoolField(m map[string]any, key string) bool
func IntFieldAlt(m map[string]any, primary, alt string) int  // merges parseStoryPoints + parseEstimate
func StringArray(m map[string]any, key string) []string       // merges github_labels + linear_labels parsing
func ToMap(raw any) (map[string]any, error)                   // merges marshal-unmarshal round-trip
```

`IntFieldAlt` unifies `parseStoryPoints` and `parseEstimate` into one function that:
1. Checks the primary key, falls back to the alt key
2. Handles `float64`, `int`, `json.Number` types

`StringArray` unifies the label parsing pattern (handles `[]any` and `[]string`).

`ToMap` replaces the 4+ instances of `json.Marshal(raw)` then `json.Unmarshal(data, &m)`.

### 2. `internal/sourcetype` Package

#### Types

```go
// internal/sourcetype/types.go
package sourcetype

// FetchConfig describes how to fetch external issue data.
// At most one of MCPTool or Command should drive the fetch.
type FetchConfig struct {
    Type            string            `json:"type"`
    MCPTool         string            `json:"mcp_tool,omitempty"`
    Command         string            `json:"command,omitempty"`
    MCPParams       map[string]string `json:"mcp_params,omitempty"`
    ResponseMapping map[string]string `json:"response_mapping"`
    Instruction     string            `json:"instruction"`
}

// PostConfig describes how to post a comment back to the source issue.
// At most one of MCPTool, Command, or Instruction should be set.
type PostConfig struct {
    MCPTool     string            `json:"mcp_tool,omitempty"`
    Command     string            `json:"command,omitempty"`
    MCPParams   map[string]string `json:"mcp_params,omitempty"`
    BodySource  string            `json:"body_source"`
    Instruction string            `json:"instruction,omitempty"`
}

// ExternalFields holds parsed issue fields from an external source.
// Each handler populates only its own fields.
type ExternalFields struct {
    Title       string
    Body        string   // GitHub body, Jira/Linear description
    Labels      []string
    IssueType   string   // Jira only
    StoryPoints int      // Jira story points or Linear estimate
}
```

`ExternalFields` is a **unified, service-neutral** struct. This replaces the per-service fields on `externalContext` (e.g., `GitHubTitle`, `JiraSummary`, `LinearTitle` all map to `Title`). The `externalContext` struct in `tools` remains but becomes thinner — it holds `SourceURL`, `SourceID`, `TaskText`, and an `ExternalFields` from the handler.

#### Handler Interface

```go
// internal/sourcetype/handler.go
package sourcetype

import "regexp"

// Handler defines the contract for a source type integration.
// Each source type (GitHub, Jira, Linear) implements this interface.
type Handler interface {
    // Type returns the source type constant (e.g., "github_issue").
    Type() string

    // Label returns a human-readable label (e.g., "GitHub issue").
    Label() string

    // URLPattern returns the compiled regex that validates a full URL.
    URLPattern() *regexp.Regexp

    // BasePattern returns the compiled regex that matches the URL base
    // (used to identify which handler to try before full validation).
    BasePattern() *regexp.Regexp

    // InvalidURLMessage returns the error message for malformed URLs.
    InvalidURLMessage() string

    // ExtractSourceID extracts the source identifier from a validated URL.
    ExtractSourceID(rawURL string) string

    // FetchConfig returns the fetch configuration for this source type.
    FetchConfig(sourceURL, sourceID string) *FetchConfig

    // PostConfig returns the post configuration for this source type.
    // workspace is the spec workspace path (e.g., ".specs/20260418-dea-13-fix").
    PostConfig(sourceURL, sourceID, workspace, artifactName string) *PostConfig

    // ParseExternalContext extracts typed fields from the raw external_context map.
    ParseExternalContext(m map[string]any) ExternalFields

    // SupportsClosingRef returns true if this source type supports auto-close
    // references in PR bodies (e.g., "Closes #42" for GitHub).
    SupportsClosingRef() bool
}
```

#### Registry

```go
// internal/sourcetype/registry.go
package sourcetype

// registry holds all registered source type handlers.
var registry []Handler

func init() {
    registry = []Handler{
        &GitHubHandler{},
        &JiraHandler{},
        &LinearHandler{},
    }
}

// Get returns the Handler for the given source type, or nil if not found.
func Get(sourceType string) Handler {
    for _, h := range registry {
        if h.Type() == sourceType {
            return h
        }
    }
    return nil
}

// All returns all registered handlers.
func All() []Handler {
    return registry
}

// IsURLSource returns true if the source type has a registered handler.
func IsURLSource(sourceType string) bool {
    return Get(sourceType) != nil
}

// ValidateURL checks the URL against all registered handlers.
// Returns (sourceType, nil) on match, or an error listing supported formats.
func ValidateURL(url string) (string, error) {
    for _, h := range All() {
        if h.BasePattern().MatchString(url) {
            if h.URLPattern().MatchString(url) {
                return h.Type(), nil
            }
            return "", fmt.Errorf("ERROR: %s", h.InvalidURLMessage())
        }
    }
    // Build supported formats from all handlers
    return "", fmt.Errorf("ERROR: Unrecognised URL format. Supported: ...")
}
```

#### Handler Implementations

Each handler is a single file. Example for Linear:

```go
// internal/sourcetype/linear.go
package sourcetype

import (
    "regexp"
    "github.com/hiromaily/claude-forge/mcp-server/internal/maputil"
)

var (
    reLinearURL  = regexp.MustCompile(`^https://linear\.app/[^/]+/issue/[A-Z]+-[0-9]+`)
    reLinearBase = regexp.MustCompile(`^https://linear\.app/`)
)

type LinearHandler struct{}

func (h *LinearHandler) Type() string              { return "linear_issue" }
func (h *LinearHandler) Label() string             { return "Linear issue" }
func (h *LinearHandler) URLPattern() *regexp.Regexp { return reLinearURL }
func (h *LinearHandler) BasePattern() *regexp.Regexp { return reLinearBase }
func (h *LinearHandler) InvalidURLMessage() string {
    return "Invalid Linear URL format. Expected: https://linear.app/{org}/issue/{KEY}-{number}"
}
func (h *LinearHandler) SupportsClosingRef() bool   { return false }

func (h *LinearHandler) ExtractSourceID(rawURL string) string {
    // parse URL, find segment after "issue"
    // ... (moved from pipeline_init.go extractSourceID)
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

func (h *LinearHandler) PostConfig(sourceURL, sourceID, workspace, artifactName string) *PostConfig {
    return &PostConfig{
        MCPTool:    "mcp__linear__save_comment",
        MCPParams:  map[string]string{"issueId": sourceID},
        BodySource: filepath.Join(workspace, artifactName),
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
```

### 3. Consumer Changes

#### `validation/input.go`

Replace the hardcoded regex vars and `validateURL` switch with:

```go
func validateURL(core string, flags map[string]string, bareFlags []string) InputResult {
    sourceType, err := sourcetype.ValidateURL(core)
    if err != nil {
        return InputResult{Valid: false, Errors: []string{err.Error()}}
    }
    return InputResult{
        Valid: true,
        Parsed: ParsedInput{
            Flags: flags, BareFlags: normalizeBareFlags(bareFlags),
            CoreText: core, SourceType: sourceType,
        },
    }
}
```

The regex patterns are removed from `input.go` and live in each handler.

#### `tools/pipeline_init.go`

- `extractSourceID`: delegate to `sourcetype.Get(sourceType).ExtractSourceID(url)`
- `makeFetchNeeded`: delegate to `sourcetype.Get(sourceType).FetchConfig(url, id)`
- `sourceURL` check: use `sourcetype.IsURLSource(sourceType)` instead of listing types

#### `tools/context_fetcher.go`

- `externalContext` struct simplified:

```go
type externalContext struct {
    SourceURL string
    SourceID  string
    TaskText  string
    Fields    sourcetype.ExternalFields  // replaces per-service fields
}
```

- `parseExternalContext`: looks up handler, delegates parsing:

```go
func parseExternalContext(args map[string]any, sourceType string) (externalContext, error) {
    // ... parse source_url, source_id, external_context map ...
    h := sourcetype.Get(sourceType)
    if h != nil && m != nil {
        extCtx.Fields = h.ParseExternalContext(m)
    }
    return extCtx, nil
}
```

- `IsTextSource()` becomes: `ec.Fields == (sourcetype.ExternalFields{})`
- `buildRequestMDWithBody`: uses `ec.Fields.Title` and `ec.Fields.Body` generically — no service-specific switch needed:

```go
if ec.Fields.Title != "" || ec.Fields.Body != "" {
    sourceType = /* from ec.SourceURL or stored source_type */
    resolvedBody = strings.TrimSpace(ec.Fields.Title + "\n\n" + ec.Fields.Body)
}
```

However, `buildRequestMDWithBody` needs the source_type string for front matter. This requires passing `sourceType` through or storing it in `externalContext`.

**Decision**: Add `SourceType string` to `externalContext`. It's already available from `pipeline_init` result.

#### `orchestrator/engine.go`

- `sourceTypeLabel`: `sourcetype.Get(st).Label()`
- `buildPostMethod`: `sourcetype.Get(st).PostConfig(url, id, workspace, artifact)`
- `handlePRCreation` closing ref: `sourcetype.Get(st).SupportsClosingRef()`

`PostMethod` type in `actions.go` becomes `*sourcetype.PostConfig`. The import direction `orchestrator → sourcetype` is valid.

Similarly, `FetchNeeded` in `pipeline_init.go` (`PipelineInitResult`) becomes `*sourcetype.FetchConfig`.

#### `tools/pipeline_init_with_context.go`

- `isTextSource` check: uses `extCtx.IsTextSource()` (already refactored)
- Combined text for effort detection: `extCtx.Fields.Title + " " + extCtx.Fields.Body + " " + extCtx.TaskText`
- Story points: `extCtx.Fields.StoryPoints`

### 4. `refineWorkspacePath` Changes

Currently has a long switch on Jira/GitHub/Linear title fields. With unified `ExternalFields`:

```go
func refineWorkspacePath(workspace string, extCtx externalContext) string {
    summary := extCtx.Fields.Title
    if summary == "" {
        return workspace
    }
    id := extCtx.SourceID
    combined := summary
    if id != "" {
        combined = id + " " + summary
    }
    return replaceWorkspaceSlug(workspace, slugifyOrDefault(combined))
}
```

The 8-case switch becomes 4 lines.

### 5. Import Direction

```
tools ──→ sourcetype ←── orchestrator
              │
              ▼
            state (constants only)
            maputil (field extraction)
```

- `sourcetype` imports `state` (for `SourceType*` constants) and `maputil`
- `tools` imports `sourcetype` (for types and `Get()`)
- `orchestrator` imports `sourcetype` (for types and `Get()`)
- No circular dependency

### 6. Compile-Time Enforcement

Adding a new service requires:
1. Create `internal/sourcetype/newservice.go` implementing `Handler`
2. Add it to `registry` in `registry.go`

If any method is missing, the compiler rejects `&NewServiceHandler{}` as not implementing `Handler`. No other files need updating.

## Files to Change

### New Files

| File | Content |
|------|---------|
| `internal/maputil/fields.go` | Exported field extraction helpers |
| `internal/maputil/fields_test.go` | Tests |
| `internal/sourcetype/types.go` | FetchConfig, PostConfig, ExternalFields |
| `internal/sourcetype/handler.go` | Handler interface |
| `internal/sourcetype/registry.go` | Registry, Get(), All(), ValidateURL() |
| `internal/sourcetype/registry_test.go` | Registry tests |
| `internal/sourcetype/github.go` | GitHubHandler |
| `internal/sourcetype/jira.go` | JiraHandler |
| `internal/sourcetype/linear.go` | LinearHandler |
| `internal/sourcetype/github_test.go` | Tests |
| `internal/sourcetype/jira_test.go` | Tests |
| `internal/sourcetype/linear_test.go` | Tests |

### Modified Files

| File | Change |
|------|--------|
| `validation/input.go` | Remove regex vars, delegate validateURL to sourcetype.ValidateURL |
| `validation/input_test.go` | No changes needed (black-box tests) |
| `tools/pipeline_init.go` | Remove FetchNeeded struct, extractSourceID, makeFetchNeeded. Use sourcetype. |
| `tools/context_fetcher.go` | Simplify externalContext, remove parseStoryPoints/parseEstimate/stringField/etc. Use maputil + sourcetype |
| `tools/pipeline_init_with_context.go` | Simplify effort detection and isTextSource |
| `tools/pipeline_init_test.go` | Update FetchConfig type references |
| `tools/context_fetcher_test.go` | Update for simplified externalContext |
| `tools/pipeline_init_with_context_test.go` | Minor updates |
| `orchestrator/actions.go` | Replace PostMethod with sourcetype.PostConfig reference |
| `orchestrator/engine.go` | Remove sourceTypeLabel, buildPostMethod. Use sourcetype.Get() |
| `orchestrator/engine_test.go` | Update PostConfig type references |

### Deleted Code (moved to new packages)

| From | What | To |
|------|------|----|
| `context_fetcher.go` | `stringField`, `stringFieldAlt`, `boolField` | `maputil/fields.go` |
| `context_fetcher.go` | `parseStoryPoints`, `parseEstimate` | `maputil/fields.go` as `IntFieldAlt` |
| `context_fetcher.go` | label array parsing | `maputil/fields.go` as `StringArray` |
| `pipeline_init_with_context.go` | marshal-unmarshal round-trip | `maputil/fields.go` as `ToMap` |
| `pipeline_init.go` | `FetchNeeded` struct | `sourcetype/types.go` as `FetchConfig` |
| `pipeline_init.go` | `makeFetchNeeded` | `sourcetype/*.go` as `FetchConfig()` |
| `pipeline_init.go` | `extractSourceID` | `sourcetype/*.go` as `ExtractSourceID()` |
| `validation/input.go` | URL regex patterns | `sourcetype/*.go` as `URLPattern()`/`BasePattern()` |
| `orchestrator/actions.go` | `PostMethod` struct | `sourcetype/types.go` as `PostConfig` |
| `orchestrator/engine.go` | `sourceTypeLabel`, `buildPostMethod` | `sourcetype/*.go` |

## Testing Strategy

- **maputil**: Unit tests for each helper. Table-driven. Cover type coercion edge cases.
- **sourcetype handlers**: Each handler has its own test file. Test URL validation, source ID extraction, FetchConfig, PostConfig, ParseExternalContext.
- **sourcetype registry**: Test Get(), All(), ValidateURL() with all known types + unknown type.
- **Existing tests**: `validation/input_test.go` and `pipeline_init_test.go` are black-box tests that should pass without changes (behavior unchanged). `context_fetcher_test.go` and `engine_test.go` need type reference updates.

## Trade-offs

- **Pro**: New service = 1 file + 1 registry line. Compile-time enforcement.
- **Pro**: DRY violations eliminated. `parseStoryPoints`/`parseEstimate` unified. Label parsing unified.
- **Pro**: `externalContext` simplified from 13 service-specific fields to 1 `ExternalFields`.
- **Con**: New abstraction layer (Handler interface). More files.
- **Con**: `sourcetype` package needs `maputil` — adds a dependency edge. Acceptable since `maputil` is a leaf package.
- **Con**: Existing tests need updates for type changes (`FetchNeeded` → `FetchConfig`, `PostMethod` → `PostConfig`).
