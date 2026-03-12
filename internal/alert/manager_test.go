package alert

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/stretchr/testify/assert"
)

// countingServer returns an httptest.Server whose handler increments *hits on
// each request, and a cleanup function.
func countingServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain body to avoid broken-pipe errors on the sender side.
		_, _ = io.Copy(io.Discard, r.Body)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
}

// downResult builds a StatusDown Result with the given error string.
func makeDown(errMsg string) checker.Result {
	var err error
	if errMsg != "" {
		err = &errString{errMsg}
	}
	return checker.Result{
		Status:    checker.StatusDown,
		Latency:   0,
		Error:     err,
		Timestamp: time.Now(),
	}
}

// upResult builds a StatusUp Result.
func makeUp() checker.Result {
	return checker.Result{
		Status:    checker.StatusUp,
		Latency:   50 * time.Millisecond,
		Timestamp: time.Now(),
	}
}

// errString is a minimal error implementation for tests.
type errString struct{ s string }

func (e *errString) Error() string { return e.s }

// waitForHits polls until *hits reaches the expected count or the deadline is exceeded.
func waitForHits(t *testing.T, hits *atomic.Int32, expected int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hits.Load() >= expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d alert hits, got %d", expected, hits.Load())
}

func TestManager_NoAlertBeforeThreshold(t *testing.T) {
	var hits atomic.Int32
	srv := countingServer(t, &hits)
	defer srv.Close()

	mgr := NewManager(nil)
	target := config.Target{
		Name: "svc",
		URL:  "http://svc",
		Alerts: []config.Alert{
			{Type: "webhook", URL: srv.URL, After: 3},
		},
	}

	// 2 failures with after=3 — no alert should fire.
	mgr.Evaluate(context.Background(), target, makeDown("err"))
	mgr.Evaluate(context.Background(), target, makeDown("err"))

	// Give goroutines a moment then confirm nothing was sent.
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), hits.Load())
}

func TestManager_AlertOnThreshold(t *testing.T) {
	var hits atomic.Int32
	srv := countingServer(t, &hits)
	defer srv.Close()

	mgr := NewManager(nil)
	target := config.Target{
		Name: "svc",
		URL:  "http://svc",
		Alerts: []config.Alert{
			{Type: "webhook", URL: srv.URL, After: 3},
		},
	}

	// Exactly 3 failures — alert must fire.
	mgr.Evaluate(context.Background(), target, makeDown("err"))
	mgr.Evaluate(context.Background(), target, makeDown("err"))
	mgr.Evaluate(context.Background(), target, makeDown("err"))

	waitForHits(t, &hits, 1)
	assert.Equal(t, int32(1), hits.Load())
}

func TestManager_NoAlertSpam(t *testing.T) {
	var hits atomic.Int32
	srv := countingServer(t, &hits)
	defer srv.Close()

	mgr := NewManager(nil)
	target := config.Target{
		Name: "svc",
		URL:  "http://svc",
		Alerts: []config.Alert{
			{Type: "webhook", URL: srv.URL, After: 2},
		},
	}

	// Threshold is 2. Send 5 consecutive failures.
	for i := 0; i < 5; i++ {
		mgr.Evaluate(context.Background(), target, makeDown("err"))
	}

	// Only one DOWN alert should have fired regardless of extra failures.
	waitForHits(t, &hits, 1)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), hits.Load())
}

func TestManager_RecoveryAlert(t *testing.T) {
	var hits atomic.Int32
	var payloads []AlertPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p AlertPayload
		_ = json.Unmarshal(body, &p)
		payloads = append(payloads, p)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mgr := NewManager(nil)
	target := config.Target{
		Name: "svc",
		URL:  "http://svc",
		Alerts: []config.Alert{
			{Type: "webhook", URL: srv.URL, After: 1},
		},
	}

	// One failure triggers DOWN alert.
	mgr.Evaluate(context.Background(), target, makeDown("err"))
	waitForHits(t, &hits, 1)

	// Recovery triggers RECOVERY alert.
	mgr.Evaluate(context.Background(), target, makeUp())
	waitForHits(t, &hits, 2)

	assert.Equal(t, int32(2), hits.Load())

	// Verify the statuses: payloads may arrive in any order due to goroutines,
	// so collect the status strings from whichever order they landed.
	statuses := make([]string, 0, len(payloads))
	for _, p := range payloads {
		statuses = append(statuses, p.Status)
	}
	assert.Contains(t, statuses, "DOWN")
	assert.Contains(t, statuses, "RECOVERY")
}

// TestManager_DuplicateURLDedup verifies that when a target alert and a global
// alert share the same webhook URL, the webhook is called only once per event.
func TestManager_DuplicateURLDedup(t *testing.T) {
	var hits atomic.Int32
	srv := countingServer(t, &hits)
	defer srv.Close()

	// Both target and global point at the same URL.
	globalAlerts := []config.Alert{
		{Type: "webhook", URL: srv.URL, After: 1},
	}
	mgr := NewManager(globalAlerts)

	target := config.Target{
		Name: "svc",
		URL:  "http://svc",
		Alerts: []config.Alert{
			{Type: "webhook", URL: srv.URL, After: 1},
		},
	}

	mgr.Evaluate(context.Background(), target, makeDown("err"))

	waitForHits(t, &hits, 1)
	time.Sleep(100 * time.Millisecond)
	// Despite two alert definitions pointing at the same URL, the webhook
	// must be called exactly once.
	assert.Equal(t, int32(1), hits.Load())
}

func TestManager_GlobalAndTargetAlerts(t *testing.T) {
	var targetHits atomic.Int32
	var globalHits atomic.Int32

	targetSrv := countingServer(t, &targetHits)
	defer targetSrv.Close()

	globalSrv := countingServer(t, &globalHits)
	defer globalSrv.Close()

	globalAlerts := []config.Alert{
		{Type: "webhook", URL: globalSrv.URL, After: 1},
	}
	mgr := NewManager(globalAlerts)

	target := config.Target{
		Name: "svc",
		URL:  "http://svc",
		Alerts: []config.Alert{
			{Type: "webhook", URL: targetSrv.URL, After: 1},
		},
	}

	// One failure — both target and global alert should fire.
	mgr.Evaluate(context.Background(), target, makeDown("err"))

	waitForHits(t, &targetHits, 1)
	waitForHits(t, &globalHits, 1)

	assert.Equal(t, int32(1), targetHits.Load())
	assert.Equal(t, int32(1), globalHits.Load())
}
