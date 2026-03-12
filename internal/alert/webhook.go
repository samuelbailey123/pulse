package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// jsonMarshal is the JSON serialisation function used by marshalPayload.
// It is a package-level variable so tests can substitute a failing stub.
var jsonMarshal = json.Marshal

// WebhookAlerter sends an AlertPayload as a JSON POST request to a configured URL.
type WebhookAlerter struct {
	url    string
	client *http.Client
}

// NewWebhook constructs a WebhookAlerter that posts to the given URL.
// A default HTTP client with a 10-second timeout is used.
func NewWebhook(url string) *WebhookAlerter {
	return &WebhookAlerter{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send serialises payload to JSON and POSTs it to the webhook URL.
// It returns an error if the request cannot be sent or the server responds
// with a non-2xx status code.
func (w *WebhookAlerter) Send(ctx context.Context, payload AlertPayload) error {
	return w.post(ctx, marshalPayload(payload))
}

// marshalPayload serialises an AlertPayload to JSON.
// AlertPayload contains only JSON-safe types (string, int, time.Time), so
// the underlying marshal call should never fail. A panic here signals a
// programming error, not a runtime condition.
func marshalPayload(payload AlertPayload) []byte {
	b, err := jsonMarshal(payload)
	if err != nil {
		panic(fmt.Sprintf("alert: failed to marshal AlertPayload (programming error): %v", err))
	}
	return b
}

// post sends pre-marshalled JSON to the webhook URL and returns an error for
// network failures or non-2xx responses.
func (w *WebhookAlerter) post(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook to %s: %w", w.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook %s returned status %d", w.url, resp.StatusCode)
	}

	return nil
}
