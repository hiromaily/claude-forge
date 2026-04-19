// intervention.go exposes a tiny HTTP control API used by the embedded
// dashboard to approve checkpoints or abandon a pipeline without returning
// to the Claude Code session.
//
// The endpoints are intentionally minimal:
//
//	POST /api/checkpoint/approve  { "workspace": "...", "phase": "checkpoint-a" }
//	POST /api/pipeline/abandon    { "workspace": "..." }
//
// Both go through StateManager so the same guards that protect MCP tool calls
// also protect dashboard-originated calls.
//
// Safety contract — every request must:
//  1. Originate from a loopback address (127.0.0.1, ::1).
//  2. Either omit the Origin header (e.g. curl) OR carry an Origin pointing at
//     the same listener (http://localhost:<port> / http://127.0.0.1:<port>).
//
// Both checks must hold; failing either returns 403 Forbidden.

package dashboard

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
)

// interventionRequest is the JSON body shape accepted by intervention endpoints.
// Phase is required for /api/checkpoint/approve and ignored by /api/pipeline/abandon.
// Message is optional for /api/checkpoint/approve — when present, it is written to
// a checkpoint message file that the orchestrator can read as user_response.
type interventionRequest struct {
	Workspace string `json:"workspace"`
	Phase     string `json:"phase,omitempty"`
	Message   string `json:"message,omitempty"`
}

// approveCheckpointHandler completes the named checkpoint phase, mirroring the
// behaviour of typing "approve" at a checkpoint inside Claude Code.
//
// The handler refuses to advance any phase that is not currently awaiting
// human input, so a misfired button cannot accidentally skip an active
// agent run.
//
// When publicMode is true (FORGE_DASHBOARD_BIND_ALL=1) the loopback/origin
// safety check is skipped so devices on the local network can reach the API.
func approveCheckpointHandler(sm *state.StateManager, bus *events.EventBus, publicMode bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !publicMode && !isLocalRequest(r) {
			httpError(w, http.StatusForbidden, "forbidden: intervention API requires loopback request and same-origin")
			return
		}
		req, ok := decodeRequest(w, r)
		if !ok {
			return
		}
		if req.Workspace == "" || req.Phase == "" {
			httpError(w, http.StatusBadRequest, "workspace and phase are required")
			return
		}

		// Pre-flight: load state and verify we are at the requested checkpoint.
		// This is a guard in addition to the StateManager-internal validation,
		// because the dashboard may race against a phase transition that
		// happens between the SSE event and the user click.
		if err := sm.LoadFromFile(req.Workspace); err != nil {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("load state: %v", err))
			return
		}
		s, err := sm.GetState()
		if err != nil {
			httpError(w, http.StatusInternalServerError, fmt.Sprintf("get state: %v", err))
			return
		}
		if s.CurrentPhase != req.Phase {
			httpError(w, http.StatusConflict, fmt.Sprintf("not at requested phase: current=%q requested=%q", s.CurrentPhase, req.Phase))
			return
		}
		if s.CurrentPhaseStatus != state.StatusAwaitingHuman {
			httpError(w, http.StatusConflict, fmt.Sprintf("phase %q is not awaiting human (status=%q)", req.Phase, s.CurrentPhaseStatus))
			return
		}

		// If the user attached a message, write it to a checkpoint message file
		// so the orchestrator can pick it up as user_response on the next
		// pipeline_next_action call.
		if req.Message != "" {
			msgFile := filepath.Join(req.Workspace, "checkpoint-message.txt")
			if writeErr := os.WriteFile(msgFile, []byte(req.Message), 0o644); writeErr != nil { //nolint:gosec // G306: 0644 is intentional; the file must be readable by the orchestrator process
				httpError(w, http.StatusInternalServerError, fmt.Sprintf("write checkpoint message: %v", writeErr))
				return
			}
		}

		if err := sm.PhaseComplete(req.Workspace, req.Phase); err != nil {
			httpError(w, http.StatusInternalServerError, fmt.Sprintf("phase_complete: %v", err))
			return
		}
		// Publish phase-complete so any pipeline_next_action long-poll wakes up.
		bus.Publish(events.Event{
			Event:     "phase-complete",
			Phase:     req.Phase,
			Workspace: req.Workspace,
			Outcome:   "completed",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "approved",
			"workspace": req.Workspace,
			"phase":     req.Phase,
		})
	}
}

// abandonHandler marks the workspace as abandoned, mirroring the abandon MCP
// tool. It refuses to abandon a workspace that is already terminal.
//
// When publicMode is true the loopback/origin check is skipped (see approveCheckpointHandler).
func abandonHandler(sm *state.StateManager, publicMode bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !publicMode && !isLocalRequest(r) {
			httpError(w, http.StatusForbidden, "forbidden: intervention API requires loopback request and same-origin")
			return
		}
		req, ok := decodeRequest(w, r)
		if !ok {
			return
		}
		if req.Workspace == "" {
			httpError(w, http.StatusBadRequest, "workspace is required")
			return
		}

		if err := sm.LoadFromFile(req.Workspace); err != nil {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("load state: %v", err))
			return
		}
		s, err := sm.GetState()
		if err != nil {
			httpError(w, http.StatusInternalServerError, fmt.Sprintf("get state: %v", err))
			return
		}
		if s.CurrentPhaseStatus == state.StatusAbandoned {
			httpError(w, http.StatusConflict, "pipeline already abandoned")
			return
		}

		if err := sm.Abandon(req.Workspace); err != nil {
			httpError(w, http.StatusInternalServerError, fmt.Sprintf("abandon: %v", err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "abandoned",
			"workspace": req.Workspace,
		})
	}
}

// isLocalRequest enforces the safety contract documented at the top of the file.
//
// A request passes when:
//   - Its remote address is a loopback IP (covers curl from the host shell), AND
//   - It either has no Origin header OR carries one whose scheme and hostname
//     point at this listener (http + localhost / 127.0.0.1).
//
// Browsers always set Origin on cross-origin or fetch-API requests, so the
// Origin allowlist is sufficient CSRF protection without a token. The Origin
// is parsed structurally rather than prefix-matched so a crafted value such
// as "http://127.0.0.1:9876@evil.com" cannot satisfy the check.
func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin header — typical of curl/CLI calls from the host shell.
		// The loopback check has already established the caller is local.
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" {
		return false
	}
	hostname := u.Hostname()
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
}

// maxRequestBodyBytes caps the request body size for intervention endpoints.
// 64 KB is generous for the small JSON payloads these endpoints accept.
const maxRequestBodyBytes = 64 * 1024

// decodeRequest reads and parses the JSON body. On parse failure it writes a
// 400 response and returns ok=false so the caller short-circuits.
func decodeRequest(w http.ResponseWriter, r *http.Request) (interventionRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	var req interventionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON body: %v", err))
		return req, false
	}
	return req, true
}

// writeJSON writes status and body as a JSON response with the standard
// Content-Type. JSON encode failures are intentionally ignored — the response
// header has already been committed and there is no useful recovery.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) //nolint:errchkjson // body is always a map[string]any literal; encode errors on a committed response cannot be acted on
}

// httpError writes a JSON error response of the form {"error": "<msg>"}.
func httpError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
