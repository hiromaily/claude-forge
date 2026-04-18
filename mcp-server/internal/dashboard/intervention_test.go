package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// newTestStateManager returns a StateManager initialized at a fresh temp
// workspace, ready for intervention API tests. The returned workspace path
// is the absolute directory backing state.json.
func newTestStateManager(t *testing.T) (*state.StateManager, string) {
	t.Helper()
	dir := t.TempDir()
	sm := state.NewStateManager("test")
	if err := sm.Init(dir, "spec-intervention"); err != nil {
		t.Fatalf("StateManager.Init: %v", err)
	}
	return sm, dir
}

// jsonBody marshals v to JSON and wraps it in a *bytes.Buffer for use as a
// request body. Failures are reported via t.Fatalf because they indicate a
// bug in the test setup, not in the code under test.
func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("jsonBody marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

// readErrorBody decodes the standard {"error": "..."} response body for
// assertion in negative-path tests.
func readErrorBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("readErrorBody: %v", err)
	}
	var env map[string]string
	if err := json.Unmarshal(b, &env); err != nil {
		return string(b)
	}
	return env["error"]
}

// TestIsLocalRequest covers the loopback + Origin allowlist policy in one
// table so the safety contract is documented in a single place.
func TestIsLocalRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		remoteAddr string
		origin     string
		want       bool
	}{
		{name: "loopback_no_origin", remoteAddr: "127.0.0.1:54321", origin: "", want: true},
		{name: "loopback_v6_no_origin", remoteAddr: "[::1]:54321", origin: "", want: true},
		{name: "loopback_v6_with_ipv6_origin", remoteAddr: "[::1]:54321", origin: "http://[::1]:9876", want: true},
		{name: "loopback_with_localhost_origin", remoteAddr: "127.0.0.1:54321", origin: "http://localhost:9876", want: true},
		{name: "loopback_with_127_origin", remoteAddr: "127.0.0.1:54321", origin: "http://127.0.0.1:9876", want: true},
		{name: "non_loopback_rejected", remoteAddr: "192.168.1.10:54321", origin: "", want: false},
		{name: "loopback_but_foreign_origin", remoteAddr: "127.0.0.1:54321", origin: "http://evil.example.com", want: false},
		{name: "loopback_but_https_origin", remoteAddr: "127.0.0.1:54321", origin: "https://localhost:9876", want: false},
		{name: "loopback_but_file_origin", remoteAddr: "127.0.0.1:54321", origin: "file://", want: false},
		{name: "malformed_remote_addr_rejected", remoteAddr: "not-a-host", origin: "", want: false},
		// Host-suffix attack: a prefix-only check would accept this because
		// the string starts with "http://127.0.0.1:". Structural parsing
		// rejects it because the hostname resolves to "evil.example.com".
		{name: "loopback_but_userinfo_host_attack", remoteAddr: "127.0.0.1:54321", origin: "http://127.0.0.1:9876@evil.example.com", want: false},
		// Subdomain attack: "localhost.evil.example" must not be confused with "localhost".
		{name: "loopback_but_subdomain_attack", remoteAddr: "127.0.0.1:54321", origin: "http://localhost.evil.example", want: false},
		// Malformed Origin (unparseable) must be rejected, not accepted.
		{name: "malformed_origin_rejected", remoteAddr: "127.0.0.1:54321", origin: "://no-scheme", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodPost, "/api/checkpoint/approve", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := isLocalRequest(r); got != tc.want {
				t.Errorf("isLocalRequest(remote=%q origin=%q) = %v, want %v", tc.remoteAddr, tc.origin, got, tc.want)
			}
		})
	}
}

// TestApproveCheckpoint_Success drives a workspace into the awaiting_human
// state at checkpoint-a, then verifies that POST /api/checkpoint/approve
// advances the phase via PhaseComplete.
func TestApproveCheckpoint_Success(t *testing.T) {
	t.Parallel()

	sm, workspace := newTestStateManager(t)

	// Move state to checkpoint-a / awaiting_human via the public API the
	// orchestrator would use. We complete every preceding phase deterministically
	// instead of mutating state.json directly.
	for _, phase := range []string{"phase-1", "phase-2", "phase-3", "phase-3b"} {
		if err := sm.PhaseComplete(workspace, phase); err != nil {
			t.Fatalf("PhaseComplete %s: %v", phase, err)
		}
	}
	if err := sm.Checkpoint(workspace, "checkpoint-a"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	handler := approveCheckpointHandler(sm)
	req := httptest.NewRequest(http.MethodPost, "/api/checkpoint/approve",
		jsonBody(t, interventionRequest{Workspace: workspace, Phase: "checkpoint-a"}))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%q", resp.StatusCode, readErrorBody(t, resp))
	}

	if err := sm.LoadFromFile(workspace); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	s, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if s.CurrentPhase == "checkpoint-a" {
		t.Errorf("CurrentPhase did not advance: still %q", s.CurrentPhase)
	}
}

