package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhook_Send(t *testing.T) {
	var received AlertPayload
	var contentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &received))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := AlertPayload{
		TargetName: "my-service",
		URL:        "https://example.com",
		Status:     "DOWN",
		Error:      "connection refused",
		Latency:    "120ms",
		Timestamp:  time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		ConsecFail: 3,
	}

	w := NewWebhook(srv.URL)
	err := w.Send(context.Background(), payload)

	require.NoError(t, err)
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, payload.TargetName, received.TargetName)
	assert.Equal(t, payload.URL, received.URL)
	assert.Equal(t, payload.Status, received.Status)
	assert.Equal(t, payload.Error, received.Error)
	assert.Equal(t, payload.Latency, received.Latency)
	assert.Equal(t, payload.ConsecFail, received.ConsecFail)
	assert.Equal(t, payload.Timestamp.UTC(), received.Timestamp.UTC())
}

// TestWebhook_Post_BadURL verifies that post() returns an error when the URL
// results in a request-build failure (e.g. a URL with a space in the scheme).
func TestWebhook_Post_BadURL(t *testing.T) {
	w := &WebhookAlerter{
		url:    "://bad url with spaces",
		client: &http.Client{Timeout: 5 * time.Second},
	}
	err := w.post(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build webhook request")
}

// TestMarshalPayload_Panic verifies that marshalPayload panics when the
// underlying marshal function returns an error (simulated via the jsonMarshal
// variable).
func TestMarshalPayload_Panic(t *testing.T) {
	orig := jsonMarshal
	t.Cleanup(func() { jsonMarshal = orig })

	jsonMarshal = func(v any) ([]byte, error) {
		return nil, fmt.Errorf("simulated marshal error")
	}

	assert.Panics(t, func() {
		marshalPayload(AlertPayload{TargetName: "svc"})
	})
}

func TestWebhook_NetworkError(t *testing.T) {
	// Point at a port that is not listening — Do() returns a network error.
	w := NewWebhook("http://127.0.0.1:1") // port 1 is privileged and never open
	payload := AlertPayload{
		TargetName: "svc",
		URL:        "http://svc",
		Status:     "DOWN",
		Latency:    "0s",
		Timestamp:  time.Now(),
	}

	err := w.Send(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send webhook")
}

func TestWebhook_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	payload := AlertPayload{
		TargetName: "broken-service",
		URL:        "https://broken.example.com",
		Status:     "DOWN",
		Latency:    "0s",
		Timestamp:  time.Now(),
	}

	w := NewWebhook(srv.URL)
	err := w.Send(context.Background(), payload)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
