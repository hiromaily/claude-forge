package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// slackPayload is the JSON body sent to a Slack incoming-webhook endpoint.
type slackPayload struct {
	Text string `json:"text"`
}

// SlackNotifier posts a formatted message to a Slack incoming webhook for selected
// phase-transition events. It is safe to call Notify concurrently.
type SlackNotifier struct {
	webhookURL string
	httpClient *http.Client
}

// NewSlackNotifier constructs a SlackNotifier that will POST to webhookURL.
// If webhookURL is empty, the notifier is disabled and Notify is a no-op.
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled reports whether the notifier has a non-empty webhook URL.
func (n *SlackNotifier) Enabled() bool {
	return n.webhookURL != ""
}

// notifyEventTypes is the set of event types that trigger a Slack notification.
var notifyEventTypes = map[string]bool{
	"phase-complete": true,
	"phase-fail":     true,
	"abandon":        true,
}

// Notify fires an asynchronous HTTP POST to the Slack webhook for events whose
// type is "phase-complete", "phase-fail", or "abandon". All other event types and
// calls on a disabled notifier are silently ignored. Notify always returns
// immediately without blocking the caller.
func (n *SlackNotifier) Notify(e Event) {
	if !n.Enabled() {
		return
	}
	if !notifyEventTypes[e.Event] {
		return
	}

	go func() {
		if err := n.post(e); err != nil {
			log.Printf("slack notify error: %v", err)
		}
	}()
}

// post constructs a Slack payload and POSTs it to the webhook URL.
func (n *SlackNotifier) post(e Event) error {
	text := fmt.Sprintf("[forge] *%s* — phase: %s | spec: %s | outcome: %s | %s",
		e.Event, e.Phase, e.SpecName, e.Outcome, e.Timestamp)

	payload := slackPayload{Text: text}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	resp, err := n.httpClient.Post(n.webhookURL, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("post to slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}
