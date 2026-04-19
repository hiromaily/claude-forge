package dashboard

import (
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
)

// publicModeEnabled returns true when FORGE_DASHBOARD_BIND_ALL is set to a non-empty value.
// In public mode the dashboard binds to 0.0.0.0 (all interfaces) and the
// loopback/origin safety checks in the intervention handlers are disabled.
// This is intentionally insecure and intended for local network development only.
func publicModeEnabled() bool {
	return os.Getenv("FORGE_DASHBOARD_BIND_ALL") != ""
}

const (
	// fallbackPortMin and fallbackPortMax define the inclusive range of ports
	// tried when the configured eventsPort is already in use.
	fallbackPortMin = 8100
	fallbackPortMax = 8200
	// fallbackAttempts is the number of random ports to try before giving up.
	fallbackAttempts = 10
)

// readHeaderTimeout caps the time a client may take to send the request line
// and headers. Five seconds is generous for localhost traffic and shuts the
// door on slowloris-style header drips.
const readHeaderTimeout = 5 * time.Second

// Start binds an HTTP listener on "127.0.0.1:<eventsPort>" and serves:
//
//	GET  /events                  — SSE stream of pipeline phase transitions
//	GET  /                        — embedded zero-dependency dashboard HTML
//	POST /api/checkpoint/approve  — dashboard-driven checkpoint approval
//	POST /api/pipeline/abandon    — dashboard-driven pipeline abandon
//
// On successful bind it logs the dashboard URL to stderr so users can click
// straight through to the live timeline. The log goes to stderr — never
// stdout — so it never interferes with the MCP stdio transport.
//
// Returns the started *http.Server on success, or nil when:
//   - eventsPort is empty (caller chose not to enable the dashboard), or
//   - the port cannot be bound (the error is logged to stderr and execution
//     continues so the MCP stdio transport remains functional).
//
// The caller is responsible for calling Shutdown on the returned server
// during graceful shutdown.

// StartOptions holds optional configuration for the dashboard server.
type StartOptions struct {
	// PhaseLabels maps phase IDs (e.g. "phase-3") to human-readable labels
	// (e.g. "Design"). The dashboard serves this map via GET /api/phase-labels
	// so the frontend can resolve labels client-side without coupling the event
	// publishing path to the orchestrator package.
	PhaseLabels map[string]string
}

func Start(eventsPort string, bus *events.EventBus, sm *state.StateManager, opts *StartOptions) *http.Server {
	if eventsPort == "" {
		return nil
	}

	public := publicModeEnabled()
	ln := listenWithFallback(eventsPort, public)
	if ln == nil {
		return nil
	}

	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		fmt.Fprintf(os.Stderr, "forge-state: dashboard ready at %s\n", dashboardURL(tcpAddr.Port))
	} else {
		fmt.Fprintf(os.Stderr, "forge-state: dashboard ready (listener addr type %T; URL unavailable)\n", ln.Addr())
	}
	if public {
		fmt.Fprintf(os.Stderr, "forge-state: dashboard running in public mode (FORGE_DASHBOARD_BIND_ALL=1) — accessible from any network interface\n")
	}

	var labels map[string]string
	if opts != nil && opts.PhaseLabels != nil {
		labels = opts.PhaseLabels
	}

	mux := http.NewServeMux()
	mux.Handle("GET /events", events.SSEHandler(bus))
	mux.Handle("GET /", dashboardHandler())
	mux.Handle("GET /api/phase-labels", phaseLabelsHandler(labels))
	mux.Handle("GET /api/phase-artifacts", phaseArtifactsHandler())
	mux.Handle("GET /api/artifact", artifactHandler())
	mux.Handle("POST /api/checkpoint/approve", approveCheckpointHandler(sm, bus, public))
	mux.Handle("POST /api/pipeline/abandon", abandonHandler(sm, bus, public))

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "forge-state: SSE HTTP server error: %v\n", serveErr)
		}
	}()
	return srv
}

// listenWithFallback tries to bind on the configured port first. If that fails
// (e.g. port conflict), it retries on random ports in [fallbackPortMin, fallbackPortMax].
// When public is true it binds to 0.0.0.0 (all interfaces); otherwise 127.0.0.1 only.
// Returns nil only when all attempts are exhausted.
func listenWithFallback(preferredPort string, public bool) net.Listener {
	host := "127.0.0.1"
	if public {
		host = "0.0.0.0"
	}
	addr := net.JoinHostPort(host, preferredPort)
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln
	}
	fmt.Fprintf(os.Stderr, "forge-state: port %s in use, trying fallback range %d–%d\n",
		preferredPort, fallbackPortMin, fallbackPortMax)

	for range fallbackAttempts {
		port := fallbackPortMin + rand.IntN(fallbackPortMax-fallbackPortMin+1) //nolint:gosec // port selection is non-security use of math/rand
		addr = net.JoinHostPort(host, strconv.Itoa(port))
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			return ln
		}
	}
	fmt.Fprintf(os.Stderr, "forge-state: SSE HTTP server could not bind after %d fallback attempts (continuing without SSE)\n", fallbackAttempts)
	return nil
}
