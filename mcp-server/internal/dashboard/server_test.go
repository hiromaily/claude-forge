package dashboard

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
	"github.com/hiromaily/claude-forge/mcp-server/internal/state"
)

// freePort returns a random available TCP port number as a string.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return strconv.Itoa(port)
}

// TestStart_BindsAndServesEvents verifies that Start returns a non-nil server
// that listens on the chosen port and serves SSE events from the bus.
func TestStart_BindsAndServesEvents(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	bus := events.NewEventBus()

	srv := Start(port, bus, state.NewStateManager("test"))
	if srv == nil {
		t.Fatal("Start returned nil; expected a running server")
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
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %q", ct)
	}

	bus.Publish(events.Event{
		Event:     "phase-complete",
		Phase:     "phase-1",
		SpecName:  "test-spec",
		Workspace: "/tmp/ws",
		Timestamp: "2024-01-01T00:00:00Z",
		Outcome:   "completed",
	})

	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		done <- string(buf[:n])
	}()
	select {
	case data := <-done:
		if !strings.Contains(data, "phase-complete") {
			t.Fatalf("expected SSE data to contain phase-complete, got: %q", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}

// TestStart_FallbackOnPortConflict verifies that when the preferred port is
// already in use, Start falls back to a random port in the fallback range
// and returns a running server.
func TestStart_FallbackOnPortConflict(t *testing.T) {
	t.Parallel()

	// Occupy the preferred port so Start must fall back.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	occupiedPort := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	bus := events.NewEventBus()
	srv := Start(occupiedPort, bus, state.NewStateManager("test"))
	if srv == nil {
		t.Fatal("expected Start to succeed via fallback port, got nil")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
}

// TestListenWithFallback_PreferredPortSucceeds verifies that when the
// preferred port is free, listenWithFallback returns a listener on that port.
func TestListenWithFallback_PreferredPortSucceeds(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	ln := listenWithFallback(port)
	if ln == nil {
		t.Fatal("listenWithFallback returned nil for free port")
	}
	defer func() { _ = ln.Close() }()

	gotPort := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	if gotPort != port {
		t.Errorf("listenWithFallback bound port %s, want %s", gotPort, port)
	}
}

// TestListenWithFallback_FallbackPortRange verifies that when the preferred
// port is occupied, the fallback binds within [fallbackPortMin, fallbackPortMax].
func TestListenWithFallback_FallbackPortRange(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	occupiedPort := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	fallback := listenWithFallback(occupiedPort)
	if fallback == nil {
		t.Fatal("listenWithFallback returned nil; expected fallback to succeed")
	}
	defer func() { _ = fallback.Close() }()

	gotPort := fallback.Addr().(*net.TCPAddr).Port
	if gotPort < fallbackPortMin || gotPort > fallbackPortMax {
		t.Errorf("fallback port %d outside expected range [%d, %d]", gotPort, fallbackPortMin, fallbackPortMax)
	}
}

// TestStart_NoopWhenPortEmpty verifies that an empty eventsPort short-circuits
// inside Start. main.go relies on this contract so it can call Start
// unconditionally.
func TestStart_NoopWhenPortEmpty(t *testing.T) {
	t.Parallel()

	srv := Start("", events.NewEventBus(), state.NewStateManager("test"))
	if srv != nil {
		t.Fatal("expected Start(\"\", ...) to return nil")
	}
}

// TestStart_ShutdownPath verifies that the shutdown path stops accepting
// new connections.
func TestStart_ShutdownPath(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	bus := events.NewEventBus()

	srv := Start(port, bus, state.NewStateManager("test"))
	if srv == nil {
		t.Fatal("Start returned nil")
	}
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	if _, err := http.Get(fmt.Sprintf("http://localhost:%s/events", port)); err == nil {
		t.Fatal("expected connection refused after shutdown, got success")
	}
}

// TestStart_LogsDashboardURLOnStartup verifies that Start prints a
// click-through dashboard URL to stderr after a successful bind.
//
// This test mutates os.Stderr (an OS-level global), so it cannot run in
// parallel with anything else that reads or writes stderr. Per the
// go-test.md exception clause, t.Parallel() is intentionally omitted.
func TestStart_LogsDashboardURLOnStartup(t *testing.T) {
	port := freePort(t)
	bus := events.NewEventBus()

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	srv := Start(port, bus, state.NewStateManager("test"))
	if srv == nil {
		t.Fatal("Start returned nil")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	_ = w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stderr pipe: %v", err)
	}
	logged := buf.String()

	wantPort := "http://localhost:" + port + "/"
	if !strings.Contains(logged, wantPort) {
		t.Errorf("stderr does not contain %q (got: %q)", wantPort, logged)
	}
	if !strings.Contains(logged, "dashboard ready at") {
		t.Errorf("stderr does not contain user-facing label %q (got: %q)", "dashboard ready at", logged)
	}
}

// TestSSEResponseWriterFlushes verifies the SSE response includes proper
// newline formatting for the data lines.
func TestSSEResponseWriterFlushes(t *testing.T) {
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

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://localhost:%s/events", port), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bus.Publish(events.Event{
		Event:   "phase-start",
		Phase:   "phase-2",
		Outcome: "in_progress",
	})

	buf := make([]byte, 1024)
	done := make(chan int, 1)
	go func() {
		n, _ := resp.Body.Read(buf)
		done <- n
	}()
	select {
	case n := <-done:
		data := string(buf[:n])
		if !strings.HasPrefix(data, "data: ") {
			t.Fatalf("expected SSE data to start with 'data: ', got: %q", data)
		}
		if !strings.Contains(data, "\n\n") {
			t.Fatalf("expected SSE data to contain double newline, got: %q", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}
