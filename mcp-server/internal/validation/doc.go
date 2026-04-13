// Package validation implements input and artifact validation logic for
// the forge-state MCP server.
//
// Key functions:
//   - [ValidateInput]: validates the raw pipeline input string (empty,
//     too-short, URL format) and parses it into a [ParsedInput] with
//     source type, core text, bare flags, and key-value flags.
//   - [ValidateArtifact]: checks that the expected artifact file exists
//     for a given phase and meets content constraints (non-empty, minimum
//     length, required sections).
//   - [ValidateWorkflowRules]: parses .specs/instructions.md for
//     declarative workflow rules and validates tasks.md against them.
//
// Import direction: validation → state (reads phase and artifact constants).
package validation
