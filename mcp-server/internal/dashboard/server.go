package dashboard

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// readHeaderTimeout caps the time a client may take to send the request line
// and headers. Five seconds is generous for localhost traffic and shuts the
// door on slowloris-style header drips.
const readHeaderTimeout = 5 * time.Second

// Start binds an HTTP listener on ":<eventsPort>" and serves:
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
func Start(eventsPort string, bus *events.EventBus, sm *state.StateManager) *http.Server {
	if eventsPort == "" {
		return nil
	}
	addr := ":" + eventsPort

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "forge-state: SSE HTTP server could not bind %s: %v (continuing without SSE)\n", addr, err)
		return nil
	}
	// Resolve the actual bound port — handles the FORGE_EVENTS_PORT=0 case
	// where the kernel assigns a random free port and addr would otherwise
	// be misleading. The type assertion holds for any successful tcp Listen,
	// but we still guard it so a future protocol change cannot crash startup.
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		fmt.Fprintf(os.Stderr, "forge-state: dashboard ready at %s\n", dashboardURL(tcpAddr.Port))
	} else {
		fmt.Fprintf(os.Stderr, "forge-state: dashboard ready (listener addr type %T; URL unavailable)\n", ln.Addr())
	}

	mux := http.NewServeMux()
	mux.Handle("GET /events", events.SSEHandler(bus))
	mux.Handle("GET /", dashboardHandler())
	mux.Handle("POST /api/checkpoint/approve", approveCheckpointHandler(sm))
	mux.Handle("POST /api/pipeline/abandon", abandonHandler(sm))

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
