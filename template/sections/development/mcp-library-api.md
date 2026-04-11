## MCP Library Usage

Key API surface used in `mcp-server/` (`github.com/mark3labs/mcp-go`):

```go
// Server construction and stdio transport
srv := server.NewMCPServer("forge-state", "1.0.0")
server.ServeStdio(srv)   // package-level function, not a method on srv

// Registering a tool
srv.AddTool(mcp.NewTool("tool_name",
    mcp.WithDescription("..."),
    mcp.WithString("param", mcp.Required(), mcp.Description("...")),
    mcp.WithNumber("num_param", mcp.Description("...")),
), HandlerFunc)

// Reading parameters inside a handler
workspace, err := req.RequireString("workspace")   // returns error if missing
value := req.GetString("key", "default")           // returns default if missing
num := req.GetInt("tokens", 0)
flag := req.GetBool("validated", false)
args := req.GetArguments()                         // map[string]any for complex params

// Returning results
mcp.NewToolResultText("ok")                        // success with text
mcp.NewToolResultError("error message")            // IsError=true response
```

Tool names use underscores (`phase_complete`), not hyphens — MCP protocol requirement. The corresponding MCP tool call name is `mcp__forge-state__phase_complete`.

---
