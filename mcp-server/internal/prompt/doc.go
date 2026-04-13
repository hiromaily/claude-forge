// Package prompt assembles the 4-layer prompt passed to each pipeline agent
// via the spawn_agent action.
//
// The four layers are:
//   - Layer 1: Agent instructions (loaded from agents/{name}.md).
//   - Layer 2: Input/output artifact paths for the current phase.
//   - Layer 3: Repository profile context (languages, build commands, CI).
//   - Layer 4: Data flywheel — cross-pipeline learning injected from the
//     history package (similar pipelines, review patterns, friction points).
//
// The [Builder] constructs the final prompt string, applying a token budget
// (8 000 tokens) to Layer 4 content to avoid exceeding context limits.
//
// Import direction: prompt → history (reads search results and patterns).
package prompt
