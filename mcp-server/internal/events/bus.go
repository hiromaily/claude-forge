// provides an in-process event bus for pipeline phase transition notifications.

package events

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Event represents a pipeline lifecycle notification.
type Event struct {
	// Event type. Phase-level: "phase-start", "phase-complete", "phase-fail",
	// "checkpoint", "abandon". Fine-grained: "pipeline-init", "pipeline-complete",
	// "agent-dispatch", "action-complete", "revision-required".
	Event     string `json:"event"`
	Phase     string `json:"phase"`            // e.g. "phase-3"
	SpecName  string `json:"specName"`         // from state.SpecName
	Workspace string `json:"workspace"`        // absolute path passed to the tool
	Timestamp string `json:"timestamp"`        // RFC3339 UTC
	Outcome   string `json:"outcome"`          // "in_progress" | "completed" | "failed" | "awaiting_human" | "abandoned" | "dispatched"
	Detail    string `json:"detail,omitempty"` // optional extra info (e.g. agent name, action type)
}

const subscriberBufferSize = 64

// EventBus manages a set of subscriber channels and broadcasts published events to all of them.
// Concurrent Publish calls are safe and do not serialize against each other.
// Subscribe and Unsubscribe serialize against all other operations.
//
// When a log file path is configured via SetEventLog, events are persisted to a JSONL file
// so that dashboard reloads and new sessions can replay historical events.
//
// WatchEventLog enables multi-process event sharing: events written to the shared log by
// other MCP server instances (e.g. sessions from other projects) are tailed and broadcast
// to live SSE subscribers, so a single dashboard port shows all active pipelines.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan Event
	nextID      atomic.Uint64

	// history stores all events seen during this process lifetime plus any
	// loaded from the JSONL log file on startup.
	// histMap is a dedup index keyed by histKey(e) for O(1) lookup in addAndBroadcast.
	// Both are protected by histMu.
	histMu  sync.RWMutex
	history []Event
	histMap map[string]struct{}

	// logFile is the append-only JSONL event log. nil when persistence is disabled.
	// logPath and tailOffset are set once by SetEventLog before any concurrent access.
	logMu      sync.Mutex
	logFile    *os.File
	logPath    string
	tailOffset atomic.Int64
}

// NewEventBus constructs a new, empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string]chan Event),
		histMap:     make(map[string]struct{}),
	}
}

// histKey returns a string that uniquely identifies an event for deduplication.
// It uses the fields most likely to distinguish separate events: timestamp, workspace,
// event type, phase, and outcome. The Detail field is included to handle edge cases
// such as two agent-dispatch events for the same phase (parallel tasks).
func histKey(e Event) string {
	return e.Timestamp + "|" + e.Workspace + "|" + e.Event + "|" + e.Phase + "|" + e.Outcome + "|" + e.Detail
}

// SetEventLog configures JSONL-based event persistence. It loads any existing
// events from the file into the in-memory history, then opens the file in
// append mode for future writes. Errors are returned but non-fatal — the bus
// continues to work without persistence.
//
// Durability: writes are best-effort with no fsync per event. A process crash
// may lose the last few events buffered in the OS page cache.
//
// Cross-process safety: multiple MCP server processes may append to the same
// file concurrently. On POSIX, O_APPEND writes smaller than PIPE_BUF (~4 KB)
// are atomic. Individual event JSON lines are well under this limit, so
// interleaving is unlikely in practice, but not formally guaranteed on all
// platforms (notably macOS does not document atomicity for regular files).
func (b *EventBus) SetEventLog(path string) error {
	// Load existing events from the file (if it exists).
	loaded, err := loadEventsFromFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "forge-state: event log load warning: %v\n", err)
	}
	if len(loaded) > 0 {
		b.histMu.Lock()
		b.history = append(loaded, b.history...)
		for _, e := range loaded {
			b.histMap[histKey(e)] = struct{}{}
		}
		b.histMu.Unlock()
		fmt.Fprintf(os.Stderr, "forge-state: loaded %d historical events from %s\n", len(loaded), path)
	}

	// Record file size after loading so WatchEventLog starts tailing from here.
	// Events before this offset are already in history; we only broadcast new appends.
	if fi, statErr := os.Stat(path); statErr == nil {
		b.tailOffset.Store(fi.Size())
	}

	// Open (or create) the file in append mode for future writes.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open event log for append: %w", err)
	}
	b.logMu.Lock()
	b.logFile = f
	b.logPath = path
	b.logMu.Unlock()
	return nil
}

