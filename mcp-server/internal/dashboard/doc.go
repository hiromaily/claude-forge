// Package dashboard exposes the optional HTTP listener that serves the
// embedded zero-dependency web dashboard, the SSE events stream, and a
// loopback-only intervention API for approving checkpoints or abandoning a
// pipeline from the browser.
//
// The package is opt-in. main.go calls [Start] only when the user sets
// FORGE_EVENTS_PORT; otherwise no listener is created and the MCP stdio
// transport behaves exactly as before.
//
// Routes mounted on the listener:
//
//	GET  /events                  — Server-Sent Events stream of phase transitions
//	GET  /                        — embedded dashboard HTML (single file, no CDN)
//	POST /api/checkpoint/approve  — advance the current checkpoint phase
//	POST /api/pipeline/abandon    — mark the pipeline as abandoned
//
// The intervention endpoints enforce a loopback + same-origin safety contract
// (see [isLocalRequest]) so a malicious page in another tab cannot drive a
// pipeline. They go through the StateManager so the same guards that protect
// MCP tool calls also protect dashboard-originated calls.
//
// Import direction: dashboard → state, dashboard → events. The package
// imports nothing from cmd/ or tools/, keeping the cmd entry point thin.
package dashboard
