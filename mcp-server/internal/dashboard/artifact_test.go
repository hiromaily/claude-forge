package dashboard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArtifactHandler_Success verifies that a valid workspace + .md file
// combination returns the file content with the expected headers.
func TestArtifactHandler_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "# Design\n\nThis is the design doc."
	if err := os.WriteFile(filepath.Join(dir, "design.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	handler := artifactHandler()
	req := httptest.NewRequest(http.MethodGet,
		"/api/artifact?workspace="+dir+"&file=design.md", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type: got %q, want text/plain prefix", ct)
	}
	if got := rec.Body.String(); got != content {
		t.Errorf("body: got %q, want %q", got, content)
	}
}

// TestArtifactHandler_PathTraversal verifies that directory traversal
// attempts are rejected with 403.
func TestArtifactHandler_PathTraversal(t *testing.T) {
	t.Parallel()

	handler := artifactHandler()
	cases := []struct {
		name string
		file string
	}{
		{name: "dot_dot_slash", file: "../etc/passwd.md"},
		// Note: absolute paths like "/etc/passwd.md" are sanitised by
		// filepath.Join to "<workspace>/etc/passwd.md", staying inside the
		// workspace. Only relative traversal can escape.
		{name: "dot_dot_encoded", file: "..%2F..%2Fetc%2Fpasswd.md"},
		{name: "nested_traversal", file: "sub/../../outside.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			req := httptest.NewRequest(http.MethodGet,
				"/api/artifact?workspace="+dir+"&file="+tc.file, nil)
			req.RemoteAddr = "127.0.0.1:54321"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			// Should be 400 (bad extension) or 403 (path traversal).
			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
				t.Errorf("file=%q: got %d, want 400 or 403", tc.file, rec.Code)
			}
		})
	}
}

// TestArtifactHandler_NonMdFileRejected verifies that non-.md files are
// rejected with 400.
func TestArtifactHandler_NonMdFileRejected(t *testing.T) {
	t.Parallel()

	handler := artifactHandler()
	cases := []struct {
		name string
		file string
	}{
		{name: "go_file", file: "main.go"},
		{name: "json_file", file: "state.json"},
		{name: "no_extension", file: "README"},
		{name: "html_file", file: "index.html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			req := httptest.NewRequest(http.MethodGet,
				"/api/artifact?workspace="+dir+"&file="+tc.file, nil)
			req.RemoteAddr = "127.0.0.1:54321"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("file=%q: got %d, want 400", tc.file, rec.Code)
			}
		})
	}
}

// TestArtifactHandler_MissingParams verifies that missing query parameters
// return 400.
func TestArtifactHandler_MissingParams(t *testing.T) {
	t.Parallel()

	handler := artifactHandler()
	cases := []struct {
		name  string
		query string
	}{
		{name: "no_params", query: ""},
		{name: "workspace_only", query: "workspace=/tmp/ws"},
		{name: "file_only", query: "file=design.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			url := "/api/artifact"
			if tc.query != "" {
				url += "?" + tc.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req.RemoteAddr = "127.0.0.1:54321"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("query=%q: got %d, want 400", tc.query, rec.Code)
			}
		})
	}
}

// TestArtifactHandler_FileNotFound verifies that a valid .md path that does
// not exist on disk returns 404.
func TestArtifactHandler_FileNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	handler := artifactHandler()
	req := httptest.NewRequest(http.MethodGet,
		"/api/artifact?workspace="+dir+"&file=nonexistent.md", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
}

// TestArtifactHandler_Forbidden verifies that non-loopback requests are
// rejected with 403.
func TestArtifactHandler_Forbidden(t *testing.T) {
	t.Parallel()

	handler := artifactHandler()
	req := httptest.NewRequest(http.MethodGet,
		"/api/artifact?workspace=/tmp&file=design.md", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", rec.Code)
	}
}

// TestPhaseArtifactsHandler_ReturnsJSON verifies that the phase-artifacts
// endpoint returns a valid JSON map.
func TestPhaseArtifactsHandler_ReturnsJSON(t *testing.T) {
	t.Parallel()

	handler := phaseArtifactsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/phase-artifacts", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}

	body := rec.Body.String()
	// Verify key phases are present in the response.
	for _, phase := range []string{"phase-1", "phase-3", "phase-7"} {
		if !strings.Contains(body, phase) {
			t.Errorf("body missing phase %q: %s", phase, body)
		}
	}
}