// TestApproveCheckpoint_MessageWritten verifies that when the checkpoint
// approve request includes a non-empty message, the handler writes the
// message to checkpoint-message.txt inside the workspace directory.
func TestApproveCheckpoint_MessageWritten(t *testing.T) {
	t.Parallel()

	sm, workspace := newTestStateManager(t)

	// Advance to checkpoint-a / awaiting_human.
	for _, phase := range []string{"phase-1", "phase-2", "phase-3", "phase-3b"} {
		if err := sm.PhaseComplete(workspace, phase); err != nil {
			t.Fatalf("PhaseComplete %s: %v", phase, err)
		}
	}
	if err := sm.Checkpoint(workspace, "checkpoint-a"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	handler := approveCheckpointHandler(sm)
	body := jsonBody(t, interventionRequest{
		Workspace: workspace,
		Phase:     "checkpoint-a",
		Message:   "Please add error handling to the parser module.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/checkpoint/approve", body)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%q", rec.Code, readErrorBody(t, rec.Result()))
	}

	// Verify checkpoint-message.txt was written with the expected content.
	msgFile := filepath.Join(workspace, "checkpoint-message.txt")
	data, err := os.ReadFile(msgFile)
	if err != nil {
		t.Fatalf("ReadFile checkpoint-message.txt: %v", err)
	}
	want := "Please add error handling to the parser module."
	if string(data) != want {
		t.Errorf("checkpoint-message.txt: got %q, want %q", string(data), want)
	}
}

// TestApproveCheckpoint_NoMessageNoFile verifies that when the checkpoint
// approve request has an empty message, no checkpoint-message.txt file is
// created.
func TestApproveCheckpoint_NoMessageNoFile(t *testing.T) {
	t.Parallel()

	sm, workspace := newTestStateManager(t)

	// Advance to checkpoint-a / awaiting_human.
	for _, phase := range []string{"phase-1", "phase-2", "phase-3", "phase-3b"} {
		if err := sm.PhaseComplete(workspace, phase); err != nil {
			t.Fatalf("PhaseComplete %s: %v", phase, err)
		}
	}
	if err := sm.Checkpoint(workspace, "checkpoint-a"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	handler := approveCheckpointHandler(sm)
	body := jsonBody(t, interventionRequest{
		Workspace: workspace,
		Phase:     "checkpoint-a",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/checkpoint/approve", body)
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%q", rec.Code, readErrorBody(t, rec.Result()))
	}

	// Verify no checkpoint-message.txt was created.
	msgFile := filepath.Join(workspace, "checkpoint-message.txt")
	if _, err := os.Stat(msgFile); err == nil {
		t.Errorf("checkpoint-message.txt should not exist when message is empty")
	}
}

// TestApproveCheckpoint_Forbidden covers all 403 cases: non-loopback caller,
// foreign Origin, and HTTPS origin (mismatched scheme).
func TestApproveCheckpoint_Forbidden(t *testing.T) {
	t.Parallel()

	sm, workspace := newTestStateManager(t)
	handler := approveCheckpointHandler(sm)

	cases := []struct {
		name       string
		remoteAddr string
		origin     string
	}{
		{name: "non_loopback", remoteAddr: "10.0.0.5:1234", origin: ""},
		{name: "foreign_origin", remoteAddr: "127.0.0.1:54321", origin: "http://evil.example.com"},
		{name: "https_origin", remoteAddr: "127.0.0.1:54321", origin: "https://localhost:9876"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/api/checkpoint/approve",
				jsonBody(t, interventionRequest{Workspace: workspace, Phase: "checkpoint-a"}))
			req.RemoteAddr = tc.remoteAddr
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("got %d, want 403", rec.Code)
			}
		})
	}
}

