package dashboard

import (
	_ "embed"
	"encoding/json"
	"net/http"
)

// dashboardHTML is the static HTML/CSS/JS dashboard served at GET /.
// It is a zero-dependency client that subscribes to GET /events via
// EventSource and renders a real-time pipeline timeline.
//
//go:embed dashboard.html
var dashboardHTML []byte

// dashboardHandler serves the embedded dashboard HTML at GET /.
// It returns 404 for any other path so the SSE endpoint and intervention
// API routes are not shadowed by the catch-all "GET /" mux pattern.
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

// phaseLabelsHandler serves the phase ID → label map as JSON.
// The dashboard fetches this once on load and resolves labels client-side,
// keeping the event publishing path free from orchestrator dependencies.
func phaseLabelsHandler(labels map[string]string) http.HandlerFunc {
	// Pre-encode the response; the map is immutable after startup.
	if labels == nil {
		labels = make(map[string]string)
	}
	data, _ := json.Marshal(labels)

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write(data)
	}
}