// WatchEventLog starts a background goroutine that tails the shared event log file
// and broadcasts new events written by other MCP server processes to live SSE subscribers.
// This enables a single dashboard to show events from all active project sessions.
// Events already in history (written by this instance) are deduplicated and skipped.
// The goroutine exits when ctx is cancelled.
func (b *EventBus) WatchEventLog(ctx context.Context) {
	b.logMu.Lock()
	path := b.logPath
	b.logMu.Unlock()
	if path == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		offset := b.tailOffset.Load()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				offset = b.readTail(path, offset)
			}
		}
	}()
}

// readTail reads new events appended to path since offset, calls addAndBroadcast for each,
// and returns the new file offset to resume from next time.
func (b *EventBus) readTail(path string, offset int64) int64 {
	// Stat before opening: skip the open syscall entirely when nothing is new.
	fi, err := os.Stat(path)
	if err != nil || fi.Size() <= offset {
		return offset
	}

	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset
	}

	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")
		if len(line) > 0 {
			var e Event
			if jsonErr := json.Unmarshal(line, &e); jsonErr == nil {
				b.addAndBroadcast(e)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			break
		}
	}

	// Use the file size at the start of this read as the new offset.
	// Any bytes appended during the read will be re-processed next tick;
	// addAndBroadcast deduplicates against history so re-processing is safe.
	return fi.Size()
}

// addAndBroadcast adds e to history if not already present and broadcasts it to live
// subscribers. It does NOT write to the log file (the event is already there —
// this is called by readTail for events written by other MCP server processes).
func (b *EventBus) addAndBroadcast(e Event) {
	b.histMu.Lock()
	if _, ok := b.histMap[histKey(e)]; ok {
		b.histMu.Unlock()
		return // already in history — our own event or a cross-process duplicate
	}
	b.history = append(b.history, e)
	b.histMap[histKey(e)] = struct{}{}
	b.histMu.Unlock()

	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
		}
	}
}

// History returns a copy of all historical events (loaded from file + current session).
func (b *EventBus) History() []Event {
	b.histMu.RLock()
	defer b.histMu.RUnlock()
	out := make([]Event, len(b.history))
	copy(out, b.history)
	return out
}

// Subscribe registers a new subscriber and returns its unique ID and a read-only channel.
// The channel has a buffer of 64 events. Events are dropped (not delivered) when the buffer is full.
func (b *EventBus) Subscribe() (id string, ch <-chan Event) {
	rawID := b.nextID.Add(1)
	id = fmt.Sprintf("sub-%d", rawID)
	c := make(chan Event, subscriberBufferSize)

	b.mu.Lock()
	b.subscribers[id] = c
	b.mu.Unlock()

	return id, c
}

// Unsubscribe removes the subscriber with the given ID and closes its channel.
// Calling Unsubscribe with an unknown ID is a no-op.
func (b *EventBus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	c, ok := b.subscribers[id]
	if !ok {
		return
	}
	delete(b.subscribers, id)
	close(c)
}

// Publish broadcasts e to all current subscribers using non-blocking sends.
// Events are dropped for any subscriber whose channel buffer is full.
// Publish acquires only a read lock so concurrent Publish calls do not serialize.
// The event is also appended to the in-memory history and JSONL log file.
func (b *EventBus) Publish(e Event) {
	// Append to in-memory history and dedup index.
	b.histMu.Lock()
	b.history = append(b.history, e)
	b.histMap[histKey(e)] = struct{}{}
	b.histMu.Unlock()

	// Persist to JSONL log file (best-effort).
	// Single Write call keeps the line atomic up to PIPE_BUF on POSIX systems.
	b.logMu.Lock()
	if b.logFile != nil {
		if data, err := json.Marshal(e); err == nil {
			_, _ = b.logFile.Write(append(data, '\n'))
		}
	}
	b.logMu.Unlock()

	// Broadcast to live subscribers.
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- e:
		default:
			// subscriber is slow; drop the event rather than blocking
		}
	}
}

// CloseLog closes the JSONL log file if open. Safe to call multiple times.
func (b *EventBus) CloseLog() {
	b.logMu.Lock()
	defer b.logMu.Unlock()
	if b.logFile != nil {
		_ = b.logFile.Close()
		b.logFile = nil
	}
}

// loadEventsFromFile reads a JSONL file and returns the parsed events.
// Returns nil, nil if the file does not exist.
func loadEventsFromFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open event log: %w", err)
	}
	defer func() { _ = f.Close() }()

	var result []Event
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")
		if len(line) > 0 {
			var e Event
			if jsonErr := json.Unmarshal(line, &e); jsonErr == nil {
				result = append(result, e)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, fmt.Errorf("read event log: %w", err)
		}
	}
	return result, nil
}
