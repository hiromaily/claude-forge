// Package main is the entry point for the forge-state MCP server.
// It wires together the StateManager, registers all 27 MCP tool handlers,
// and starts the stdio transport.
package main

import (
	"log"

	"github.com/hiromaily/claude-forge/mcp-server/state"
	"github.com/hiromaily/claude-forge/mcp-server/tools"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	sm := state.NewStateManager()
	srv := server.NewMCPServer("forge-state", "1.0.0")
	tools.RegisterAll(srv, sm)
	if err := server.ServeStdio(srv); err != nil {
		log.Fatal(err)
	}
}
