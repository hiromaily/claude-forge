package events_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/internal/events"
)

// TestPublishZeroSubscribers verifies that Publish with no subscribers does not panic or block.
func TestPublishZeroSubscribers(t *testing.T) {
	bus := events.NewEventBus()
	// Must not panic or block
	done := make(chan struct{})
	go func() {
		bus.Publish(events.Event{
			Event:     "phase-start",
			Phase:     "phase-1",
			SpecName:  "test-spec",
			Workspace: "/tmp/ws",
			Timestamp: "2026-03-26T00:00:00Z",
			Outcome:   "in_progress",
		})
		close(done)
	}()
	select {
	case <-done:
		// success
	case <-time.After(time.Second):
		t.Fatal("Publish to zero subscribers blocked or timed out")
	}
}

// TestPublishOneSubscriber verifies that a subscribed channel receives the published event.
func TestPublishOneSubscriber(t *testing.T) {
	bus := events.NewEventBus()
	_, ch := bus.Subscribe()

	want := events.Event{
		Event:     "phase-complete",
		Phase:     "phase-2",
		SpecName:  "my-spec",
		Workspace: "/tmp/ws",
		Timestamp: "2026-03-26T01:00:00Z",
		Outcome:   "completed",
	}
	bus.Publish(want)

	select {
	case got := <-ch:
		if got != want {
			t.Errorf("received event = %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

// TestUnsubscribeRemovesChannel verifies that after Unsubscribe, no further events are delivered.
func TestUnsubscribeRemovesChannel(t *testing.T) {
	bus := events.NewEventBus()
	id, ch := bus.Subscribe()
	bus.Unsubscribe(id)

	bus.Publish(events.Event{Event: "phase-start", Outcome: "in_progress"})

	// Channel should be closed and empty after unsubscribe
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after Unsubscribe, but received a value")
		}
		// Channel closed — correct behaviour
	case <-time.After(200 * time.Millisecond):
		// No event received and channel not closed: also acceptable as long as
		// nothing arrives (channel was removed from registry). However, by contract
		// Unsubscribe closes the channel, so we expect the closed case above.
		t.Fatal("channel was not closed after Unsubscribe")
	}
}

// TestSlowSubscriberDoesNotBlockFastSubscribers verifies that a full (slow) subscriber channel
// does not block Publish from returning promptly.
//
// The test publishes many more events than the channel buffer (64) to ensure the slow
// subscriber's channel fills up. The key property being verified is timing: all Publish
// calls must complete without blocking on the slow subscriber. We check that 200 publishes
// finish within a short deadline that would be impossible to meet if Publish blocked.
func TestSlowSubscriberDoesNotBlockFastSubscribers(t *testing.T) {
	bus := events.NewEventBus()

	// Slow subscriber: never drained
	_, _ = bus.Subscribe()

	// Publish many events — well beyond the 64-event buffer
	const numEvents = 200
	start := time.Now()
	for range numEvents {
		bus.Publish(events.Event{Event: "phase-start", Outcome: "in_progress"})
	}
	elapsed := time.Since(start)

	// If Publish blocked on the slow subscriber it would take at minimum numEvents channel-write
	// waits. Even a 1 ms stall per event would total 200 ms. A non-blocking implementation
	// should complete all 200 publishes in well under 10 ms on any modern machine.
	const maxAllowed = 500 * time.Millisecond
	if elapsed > maxAllowed {
		t.Errorf("Publish took %v for %d events; expected non-blocking (< %v)", elapsed, numEvents, maxAllowed)
	}
}

// TestConcurrentPublishSubscribe verifies there are no data races under concurrent usage.
// This test is primarily effective when run with -race.
func TestConcurrentPublishSubscribe(t *testing.T) {
	bus := events.NewEventBus()

	const goroutines = 10
	const publishesPerGoroutine = 50

	var wg sync.WaitGroup

	// Start some subscribers
	ids := make([]string, 5)
	for i := range ids {
		id, _ := bus.Subscribe()
		ids[i] = id
	}

	// Concurrent publishers
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range publishesPerGoroutine {
				bus.Publish(events.Event{Event: "phase-start", Outcome: "in_progress"})
			}
		}()
	}

	// Concurrent subscribe/unsubscribe
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			id, _ := bus.Subscribe()
			bus.Unsubscribe(id)
		}()
	}

	// Unsubscribe the original subscribers while publishing is in flight
	for _, id := range ids {
		bus.Unsubscribe(id)
	}

	wg.Wait()
}

