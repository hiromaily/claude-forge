// Package main is the entry point for the forge-state MCP server.
// It wires together the StateManager, registers all 38 MCP tool handlers,
// and starts the stdio transport.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/events"
	"github.com/hiromaily/claude-forge/mcp-server/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/state"
	"github.com/hiromaily/claude-forge/mcp-server/tools"
)

// resolveAgentDir resolves the agents/ directory path using a 3-stage strategy:
//  1. FORGE_AGENTS_PATH environment variable (required in production)
//  2. Path derived from runtime.Caller(0) — only used as a dev fallback; skipped
//     if the derived path does not exist on disk
//  3. The literal string "agents" as a last-resort relative fallback
//
// Production deployments must set FORGE_AGENTS_PATH to the absolute path of the
// agents/ directory. The runtime.Caller(0) fallback embeds the compile-time source
// path into the binary, which is unreliable in packaged or cross-compiled builds.
func resolveAgentDir() string {
	if dir := os.Getenv("FORGE_AGENTS_PATH"); dir != "" {
		return dir
	}
	// Dev fallback: derive from the source file location at compile time.
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		derived := filepath.Join(filepath.Dir(filename), "..", "agents")
		if _, err := os.Stat(derived); err == nil {
			return derived
		}
	}
	return "agents"
}

// startSSEServer attempts to bind an HTTP server for the SSE /events endpoint on
// the given address. It returns the started *http.Server on success, or nil when
// the port cannot be bound (the error is logged to stderr and execution continues).
// A nil return means SSE is disabled but the MCP stdio transport is unaffected.
func startSSEServer(addr string, bus *events.EventBus) *http.Server {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "forge-state: SSE HTTP server could not bind %s: %v (continuing without SSE)\n", addr, err)
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("GET /events", events.SSEHandler(bus))
	srv := &http.Server{Handler: mux}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "forge-state: SSE HTTP server error: %v\n", serveErr)
		}
	}()
	return srv
}

func main() {
	sm := state.NewStateManager()
	srv := server.NewMCPServer("forge-state", "1.0.0")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier(os.Getenv("FORGE_SLACK_WEBHOOK_URL"))
	eventsPort := os.Getenv("FORGE_EVENTS_PORT")
	agentDir := resolveAgentDir()
	specsDir := os.Getenv("FORGE_SPECS_DIR")
	eng := orchestrator.NewEngine(agentDir, specsDir)
	tools.RegisterAll(srv, sm, bus, slack, eventsPort, eng, agentDir)

	// Start the SSE HTTP server if FORGE_EVENTS_PORT is set.
	// A failed bind is non-fatal: the error is logged and execution continues
	// to ServeStdio so the MCP stdio transport remains functional.
	var httpSrv *http.Server
	if eventsPort != "" {
		httpSrv = startSSEServer(":"+eventsPort, bus)
	}

	// Run the MCP stdio transport. This blocks until stdin is closed.
	if err := server.ServeStdio(srv); err != nil {
		log.Fatal(err)
	}

	// Graceful shutdown of the HTTP server after ServeStdio returns.
	if httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "forge-state: SSE HTTP server shutdown error: %v\n", err)
		}
	}
}
