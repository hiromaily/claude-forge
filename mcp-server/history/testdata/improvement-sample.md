# Improvement Report: Sample Pipeline Run

## Summary

This report identifies areas where the AI agents struggled during the pipeline run
and recommends mitigations for future runs.

## Friction Points Observed

### Documentation

- The implementer failed to document exported functions properly. Public APIs were
  missing godoc comments, making it hard to understand the purpose of each function.
- Mitigation: Enforce a linting rule that requires documentation on all exported symbols.

### Error Handling

- Several functions ignored error return values from file I/O operations. This caused
  silent failures that were only detected during integration testing.
- The agent did not check errors from json.Marshal calls, assuming they never fail.
- Mitigation: Use errcheck linter and review all deferred close calls.

### Test Coverage

- Unit tests were missing for the critical path in the parser. Only happy-path cases
  were covered, leaving edge cases untested.
- Mitigation: Require minimum test coverage thresholds and add table-driven tests.

### Naming Convention

- Variable names were abbreviated inconsistently: `idx` vs `index`, `err` vs `e`.
- Struct field names used mixed conventions (camelCase vs snake_case in JSON tags).
- Mitigation: Adopt and enforce a consistent naming guide in the project CLAUDE.md.

### Performance

- The implementation used a naive O(n^2) scan where a map lookup would suffice.
- Repeated string concatenation in a loop caused excessive allocations.
- Mitigation: Profile hot paths and replace linear scans with map-based lookups.