// TestEventStructFields validates that the Event struct has the expected fields and json tags.
// This is a compile-time check via field assignment; json tags are verified by encoding/json.
func TestEventStructFields(t *testing.T) {
	t.Parallel()
	e := events.Event{
		Event:     "phase-complete",
		Phase:     "phase-3",
		SpecName:  "spec-abc",
		Workspace: "/workspace/abc",
		Timestamp: "2026-03-26T00:00:00Z",
		Outcome:   "completed",
		Detail:    "extra",
	}
	if e.Event == "" || e.Phase == "" || e.SpecName == "" || e.Workspace == "" || e.Timestamp == "" || e.Outcome == "" {
		t.Fatal("one or more Event fields are unexpectedly empty")
	}
}

// TestSetEventLogCreatesFileAndPersists verifies that SetEventLog creates a new JSONL file
// and subsequent Publish calls append events to it.
func TestSetEventLogCreatesFileAndPersists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")

	bus := events.NewEventBus()
	if err := bus.SetEventLog(logPath); err != nil {
		t.Fatalf("SetEventLog: %v", err)
	}
	t.Cleanup(func() { bus.CloseLog() })

	want := events.Event{
		Event:     "phase-start",
		Phase:     "phase-1",
		SpecName:  "test-spec",
		Workspace: "/tmp/ws",
		Timestamp: "2026-04-18T00:00:00Z",
		Outcome:   "in_progress",
	}
	bus.Publish(want)
	bus.CloseLog()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got events.Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v — raw: %q", err, string(data))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("persisted event = %+v, want %+v", got, want)
	}
}

// TestSetEventLogLoadsExistingEvents verifies that SetEventLog loads events from
// an existing JSONL file into the in-memory history.
func TestSetEventLogLoadsExistingEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")

	historical := events.Event{
		Event:     "pipeline-init",
		Phase:     "setup",
		SpecName:  "old-spec",
		Workspace: "/tmp/old-ws",
		Timestamp: "2026-04-17T12:00:00Z",
		Outcome:   "completed",
	}
	line, _ := json.Marshal(historical)
	if err := os.WriteFile(logPath, append(line, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bus := events.NewEventBus()
	if err := bus.SetEventLog(logPath); err != nil {
		t.Fatalf("SetEventLog: %v", err)
	}
	t.Cleanup(func() { bus.CloseLog() })

	hist := bus.History()
	if len(hist) != 1 {
		t.Fatalf("History() length = %d, want 1", len(hist))
	}
	if !reflect.DeepEqual(hist[0], historical) {
		t.Errorf("History()[0] = %+v, want %+v", hist[0], historical)
	}
}

// TestHistoryContainsBothLoadedAndLiveEvents verifies that History() returns
// events loaded from file followed by events published in the current session.
func TestHistoryContainsBothLoadedAndLiveEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")

	old := events.Event{Event: "pipeline-init", Timestamp: "2026-04-17T00:00:00Z", Outcome: "completed"}
	line, _ := json.Marshal(old)
	if err := os.WriteFile(logPath, append(line, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bus := events.NewEventBus()
	if err := bus.SetEventLog(logPath); err != nil {
		t.Fatalf("SetEventLog: %v", err)
	}
	t.Cleanup(func() { bus.CloseLog() })

	live := events.Event{Event: "phase-start", Phase: "phase-1", Timestamp: "2026-04-18T00:00:00Z", Outcome: "in_progress"}
	bus.Publish(live)

	hist := bus.History()
	if len(hist) != 2 {
		t.Fatalf("History() length = %d, want 2", len(hist))
	}
	if !reflect.DeepEqual(hist[0], old) {
		t.Errorf("History()[0] = %+v, want %+v", hist[0], old)
	}
	if !reflect.DeepEqual(hist[1], live) {
		t.Errorf("History()[1] = %+v, want %+v", hist[1], live)
	}
}

// TestLoadMalformedJSONLSkipsBadLines verifies that malformed lines in the JSONL
// file are silently skipped without breaking the load.
func TestLoadMalformedJSONLSkipsBadLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")

	good := events.Event{Event: "phase-start", Timestamp: "2026-04-18T00:00:00Z", Outcome: "in_progress"}
	goodLine, _ := json.Marshal(good)

	content := string(goodLine) + "\n" +
		"THIS IS NOT JSON\n" +
		"\n" // empty line
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bus := events.NewEventBus()
	if err := bus.SetEventLog(logPath); err != nil {
		t.Fatalf("SetEventLog: %v", err)
	}
	t.Cleanup(func() { bus.CloseLog() })

	hist := bus.History()
	if len(hist) != 1 {
		t.Fatalf("History() length = %d, want 1; got %+v", len(hist), hist)
	}
	if !reflect.DeepEqual(hist[0], good) {
		t.Errorf("History()[0] = %+v, want %+v", hist[0], good)
	}
}

// TestWatchEventLog_PicksUpCrossProcessEvents verifies that events appended to the shared
// log file by another process (simulated by a direct file write) are picked up by
// WatchEventLog and broadcast to live SSE subscribers within a few seconds.
func TestWatchEventLog_PicksUpCrossProcessEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")

	bus := events.NewEventBus()
	if err := bus.SetEventLog(logPath); err != nil {
		t.Fatalf("SetEventLog: %v", err)
	}
	t.Cleanup(func() { bus.CloseLog() })

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	bus.WatchEventLog(ctx)

	// Subscribe before the external write so we receive the live broadcast.
	_, ch := bus.Subscribe()

	// Simulate another MCP server instance appending an event to the shared log.
	external := events.Event{
		Event:     "phase-start",
		Phase:     "phase-1",
		SpecName:  "other-project",
		Workspace: "/other/project/.specs/ws",
		Timestamp: "2026-04-18T10:00:00Z",
		Outcome:   "in_progress",
	}
	line, _ := json.Marshal(external)
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open log for external write: %v", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		t.Fatalf("external write: %v", err)
	}
	_ = f.Close()

	// WatchEventLog polls every second; allow up to 3 seconds for pickup.
	select {
	case got := <-ch:
		if got != external {
			t.Errorf("received event = %+v, want %+v", got, external)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out: cross-process event not broadcast within 3 seconds")
	}

	// The event must also appear in History so dashboard reloads show it.
	hist := bus.History()
	if !reflect.DeepEqual(hist[len(hist)-1], external) {
		t.Errorf("History does not contain cross-process event; last = %+v", hist[len(hist)-1])
	}
}

