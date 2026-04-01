package events

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSlackNotifier_EnabledFalseWhenURLEmpty(t *testing.T) {
	n := NewSlackNotifier("")
	if n.Enabled() {
		t.Fatal("Enabled() should return false when webhookURL is empty")
	}
}

func TestSlackNotifier_EnabledTrueWhenURLSet(t *testing.T) {
	n := NewSlackNotifier("https://hooks.slack.com/services/test")
	if !n.Enabled() {
		t.Fatal("Enabled() should return true when webhookURL is non-empty")
	}
}

func TestSlackNotifier_PostsCorrectJSONBody(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
	)
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	e := Event{
		Event:     "phase-complete",
		Phase:     "phase-3",
		SpecName:  "my-spec",
		Workspace: "/tmp/ws",
		Timestamp: "2026-03-26T10:00:00Z",
		Outcome:   "completed",
	}
	n.Notify(e)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: Slack webhook was not called within 3 seconds")
	}

	mu.Lock()
	body := gotBody
	mu.Unlock()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("webhook body is not valid JSON: %v — body: %s", err, body)
	}

	// Verify key event fields appear in the payload
	text, _ := payload["text"].(string)
	if text == "" {
		t.Fatalf("expected non-empty 'text' field in slack payload, got: %s", body)
	}
}

func TestSlackNotifier_NotifyReturnsImmediately(t *testing.T) {
	// Server that blocks for 1 second — Notify must return before the server finishes.
	slow := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-slow
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		close(slow)
		srv.Close()
	}()

	n := NewSlackNotifier(srv.URL)
	e := Event{Event: "phase-complete", Outcome: "completed"}

	start := time.Now()
	n.Notify(e)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Fatalf("Notify blocked for %v; expected immediate return", elapsed)
	}
}

func TestSlackNotifier_NetworkFailureSilentlySwallowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	done := make(chan struct{})
	// Close the server immediately to force a network error on the next request.
	srv.Close()

	n := NewSlackNotifier(srv.URL)
	e := Event{Event: "phase-complete", Outcome: "completed"}

	// Must not panic; any error should be silently swallowed.
	go func() {
		n.Notify(e)
		close(done)
	}()

	select {
	case <-done:
		// goroutine started; give the async POST goroutine time to finish
	case <-time.After(3 * time.Second):
		t.Fatal("Notify goroutine timed out")
	}
	// Allow the async POST goroutine to complete (max 3s)
	time.Sleep(500 * time.Millisecond)
}

func TestSlackNotifier_NoPostWhenDisabled(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier("")
	e := Event{Event: "phase-complete", Outcome: "completed"}
	n.Notify(e)
	// Give any potential goroutine time to run
	time.Sleep(100 * time.Millisecond)

	if called {
		t.Fatal("Notify should not POST when webhookURL is empty")
	}
}

func TestSlackNotifier_OnlyFiresForTargetEventTypes(t *testing.T) {
	var mu sync.Mutex
	var calledEvents []string
	allDone := make(chan struct{}, 10)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		mu.Lock()
		calledEvents = append(calledEvents, "called")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		allDone <- struct{}{}
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)

	targetEvents := []string{"phase-complete", "phase-fail", "abandon"}
	nonTargetEvents := []string{"phase-start", "checkpoint", "task-update", "revision-bump"}

	// Send non-target events — none should trigger a POST
	for _, evType := range nonTargetEvents {
		n.Notify(Event{Event: evType, Outcome: "in_progress"})
	}
	time.Sleep(200 * time.Millisecond) // give goroutines time to run if they were started

	mu.Lock()
	countBefore := len(calledEvents)
	mu.Unlock()
	if countBefore != 0 {
		t.Fatalf("expected 0 POST calls for non-target events, got %d", countBefore)
	}

	// Send target events — each should trigger exactly one POST
	for _, evType := range targetEvents {
		n.Notify(Event{Event: evType, Outcome: "completed"})
	}

	// Wait for all 3 expected calls
	timeout := time.After(3 * time.Second)
	for i := range targetEvents {
		select {
		case <-allDone:
		case <-timeout:
			t.Fatalf("timeout waiting for POST calls; received %d of %d", i, len(targetEvents))
		}
	}

	mu.Lock()
	total := len(calledEvents)
	mu.Unlock()
	if total != len(targetEvents) {
		t.Fatalf("expected %d POST calls for target events, got %d", len(targetEvents), total)
	}
}
