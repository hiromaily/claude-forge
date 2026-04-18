// Package main is the entry point for the forge-state MCP server.
// It wires together the StateManager, registers all 47 MCP tool handlers,
// and starts the stdio transport.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hiromaily/claude-forge/mcp-server/internal/dashboard"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/engine/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/handler/tools"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/analytics"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/intelligence/profile"
	"github.com/hiromaily/claude-forge/mcp-server/pkg/events"
)

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

// resolveEventsLog returns the path for the shared JSONL event log.
// It is a pure function: it reads env vars and the OS home directory but
// performs no filesystem operations (caller is responsible for MkdirAll).
//
// Priority:
//  1. FORGE_EVENTS_LOG environment variable (explicit override)
//  2. ~/.claude/forge-events.jsonl (default — shared across all project sessions)
//
// Using a home-directory path instead of a per-project specsDir path ensures
// that all MCP server instances (one per Claude Code session, regardless of
// project) write to the same file. The instance that owns the dashboard port
// can then tail this file to show events from every active session.
//
// Note: deployments that have per-project events.jsonl files from versions prior
// to this change will find those files orphaned under .specs/. They are no longer
// read or written; historical events from prior runs will not appear on the dashboard.
func resolveEventsLog() string {
	if p := os.Getenv("FORGE_EVENTS_LOG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "forge-events.jsonl"
	}
	return filepath.Join(home, ".claude", "forge-events.jsonl")
}

func main() {
	sm := state.NewStateManager(appVersion)
	specsDir := resolveSpecsDir()
	sm.SetSpecsDir(specsDir)
	srv := server.NewMCPServer("forge-state", appVersion)
	bus := events.NewEventBus()
	// Enable JSONL-based event persistence with a shared log path so all MCP server
	// instances (across different project sessions) write to the same file.
	// The instance that owns the dashboard port tails this file via WatchEventLog
	// to show events from every active session on a single dashboard.
	eventLogPath := resolveEventsLog()
	_ = os.MkdirAll(filepath.Dir(eventLogPath), 0o700)
	if err := bus.SetEventLog(eventLogPath); err != nil {
		fmt.Fprintf(os.Stderr, "forge-state: event log setup warning: %v\n", err)
	}
	eventsPort := os.Getenv("FORGE_EVENTS_PORT")
	// Start tailing the shared event log only on the instance that serves the dashboard.
	// Other instances write to the shared file but do not need to read it back.
	// This keeps pipeline and dashboard concerns separated: non-dashboard instances
	// perform no dashboard-related I/O beyond the log write in Publish.
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	if eventsPort != "" {
		bus.WatchEventLog(watchCtx)
	}
	slack := events.NewSlackNotifier(os.Getenv("FORGE_SLACK_WEBHOOK_URL"))
	agentDir := resolveAgentDir()
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

	// Start the optional dashboard / SSE / intervention HTTP server.
	// Returns nil when FORGE_EVENTS_PORT is unset or the bind fails; in either
	// case the MCP stdio transport below remains functional.
	httpSrv := dashboard.Start(eventsPort, bus, sm, &dashboard.StartOptions{
		PhaseLabels: orchestrator.PhaseLabels(),
	})

	// Run the MCP stdio transport. This blocks until stdin is closed.
	if err := server.ServeStdio(srv); err != nil {
		log.Fatal(err)
	}

	// Stop the WatchEventLog goroutine and perform graceful HTTP shutdown.
	cancelWatch()
	if httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "forge-state: SSE HTTP server shutdown error: %v\n", err)
		}
	}
	bus.CloseLog()
}