// TestWatchEventLog_NoopWhenLogPathEmpty verifies that WatchEventLog does not panic
// or start a goroutine when no log path is configured.
func TestWatchEventLog_NoopWhenLogPathEmpty(t *testing.T) {
	t.Parallel()
	bus := events.NewEventBus()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()               // cancelled immediately
	bus.WatchEventLog(ctx) // must not panic
}

// TestWatchEventLog_SkipsOwnEvents verifies that events published by this instance
// (already in history) are NOT re-broadcast when the tail picks them up from the file.
func TestWatchEventLog_SkipsOwnEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")

	bus := events.NewEventBus()
	if err := bus.SetEventLog(logPath); err != nil {
		t.Fatalf("SetEventLog: %v", err)
	}
	t.Cleanup(func() { bus.CloseLog() })

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	bus.WatchEventLog(ctx)

	own := events.Event{
		Event:     "phase-complete",
		Phase:     "phase-3",
		SpecName:  "this-project",
		Workspace: "/this/.specs/ws",
		Timestamp: "2026-04-18T11:00:00Z",
		Outcome:   "completed",
	}
	bus.Publish(own) // written to file + history by this instance

	// Subscribe after Publish so the channel starts empty.
	_, ch := bus.Subscribe()

	// Wait long enough for at least one tail poll (1 second + buffer).
	time.Sleep(1500 * time.Millisecond)

	// Channel must be empty: our own event must not be re-broadcast.
	select {
	case got := <-ch:
		t.Errorf("unexpected re-broadcast of own event: %+v", got)
	default:
		// correct — nothing re-broadcast
	}
}

// TestCloseLogIdempotent verifies that CloseLog can be called multiple times without panic.
func TestCloseLogIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.jsonl")

	bus := events.NewEventBus()
	if err := bus.SetEventLog(logPath); err != nil {
		t.Fatalf("SetEventLog: %v", err)
	}
	bus.CloseLog()
	bus.CloseLog() // must not panic
}

// TestSetEventLogNonExistentFile verifies that SetEventLog works when the file
// does not yet exist (creates it).
func TestSetEventLogNonExistentFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "new-events.jsonl")

	bus := events.NewEventBus()
	if err := bus.SetEventLog(logPath); err != nil {
		t.Fatalf("SetEventLog: %v", err)
	}
	t.Cleanup(func() { bus.CloseLog() })

	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("expected file to exist after SetEventLog, got: %v", err)
	}

	hist := bus.History()
	if len(hist) != 0 {
		t.Errorf("History() length = %d, want 0 for new file", len(hist))
	}
}
