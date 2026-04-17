package dashboard

import (
	_ "embed"
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
