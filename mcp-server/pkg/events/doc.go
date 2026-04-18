// Package events provides an in-process event bus for pipeline phase
// transition notifications.
//
// The [Bus] broadcasts phase-start, phase-complete, and phase-fail events
// to registered subscribers. It is used by the SSE handler to stream
// real-time pipeline progress to external dashboards.
//
// The [SSEHandler] exposes an HTTP endpoint for Server-Sent Events when
// the FORGE_EVENTS_PORT environment variable is set.
//
// Import direction: events → state (reads phase/status constants).
package events
