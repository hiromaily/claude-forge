package events_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
)

// TestSSEHandlerHeaders verifies that the handler sets the required SSE headers.
func TestSSEHandlerHeaders(t *testing.T) {
	bus := events.NewEventBus()
	srv := httptest.NewServer(events.SSEHandler(bus))
	defer srv.Close()

	ctx := t.Context()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events failed: %v", err)
	}
	defer resp.Body.Close()

	checkHeader := func(key, want string) {
		t.Helper()
		if got := resp.Header.Get(key); got != want {
			t.Errorf("header %q = %q, want %q", key, got, want)
		}
	}
	checkHeader("Content-Type", "text/event-stream")
	checkHeader("Cache-Control", "no-cache")
	checkHeader("Connection", "keep-alive")
}

// TestSSEHandlerReceivesEvent verifies that a published event is written as a "data: <json>\n\n"
// SSE line that is flushed to the HTTP response.
func TestSSEHandlerReceivesEvent(t *testing.T) {
	bus := events.NewEventBus()
	srv := httptest.NewServer(events.SSEHandler(bus))
	defer srv.Close()

	ctx := t.Context()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events failed: %v", err)
	}
	defer resp.Body.Close()

	// Give the handler goroutine a moment to subscribe before publishing.
	time.Sleep(20 * time.Millisecond)

	want := events.Event{
		Event:     "phase-complete",
		Phase:     "phase-3",
		SpecName:  "my-spec",
		Workspace: "/workspace/abc",
		Timestamp: "2026-03-26T00:00:00Z",
		Outcome:   "completed",
	}
	bus.Publish(want)

	// Read lines from the SSE stream.
	lines := make(chan string, 10)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	// Expect a "data: ..." line.
	var dataLine string
	select {
	case line := <-lines:
		dataLine = line
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE data line")
	}

	if !strings.HasPrefix(dataLine, "data: ") {
		t.Fatalf("expected line to start with 'data: ', got %q", dataLine)
	}

	jsonPart := strings.TrimPrefix(dataLine, "data: ")
	var got events.Event
	if err := json.Unmarshal([]byte(jsonPart), &got); err != nil {
		t.Fatalf("failed to unmarshal JSON payload: %v — raw: %q", err, jsonPart)
	}

	if got != want {
		t.Errorf("received event = %+v, want %+v", got, want)
	}
}

// TestSSEHandlerClientDisconnect verifies that cancelling the request context causes the handler
// to call bus.Unsubscribe and exit cleanly (no goroutine leak).
func TestSSEHandlerClientDisconnect(t *testing.T) {
	bus := events.NewEventBus()

	handlerDone := make(chan struct{})
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		events.SSEHandler(bus)(w, r)
		close(handlerDone)
	})

	srv := httptest.NewServer(wrapped)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events failed: %v", err)
	}
	defer resp.Body.Close()

	// Give the handler time to subscribe.
	time.Sleep(20 * time.Millisecond)

	// Cancel the client context to simulate disconnect.
	cancel()

	// The handler should exit and close handlerDone within a short time.
	select {
	case <-handlerDone:
		// success — handler returned
	case <-time.After(2 * time.Second):
		t.Fatal("SSEHandler did not exit after client disconnect")
	}

	// After the handler exits, the bus should have no subscribers (Unsubscribe was called).
	// We verify this indirectly: publish an event and ensure no channel-related panics occur.
	bus.Publish(events.Event{Event: "phase-start", Outcome: "in_progress"})
}

// TestSSEHandlerWorkspaceFilter verifies that when a ?workspace= query parameter is provided,
// only events whose Workspace field matches are forwarded; others are silently skipped.
func TestSSEHandlerWorkspaceFilter(t *testing.T) {
	bus := events.NewEventBus()
	srv := httptest.NewServer(events.SSEHandler(bus))
	defer srv.Close()

	ctx := t.Context()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events?workspace=/workspace/target", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events?workspace=... failed: %v", err)
	}
	defer resp.Body.Close()

	// Give the handler time to subscribe.
	time.Sleep(20 * time.Millisecond)

	// Publish an event that should be skipped (wrong workspace).
	bus.Publish(events.Event{
		Event:     "phase-start",
		Workspace: "/workspace/other",
		Outcome:   "in_progress",
	})

	// Publish an event that should be forwarded (matching workspace).
	wantEvent := events.Event{
		Event:     "phase-complete",
		Phase:     "phase-3",
		SpecName:  "target-spec",
		Workspace: "/workspace/target",
		Timestamp: "2026-03-26T00:00:00Z",
		Outcome:   "completed",
	}
	bus.Publish(wantEvent)

	lines := make(chan string, 10)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	var dataLine string
	select {
	case line := <-lines:
		dataLine = line
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE data line")
	}

	if !strings.HasPrefix(dataLine, "data: ") {
		t.Fatalf("expected line to start with 'data: ', got %q", dataLine)
	}

	jsonPart := strings.TrimPrefix(dataLine, "data: ")
	var got events.Event
	if err := json.Unmarshal([]byte(jsonPart), &got); err != nil {
		t.Fatalf("failed to unmarshal JSON payload: %v — raw: %q", err, jsonPart)
	}

	if got.Workspace != "/workspace/target" {
		t.Errorf("received event from workspace %q, want %q", got.Workspace, "/workspace/target")
	}
	if got != wantEvent {
		t.Errorf("received event = %+v, want %+v", got, wantEvent)
	}
}
