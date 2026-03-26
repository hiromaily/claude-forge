// Package main is the entry point for the forge-state MCP server.
// Implementation is completed by Task 8.
package main

import (
	"log"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	srv := server.NewMCPServer("forge-state", "1.0.0")
	if err := server.ServeStdio(srv); err != nil {
		log.Fatal(err)
	}
}
