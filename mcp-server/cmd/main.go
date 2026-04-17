// Package main is the entry point for the forge-state MCP server.
// It wires together the StateManager, registers all 46 MCP tool handlers,
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

	"github.com/hiromaily/claude-forge/mcp-server/internal/analytics"
	"github.com/hiromaily/claude-forge/mcp-server/internal/dashboard"
	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/history"
	"github.com/hiromaily/claude-forge/mcp-server/internal/orchestrator"
	"github.com/hiromaily/claude-forge/mcp-server/internal/profile"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
	"github.com/hiromaily/claude-forge/mcp-server/internal/tools"
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

func main() {
	sm := state.NewStateManager(appVersion)
	specsDir := resolveSpecsDir()
	sm.SetSpecsDir(specsDir)
	srv := server.NewMCPServer("forge-state", appVersion)
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier(os.Getenv("FORGE_SLACK_WEBHOOK_URL"))
	eventsPort := os.Getenv("FORGE_EVENTS_PORT")
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
	httpSrv := dashboard.Start(eventsPort, bus, sm)

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
