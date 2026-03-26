package events

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// SSEHandler returns an http.HandlerFunc that streams pipeline events as Server-Sent Events.
//
// Each published Event is encoded as JSON and written in the SSE data-only format:
//
//	data: <json>\n\n
//
// The handler subscribes to bus on entry and calls bus.Unsubscribe when the client disconnects
// (detected via request context cancellation) or when the server is shutting down.
//
// Optional server-side filtering: if the request includes a "workspace" query parameter, only
// events whose Workspace field equals that value are written to the stream; all other events are
// silently skipped.
func SSEHandler(bus *EventBus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers before writing any body.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Obtain the Flusher interface — required for real-time streaming.
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// Write the 200 OK status and flush headers immediately so the client
		// receives the response headers before the first event arrives.
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		// Optional workspace filter.
		wsFilter := r.URL.Query().Get("workspace")

		// Subscribe to the bus.
		id, ch := bus.Subscribe()
		defer bus.Unsubscribe(id)

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				// Client disconnected or server is shutting down.
				return
			case e, ok := <-ch:
				if !ok {
					// Channel was closed (Unsubscribe called externally).
					return
				}
				// Apply workspace filter if set.
				if wsFilter != "" && e.Workspace != wsFilter {
					continue
				}
				payload, err := json.Marshal(e)
				if err != nil {
					// Skip malformed events rather than crashing.
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
			}
		}
	}
}
