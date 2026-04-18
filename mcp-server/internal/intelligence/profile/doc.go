// Package profile detects and caches repository metadata for injection into
// agent prompts as Layer 3 context.
//
// The [Analyzer] scans the repository root to detect languages, CI system,
// linter configurations, and build/test/lint commands. Results are cached
// in .specs/repo-profile.json and refreshed when the cache is older than
// a configurable TTL.
//
// The profile_get MCP tool returns the cached profile, triggering
// [Analyzer.AnalyzeOrUpdate] on first call.
//
// Import direction: profile has no internal dependencies (leaf package).
package profile
