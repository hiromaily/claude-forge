// Package main is the entry point for the forge-state MCP server.
// It wires together the StateManager, registers all 28 MCP tool handlers,
// and starts the stdio transport.
package main

import (
	"log"
	"os"

	"github.com/hiromaily/claude-forge/mcp-server/events"
	"github.com/hiromaily/claude-forge/mcp-server/state"
	"github.com/hiromaily/claude-forge/mcp-server/tools"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	sm := state.NewStateManager()
	srv := server.NewMCPServer("forge-state", "1.0.0")
	bus := events.NewEventBus()
	slack := events.NewSlackNotifier(os.Getenv("FORGE_SLACK_WEBHOOK_URL"))
	eventsPort := os.Getenv("FORGE_EVENTS_PORT")
	tools.RegisterAll(srv, sm, bus, slack, eventsPort)
	if err := server.ServeStdio(srv); err != nil {
		log.Fatal(err)
	}
}
