package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHTTPTarget returns a config.Target pointing at the given httptest server URL
// with a short interval appropriate for tests.
func newHTTPTarget(name, url string) config.Target {
	return config.Target{
		Name:     name,
		URL:      url,
		Type:     "http",
		Method:   "GET",
		Interval: config.Duration{Duration: 50 * time.Millisecond},
		Timeout:  config.Duration{Duration: 5 * time.Second},
	}
}

// newTestServer returns an httptest.Server that always responds with 200 OK.
// The server is closed automatically when the test ends.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newStatusServer returns an httptest.Server that always responds with the
// provided HTTP status code. The server is closed automatically when the test ends.
func newStatusServer(t *testing.T, statusCode int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestEngine_RunOnce creates two targets — one backed by a 200 server and one
// backed by a 500 server — and verifies that RunOnce returns the correct status
// for each. The 200 target must be StatusUp; the 500 target must be StatusDown
// and carry a non-nil error.
func TestEngine_RunOnce(t *testing.T) {
	srvUp := newStatusServer(t, http.StatusOK)
	srvDown := newStatusServer(t, http.StatusInternalServerError)

	cfg := &config.Config{
		Targets: []config.Target{
			newHTTPTarget("svc-up", srvUp.URL),
			newHTTPTarget("svc-down", srvDown.URL),
		},
	}

	eng := New(cfg)
	results := eng.RunOnce(context.Background())

	require.Len(t, results, 2)

	byName := make(map[string]TargetResult, 2)
	for _, r := range results {
		byName[r.Target.Name] = r
	}

	require.Contains(t, byName, "svc-up")
	require.Contains(t, byName, "svc-down")

	up := byName["svc-up"]
	assert.Equal(t, checker.StatusUp, up.Result.Status, "200 server must produce StatusUp")
	assert.Equal(t, int64(1), up.Stats.TotalChecks)
	assert.NoError(t, up.Result.Error)

	down := byName["svc-down"]
	assert.Equal(t, checker.StatusDown, down.Result.Status, "500 server must produce StatusDown")
	assert.Equal(t, int64(1), down.Stats.TotalChecks)
	require.Error(t, down.Result.Error, "500 server must carry a non-nil error")
}

// TestEngine_Start_ReceivesResults starts the engine against two httptest servers
// with a short interval. It reads from the output channel until at least one
// result per target has arrived, then cancels and verifies the channel drains.
func TestEngine_Start_ReceivesResults(t *testing.T) {
	srv1 := newTestServer(t)
	srv2 := newTestServer(t)

	cfg := &config.Config{
		Targets: []config.Target{
			newHTTPTarget("svc-a", srv1.URL),
			newHTTPTarget("svc-b", srv2.URL),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	eng := New(cfg)
	ch := eng.Start(ctx)

	received := make(map[string]struct{})
	for r := range ch {
		received[r.Target.Name] = struct{}{}
		if len(received) == len(cfg.Targets) {
			cancel()
		}
	}

	assert.Contains(t, received, "svc-a")
	assert.Contains(t, received, "svc-b")
}

// TestEngine_Start_TickerFires verifies the ticker branch inside runTarget.
// It uses a 30 ms interval and collects at least 3 results from a single target,
// confirming that both the initial check and subsequent ticker-driven checks fire.
func TestEngine_Start_TickerFires(t *testing.T) {
	srv := newTestServer(t)

	cfg := &config.Config{
		Targets: []config.Target{
			{
				Name:     "svc",
				URL:      srv.URL,
				Type:     "http",
				Method:   "GET",
				Interval: config.Duration{Duration: 30 * time.Millisecond},
				Timeout:  config.Duration{Duration: 5 * time.Second},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := New(cfg)
	ch := eng.Start(ctx)

	const wantResults = 3
	count := 0
	deadline := time.After(5 * time.Second)
	for count < wantResults {
		select {
		case r, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed after only %d results; wanted %d", count, wantResults)
			}
			assert.Equal(t, checker.StatusUp, r.Result.Status)
			count++
		case <-deadline:
			t.Fatalf("timed out after collecting only %d of %d results", count, wantResults)
		}
	}

	cancel()
	for range ch {
	}
}

// TestEngine_Start_MultipleTickResults is a broader variant of TickerFires that
// uses the channel-range pattern and confirms the final count via an assertion.
func TestEngine_Start_MultipleTickResults(t *testing.T) {
	srv := newTestServer(t)

	cfg := &config.Config{
		Targets: []config.Target{
			{
				Name:     "svc",
				URL:      srv.URL,
				Type:     "http",
				Method:   "GET",
				Interval: config.Duration{Duration: 30 * time.Millisecond},
				Timeout:  config.Duration{Duration: 5 * time.Second},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	eng := New(cfg)
	ch := eng.Start(ctx)

	count := 0
	for r := range ch {
		assert.Equal(t, "svc", r.Target.Name)
		count++
		if count >= 3 {
			cancel()
		}
	}

	assert.GreaterOrEqual(t, count, 3, "expected at least 3 results from ticker")
}

// TestEngine_Start_Shutdown verifies that cancelling the context causes the
// output channel to close promptly and all goroutines to exit cleanly. A long
// interval ensures the shutdown signal comes from context cancellation, not a tick.
func TestEngine_Start_Shutdown(t *testing.T) {
	srv := newTestServer(t)

	cfg := &config.Config{
		Targets: []config.Target{
			{
				Name:     "svc",
				URL:      srv.URL,
				Type:     "http",
				Method:   "GET",
				Interval: config.Duration{Duration: 10 * time.Second},
				Timeout:  config.Duration{Duration: 5 * time.Second},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	eng := New(cfg)
	ch := eng.Start(ctx)

	select {
	case r, ok := <-ch:
		require.True(t, ok, "expected a result before shutdown")
		assert.Equal(t, "svc", r.Target.Name)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first result")
	}

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			for range ch {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for channel close after context cancel")
	}
}

// TestEngine_RunOnce_UnknownType verifies that a target with an unsupported type
// (e.g. "grpc") produces a single StatusDown result carrying a descriptive error
// that references the unknown type name.
func TestEngine_RunOnce_UnknownType(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.Target{
			{
				Name:     "bad-type",
				URL:      "http://example.com",
				Type:     "grpc",
				Method:   "GET",
				Interval: config.Duration{Duration: 30 * time.Second},
				Timeout:  config.Duration{Duration: 5 * time.Second},
			},
		},
	}

	eng := New(cfg)
	results := eng.RunOnce(context.Background())

	require.Len(t, results, 1)
	assert.Equal(t, checker.StatusDown, results[0].Result.Status)
	require.Error(t, results[0].Result.Error)
	assert.Contains(t, results[0].Result.Error.Error(), "grpc")
}

// TestEngine_Start_UnknownType verifies that Start emits a single StatusDown
// result for an unsupported target type and then closes the channel without
// entering the ticker loop.
func TestEngine_Start_UnknownType(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.Target{
			{
				Name:     "bad-type",
				URL:      "http://example.com",
				Type:     "grpc",
				Method:   "GET",
				Interval: config.Duration{Duration: 30 * time.Second},
				Timeout:  config.Duration{Duration: 5 * time.Second},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	eng := New(cfg)
	ch := eng.Start(ctx)

	var got TargetResult
	select {
	case r, ok := <-ch:
		require.True(t, ok)
		got = r
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error result")
	}

	assert.Equal(t, checker.StatusDown, got.Result.Status)
	require.Error(t, got.Result.Error)

	select {
	case _, ok := <-ch:
		if ok {
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

// TestEngine_Snapshots calls RunOnce against a healthy server and an unhealthy
// server, then asserts that Snapshots returns tracker data for every target with
// TotalChecks > 0, correct LastStatus, a non-zero LastCheck timestamp, and
// accurate Uptime and ConsecFails values.
func TestEngine_Snapshots(t *testing.T) {
	srvUp := newStatusServer(t, http.StatusOK)
	srvDown := newStatusServer(t, http.StatusInternalServerError)

	cfg := &config.Config{
		Targets: []config.Target{
			newHTTPTarget("svc-up", srvUp.URL),
			newHTTPTarget("svc-down", srvDown.URL),
		},
	}

	eng := New(cfg)
	eng.RunOnce(context.Background())

	snaps := eng.Snapshots()

	require.Contains(t, snaps, "svc-up")
	require.Contains(t, snaps, "svc-down")

	snapUp := snaps["svc-up"]
	assert.Equal(t, int64(1), snapUp.TotalChecks, "healthy target must have TotalChecks=1")
	assert.Equal(t, checker.StatusUp, snapUp.LastStatus)
	assert.False(t, snapUp.LastCheck.IsZero(), "LastCheck must be populated after a check")
	assert.InDelta(t, 100.0, snapUp.Uptime, 0.001, "healthy target must have 100%% uptime")
	assert.Equal(t, 0, snapUp.ConsecFails)

	snapDown := snaps["svc-down"]
	assert.Equal(t, int64(1), snapDown.TotalChecks, "failing target must have TotalChecks=1")
	assert.Equal(t, checker.StatusDown, snapDown.LastStatus)
	assert.False(t, snapDown.LastCheck.IsZero(), "LastCheck must be populated after a check")
	assert.InDelta(t, 0.0, snapDown.Uptime, 0.001, "failing target must have 0%% uptime")
	assert.Equal(t, 1, snapDown.ConsecFails)
}

// TestEngine_Snapshots_Empty verifies that Snapshots returns zero-value entries
// for targets that have never been checked (TotalChecks == 0).
func TestEngine_Snapshots_Empty(t *testing.T) {
	cfg := &config.Config{
		Targets: []config.Target{
			newHTTPTarget("svc", "http://127.0.0.1:1"),
		},
	}

	eng := New(cfg)

	snaps := eng.Snapshots()
	require.Contains(t, snaps, "svc")
	assert.Equal(t, int64(0), snaps["svc"].TotalChecks)
}

// TestEngine_RunOnce_ResultCount verifies that RunOnce always returns exactly
// len(cfg.Targets) results, even when individual targets fail.
func TestEngine_RunOnce_ResultCount(t *testing.T) {
	targets := []config.Target{
		newHTTPTarget("a", newStatusServer(t, http.StatusOK).URL),
		newHTTPTarget("b", newStatusServer(t, http.StatusOK).URL),
		newHTTPTarget("c", newStatusServer(t, http.StatusInternalServerError).URL),
	}

	cfg := &config.Config{Targets: targets}
	eng := New(cfg)
	results := eng.RunOnce(context.Background())

	assert.Len(t, results, len(targets), "RunOnce must return exactly one result per target")
}
