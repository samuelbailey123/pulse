package stats

import (
	"testing"
	"time"

	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upResult is a convenience helper that builds a StatusUp Result with the
// given latency and a zero Timestamp.
func upResult(latency time.Duration) checker.Result {
	return checker.Result{
		Status:    checker.StatusUp,
		Latency:   latency,
		Timestamp: time.Now(),
	}
}

// downResult is a convenience helper for StatusDown results.
func downResult() checker.Result {
	return checker.Result{
		Status:    checker.StatusDown,
		Latency:   0,
		Timestamp: time.Now(),
	}
}

func TestTracker_Empty(t *testing.T) {
	tr := NewTracker()
	snap := tr.Snapshot()

	assert.Equal(t, time.Duration(0), snap.Min)
	assert.Equal(t, time.Duration(0), snap.Avg)
	assert.Equal(t, time.Duration(0), snap.Max)
	assert.Equal(t, time.Duration(0), snap.P95)
	assert.Equal(t, float64(0), snap.Uptime)
	assert.Equal(t, int64(0), snap.TotalChecks)
	assert.Equal(t, 0, snap.ConsecFails)
	assert.True(t, snap.LastCheck.IsZero())
}

func TestTracker_SingleRecord(t *testing.T) {
	tr := NewTracker()
	now := time.Now()
	r := checker.Result{
		Status:    checker.StatusUp,
		Latency:   50 * time.Millisecond,
		Timestamp: now,
	}
	tr.Record(r)

	snap := tr.Snapshot()

	assert.Equal(t, int64(1), snap.TotalChecks)
	assert.Equal(t, 50*time.Millisecond, snap.Min)
	assert.Equal(t, 50*time.Millisecond, snap.Avg)
	assert.Equal(t, 50*time.Millisecond, snap.Max)
	assert.Equal(t, float64(100), snap.Uptime)
	assert.Equal(t, checker.StatusUp, snap.LastStatus)
	assert.Equal(t, 0, snap.ConsecFails)
	assert.WithinDuration(t, now, snap.LastCheck, time.Second)
}

func TestTracker_MinAvgMax(t *testing.T) {
	tr := NewTracker()
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}
	for _, l := range latencies {
		tr.Record(upResult(l))
	}

	snap := tr.Snapshot()

	assert.Equal(t, 10*time.Millisecond, snap.Min)
	assert.Equal(t, 50*time.Millisecond, snap.Max)
	// avg = (10+20+30+40+50)/5 = 30ms
	assert.Equal(t, 30*time.Millisecond, snap.Avg)
}

func TestTracker_P95(t *testing.T) {
	tr := NewTracker()

	// Record 100 results with latencies 1ms … 100ms.
	for i := 1; i <= 100; i++ {
		tr.Record(upResult(time.Duration(i) * time.Millisecond))
	}

	snap := tr.Snapshot()

	// sorted[0..99], idx = floor(99 * 0.95) = floor(94.05) = 94
	// sorted[94] = 95ms (1-indexed values stored in sorted order)
	assert.Equal(t, 95*time.Millisecond, snap.P95)
}

func TestTracker_Uptime(t *testing.T) {
	tr := NewTracker()

	// 3 up, 1 down → 75%
	tr.Record(upResult(10 * time.Millisecond))
	tr.Record(upResult(10 * time.Millisecond))
	tr.Record(upResult(10 * time.Millisecond))
	tr.Record(downResult())

	snap := tr.Snapshot()

	assert.Equal(t, int64(4), snap.TotalChecks)
	assert.InDelta(t, 75.0, snap.Uptime, 0.001)
}

func TestTracker_ConsecutiveFails(t *testing.T) {
	tr := NewTracker()

	tr.Record(upResult(10 * time.Millisecond))
	tr.Record(downResult())
	tr.Record(downResult())

	snap := tr.Snapshot()
	assert.Equal(t, 2, snap.ConsecFails)
}

func TestTracker_ConsecutiveFails_ResetOnUp(t *testing.T) {
	tr := NewTracker()

	tr.Record(downResult())
	tr.Record(downResult())
	tr.Record(upResult(10 * time.Millisecond))

	snap := tr.Snapshot()
	assert.Equal(t, 0, snap.ConsecFails)
}

func TestTracker_SlidingWindow(t *testing.T) {
	tr := NewTracker()
	require.Equal(t, defaultMaxSamples, tr.maxSamples)

	// Fill beyond the window with a known early latency, then add one more
	// with a distinct latency to confirm the window rolls over correctly.
	for i := 0; i < defaultMaxSamples; i++ {
		tr.Record(upResult(100 * time.Millisecond))
	}

	// Now push one entry with a very different latency.
	tr.Record(upResult(1 * time.Millisecond))

	snap := tr.Snapshot()

	// The total checks should be 1001.
	assert.Equal(t, int64(defaultMaxSamples+1), snap.TotalChecks)

	// The window still holds maxSamples entries.
	tr.mu.Lock()
	windowSize := len(tr.latencies)
	tr.mu.Unlock()
	assert.Equal(t, defaultMaxSamples, windowSize)

	// Min should now be 1ms (the newly-added entry replaced the oldest 100ms).
	assert.Equal(t, 1*time.Millisecond, snap.Min)
}
