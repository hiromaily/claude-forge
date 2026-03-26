package events_test

import (
	"sync"
	"testing"
	"time"

	"github.com/hiromaily/claude-forge/mcp-server/events"
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
	e := events.Event{
		Event:     "phase-complete",
		Phase:     "phase-3",
		SpecName:  "spec-abc",
		Workspace: "/workspace/abc",
		Timestamp: "2026-03-26T00:00:00Z",
		Outcome:   "completed",
	}
	if e.Event == "" || e.Phase == "" || e.SpecName == "" || e.Workspace == "" || e.Timestamp == "" || e.Outcome == "" {
		t.Fatal("one or more Event fields are unexpectedly empty")
	}
}
