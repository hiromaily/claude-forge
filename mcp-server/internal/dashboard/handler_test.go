package dashboard

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// TestDashboardHandler_ServesEmbeddedHTMLAtRoot verifies that GET / serves the
// embedded dashboard HTML with the expected content type and that the safety
// mechanisms (DOM cap, allowlist) are present in the served body so a future
// edit cannot silently remove them.
func TestDashboardHandler_ServesEmbeddedHTMLAtRoot(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	bus := events.NewEventBus()
	srv := Start(port, bus, state.NewStateManager("test"))
	if srv == nil {
		t.Fatal("Start returned nil")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	time.Sleep(20 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%s/", port)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html prefix", ct)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	body := string(bodyBytes)
	wantSubstrings := []string{
		"<!DOCTYPE html>",
		"claude-forge",
		"EventSource",
		"/events",
		// Safety mechanisms — fail the test if a future edit removes the
		// DOM cap or the CSS-class allowlist.
		"MAX_TIMELINE_ROWS",
		"safeClassFragment",
		"KNOWN_EVENTS",
		"KNOWN_OUTCOMES",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q (body length=%d)", want, len(body))
		}
	}
}

// TestDashboardHandler_404ForUnknownPaths verifies that paths other than /
// return 404 so /events and the intervention API routes are not shadowed by
// the catch-all "GET /" mux pattern.
func TestDashboardHandler_404ForUnknownPaths(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	bus := events.NewEventBus()
	srv := Start(port, bus, state.NewStateManager("test"))
	if srv == nil {
		t.Fatal("Start returned nil")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	time.Sleep(20 * time.Millisecond)

	cases := []struct {
		name string
		path string
	}{
		{name: "unknown_subpath", path: "/dashboard"},
		{name: "nested_unknown", path: "/static/main.js"},
		{name: "deep_unknown", path: "/a/b/c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			url := fmt.Sprintf("http://localhost:%s%s", port, tc.path)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("http.NewRequest: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("GET %s status: got %d, want 404", tc.path, resp.StatusCode)
			}
		})
	}
}

// TestDashboardHandler_DoesNotShadowEvents verifies that adding the / route
// does not break the existing GET /events SSE endpoint.
func TestDashboardHandler_DoesNotShadowEvents(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	bus := events.NewEventBus()
	srv := Start(port, bus, state.NewStateManager("test"))
	if srv == nil {
		t.Fatal("Start returned nil")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	time.Sleep(20 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%s/events", port)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/events status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("/events Content-Type: got %q, want text/event-stream", ct)
	}
}
