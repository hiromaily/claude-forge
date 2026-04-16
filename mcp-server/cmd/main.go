// Package main is the entry point for the forge-state MCP server.
// It wires together the StateManager, registers all 44 MCP tool handlers,
// and starts the stdio transport.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/analytics"
	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/profile"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/tools"
)

// dashboardHTML is the static HTML/CSS/JS dashboard served at GET /.
// It is a zero-dependency client that subscribes to GET /events via
// EventSource and renders a real-time pipeline timeline.
//
//go:embed dashboard.html
var dashboardHTML []byte

var appVersion = "dev"

// resolveSpecsDir resolves the .specs/ directory path using a 3-stage strategy:
//  1. FORGE_SPECS_DIR environment variable (required in production)
//  2. Path derived from runtime.Caller(0) — only used as a dev fallback; skipped
//     if the derived path does not exist on disk
//  3. The literal string ".specs" as a last-resort relative fallback
//
// Production deployments must set FORGE_SPECS_DIR to the absolute path of the
// .specs/ directory at the repo root.
func resolveSpecsDir() string {
	if dir := os.Getenv("FORGE_SPECS_DIR"); dir != "" {
		return dir
	}
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		derived := filepath.Join(filepath.Dir(filename), "../..", ".specs")
		if _, err := os.Stat(derived); err == nil {
			return derived
		}
	}
	return ".specs"
}

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
		derived := filepath.Join(filepath.Dir(filename), "../..", "agents")
		if _, err := os.Stat(derived); err == nil {
			return derived
		}
	}
	return "agents"
}

// dashboardURL formats the user-facing URL printed at startup.
//
// Always uses "localhost" rather than the bind interface so the line is
// click-through in any modern terminal regardless of which host the
// listener bound to (commonly "[::]:<port>" on dual-stack systems).
//
// port is expected to be a TCP port the kernel actually accepted — in
// practice the value returned by net.Listener.Addr().(*net.TCPAddr).Port.
// No range validation is performed; callers control the input.
func dashboardURL(port int) string {
	return fmt.Sprintf("http://localhost:%d/", port)
}

// dashboardHandler serves the embedded dashboard HTML at GET /.
// It returns 404 for any other path so the SSE endpoint and future routes
// are not shadowed.
func dashboardHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(dashboardHTML)
	}
}

// startSSEServer attempts to bind an HTTP server on the given address.
// It exposes:
//   - GET /events  — Server-Sent Events stream of pipeline phase transitions
//   - GET /        — embedded zero-dependency dashboard HTML
//
// On successful bind it logs the dashboard URL to stderr so users can
// click straight through to the live timeline. The log goes to stderr —
// never stdout — so it never interferes with the MCP stdio transport.
//
// It returns the started *http.Server on success, or nil when the port cannot
// be bound (the error is logged to stderr and execution continues).
// A nil return means the dashboard and SSE are disabled but the MCP stdio
// transport is unaffected.
func startSSEServer(addr string, bus *events.EventBus) *http.Server {
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
	srv := &http.Server{Handler: mux}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "forge-state: SSE HTTP server error: %v\n", serveErr)
		}
	}()
	return srv
}

func main() {
	sm := state.NewStateManager(appVersion)
	srv := server.NewMCPServer("forge-state", appVersion)
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier(os.Getenv("FORGE_SLACK_WEBHOOK_URL"))
	eventsPort := os.Getenv("FORGE_EVENTS_PORT")
	agentDir := resolveAgentDir()
	specsDir := resolveSpecsDir()
	eng := orchestrator.NewEngine(agentDir, specsDir)
	histIdx := history.New(specsDir)
	if err := histIdx.Build(); err != nil {
		fmt.Fprintf(os.Stderr, "forge-state: history index build warning: %v\n", err)
	}
	kb := history.NewKnowledgeBase(specsDir)
	if err := kb.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "forge-state: knowledge base load warning: %v\n", err)
	}
	profiler := profile.New(filepath.Join(specsDir, "repo-profile.json"), filepath.Dir(specsDir))
	if _, err := profiler.AnalyzeOrUpdate(); err != nil {
		fmt.Fprintf(os.Stderr, "forge-state: repo profiler warning: %v\n", err)
	}
	col := analytics.NewCollector(specsDir)
	est := analytics.NewEstimator(specsDir)
	rep := analytics.NewReporter(specsDir, kb)
	tools.RegisterAll(srv, sm, bus, slack, eventsPort, eng, agentDir, histIdx, kb, profiler, col, est, rep)

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
