// artifact.go serves workspace artifact files (markdown documents generated
// during pipeline phases) to the dashboard.
//
// The endpoint is:
//
//	GET /api/artifact?workspace=<abs-path>&file=<relative-name>
//
// Safety: only .md files within the workspace directory are served.
// The handler validates that the resolved path stays inside the workspace
// and rejects directory traversal attempts.

package dashboard

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// artifactHandler serves markdown artifact files from a workspace directory.
// Query parameters:
//   - workspace: absolute path to the workspace directory
//   - file: relative filename (e.g. "analysis.md", "design.md")
//
// Only .md files are served. Path traversal is blocked.
func artifactHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			httpError(w, http.StatusForbidden, "forbidden: requires loopback request")
			return
		}

		workspace := r.URL.Query().Get("workspace")
		file := r.URL.Query().Get("file")
		if workspace == "" || file == "" {
			httpError(w, http.StatusBadRequest, "workspace and file query parameters are required")
			return
		}

		// Only allow .md files.
		if !strings.HasSuffix(file, ".md") {
			httpError(w, http.StatusBadRequest, "only .md files are allowed")
			return
		}

		// Resolve and validate the path stays within the workspace.
		absWorkspace, err := filepath.Abs(workspace)
		if err != nil {
			httpError(w, http.StatusBadRequest, "invalid workspace path")
			return
		}
		resolved := filepath.Join(absWorkspace, filepath.Clean(file))
		if !strings.HasPrefix(resolved, absWorkspace+string(filepath.Separator)) {
			httpError(w, http.StatusForbidden, "path traversal detected")
			return
		}

		data, err := os.ReadFile(resolved) //nolint:gosec // G703: path is validated to stay within workspace via prefix check above
		if err != nil {
			if os.IsNotExist(err) {
				httpError(w, http.StatusNotFound, "artifact not found")
				return
			}
			httpError(w, http.StatusInternalServerError, "failed to read artifact")
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data) //nolint:gosec // G705: Content-Type is text/plain; no XSS risk
	}
}

// phaseArtifacts maps phase IDs to their output artifact filenames.
// This is served as JSON so the dashboard knows which files to offer for viewing.
//
// NOTE: The dashboard.html JS also has a checkpointArtifactMap that lists
// related artifacts to show at checkpoint events. When adding phases here,
// check whether the JS map needs updating too (and vice versa).
var phaseArtifacts = map[string][]string{
	"phase-1":            {"analysis.md"},
	"phase-2":            {"investigation.md"},
	"phase-3":            {"design.md"},
	"phase-3b":           {"review-design.md"},
	"phase-4":            {"tasks.md"},
	"phase-4b":           {"review-tasks.md"},
	"phase-5":            {}, // impl-{N}.md — dynamic, handled client-side
	"phase-6":            {}, // review-{N}.md — dynamic
	"phase-7":            {"comprehensive-review.md"},
	"final-verification": {"final-verification.md"},
	"final-summary":      {"summary.md"},
}

// phaseArtifactsHandler serves the phase → artifact filename map as JSON.
func phaseArtifactsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			httpError(w, http.StatusForbidden, "forbidden: requires loopback request")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		writeJSON(w, http.StatusOK, phaseArtifacts)
	}
}