// TestApproveCheckpoint_BadRequest covers malformed payloads.
func TestApproveCheckpoint_BadRequest(t *testing.T) {
	t.Parallel()

	sm, _ := newTestStateManager(t)
	handler := approveCheckpointHandler(sm)

	cases := []struct {
		name string
		body string
	}{
		{name: "invalid_json", body: "{not-json"},
		{name: "missing_workspace", body: `{"phase":"checkpoint-a"}`},
		{name: "missing_phase", body: `{"workspace":"/tmp/x"}`},
		{name: "unknown_field", body: `{"workspace":"/tmp/x","phase":"checkpoint-a","extra":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/api/checkpoint/approve",
				strings.NewReader(tc.body))
			req.RemoteAddr = "127.0.0.1:54321"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("body %q: got %d, want 400", tc.body, rec.Code)
			}
		})
	}
}

// TestApproveCheckpoint_Conflict covers the two pre-flight conflict cases:
// the requested phase is not the current phase, and the current phase is not
// in awaiting_human status.
func TestApproveCheckpoint_Conflict(t *testing.T) {
	t.Parallel()

	t.Run("phase_mismatch", func(t *testing.T) {
		t.Parallel()
		sm, workspace := newTestStateManager(t)
		// Workspace is at phase-1 / pending. Caller asks for checkpoint-a.
		handler := approveCheckpointHandler(sm)
		req := httptest.NewRequest(http.MethodPost, "/api/checkpoint/approve",
			jsonBody(t, interventionRequest{Workspace: workspace, Phase: "checkpoint-a"}))
		req.RemoteAddr = "127.0.0.1:54321"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("got %d, want 409", rec.Code)
		}
		if msg := readErrorBody(t, rec.Result()); !strings.Contains(msg, "not at requested phase") {
			t.Errorf("error body lacks phase-mismatch reason: %q", msg)
		}
	})

	t.Run("not_awaiting_human", func(t *testing.T) {
		t.Parallel()
		sm, workspace := newTestStateManager(t)
		// At phase-1 / pending. Caller asks to approve phase-1, which is the
		// current phase but is not at awaiting_human status.
		handler := approveCheckpointHandler(sm)
		req := httptest.NewRequest(http.MethodPost, "/api/checkpoint/approve",
			jsonBody(t, interventionRequest{Workspace: workspace, Phase: "phase-1"}))
		req.RemoteAddr = "127.0.0.1:54321"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("got %d, want 409", rec.Code)
		}
		if msg := readErrorBody(t, rec.Result()); !strings.Contains(msg, "not awaiting human") {
			t.Errorf("error body lacks awaiting-human reason: %q", msg)
		}
	})
}

// TestAbandon_Success verifies the happy path and idempotency of the abandon endpoint.
func TestAbandon_Success(t *testing.T) {
	t.Parallel()

	sm, workspace := newTestStateManager(t)
	handler := abandonHandler(sm)

	req := httptest.NewRequest(http.MethodPost, "/api/pipeline/abandon",
		jsonBody(t, interventionRequest{Workspace: workspace}))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body=%q", rec.Code, readErrorBody(t, rec.Result()))
	}

	if err := sm.LoadFromFile(workspace); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	s, err := sm.GetState()
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if s.CurrentPhaseStatus != state.StatusAbandoned {
		t.Errorf("CurrentPhaseStatus: got %q, want %q", s.CurrentPhaseStatus, state.StatusAbandoned)
	}

	// Second call must 409: cannot abandon an already-abandoned pipeline.
	req2 := httptest.NewRequest(http.MethodPost, "/api/pipeline/abandon",
		jsonBody(t, interventionRequest{Workspace: workspace}))
	req2.RemoteAddr = "127.0.0.1:54321"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Errorf("second abandon: got %d, want 409", rec2.Code)
	}
}

// TestAbandon_Forbidden verifies the loopback + Origin guard also gates
// /api/pipeline/abandon.
func TestAbandon_Forbidden(t *testing.T) {
	t.Parallel()

	sm, workspace := newTestStateManager(t)
	handler := abandonHandler(sm)

	req := httptest.NewRequest(http.MethodPost, "/api/pipeline/abandon",
		jsonBody(t, interventionRequest{Workspace: workspace}))
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rec.Code)
	}
}

// TestInterventionRoutes_RegisteredOnHTTPServer verifies that the intervention
// routes are wired onto the same listener that serves /events and /, so a
// dashboard can reach them at the same origin without CORS gymnastics.
func TestInterventionRoutes_RegisteredOnHTTPServer(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	bus := events.NewEventBus()
	sm, _ := newTestStateManager(t)

	srv := Start(port, bus, sm, nil)
	if srv == nil {
		t.Fatal("Start returned nil")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	time.Sleep(20 * time.Millisecond)

	cases := []struct {
		name string
		path string
	}{
		{name: "approve_route", path: "/api/checkpoint/approve"},
		{name: "abandon_route", path: "/api/pipeline/abandon"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			url := fmt.Sprintf("http://localhost:%s%s", port, tc.path)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(`{}`))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST %s: %v", tc.path, err)
			}
			defer func() { _ = resp.Body.Close() }()

			// 400 (missing fields) proves the route exists and reached the
			// handler. 404 would prove the route was not registered.
			if resp.StatusCode == http.StatusNotFound {
				t.Errorf("route %s returned 404 — handler not registered on mux", tc.path)
			}
		})
	}
}
