// Package stats provides per-target statistics tracking for Pulse.
package stats

import (
	"sort"
	"sync"
	"time"

	"github.com/samuelbailey123/pulse/internal/checker"
)

const defaultMaxSamples = 1000

// Tracker accumulates check results for a single target and computes
// aggregate statistics over a fixed-size sliding window of latency samples.
type Tracker struct {
	mu          sync.Mutex
	latencies   []time.Duration // sliding window, capacity = maxSamples
	totalChecks int64
	totalUp     int64
	lastResult  checker.Result
	maxSamples  int
	// statusHistory records the Status of every result to support ConsecFails.
	statusHistory []checker.Status
}

// Snapshot is a point-in-time view of a Tracker's statistics.
type Snapshot struct {
	// Min is the minimum observed latency over the sample window.
	Min time.Duration
	// Avg is the mean latency over the sample window.
	Avg time.Duration
	// Max is the maximum observed latency over the sample window.
	Max time.Duration
	// P95 is the 95th-percentile latency over the sample window.
	P95 time.Duration
	// Uptime is the percentage of checks that returned StatusUp (0.0–100.0).
	Uptime float64
	// TotalChecks is the cumulative number of checks recorded.
	TotalChecks int64
	// ConsecFails is the number of consecutive StatusDown results at the end of history.
	ConsecFails int
	// LastCheck is the timestamp of the most recent check.
	LastCheck time.Time
	// LastStatus is the status of the most recent check.
	LastStatus checker.Status
	// CertExpiry is the TLS certificate expiry time from the last check. Nil if unavailable.
	CertExpiry *time.Time
}

// NewTracker constructs a Tracker with the default sliding-window size (1000).
func NewTracker() *Tracker {
	return &Tracker{
		maxSamples: defaultMaxSamples,
	}
}

// Record stores the result of a single check into the tracker.
// It updates the latency window, cumulative counters, and status history.
func (t *Tracker) Record(r checker.Result) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.totalChecks++
	if r.Status == checker.StatusUp {
		t.totalUp++
	}

	// Maintain sliding window for latencies.
	if len(t.latencies) >= t.maxSamples {
		// Drop the oldest entry by shifting left.
		copy(t.latencies, t.latencies[1:])
		t.latencies = t.latencies[:len(t.latencies)-1]
	}
	t.latencies = append(t.latencies, r.Latency)

	// Maintain status history in parallel with the latency window so that
	// ConsecFails reflects only the same sample set.
	if len(t.statusHistory) >= t.maxSamples {
		copy(t.statusHistory, t.statusHistory[1:])
		t.statusHistory = t.statusHistory[:len(t.statusHistory)-1]
	}
	t.statusHistory = append(t.statusHistory, r.Status)

	t.lastResult = r
}

// Snapshot returns a consistent point-in-time view of the tracker's statistics.
func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.totalChecks == 0 {
		return Snapshot{}
	}

	// Work on a copy so we can sort without holding the lock longer than needed.
	latCopy := make([]time.Duration, len(t.latencies))
	copy(latCopy, t.latencies)

	var minL, maxL, sum time.Duration
	if len(latCopy) > 0 {
		minL = latCopy[0]
		maxL = latCopy[0]
		for _, l := range latCopy {
			sum += l
			if l < minL {
				minL = l
			}
			if l > maxL {
				maxL = l
			}
		}
	}

	var avg time.Duration
	if len(latCopy) > 0 {
		avg = sum / time.Duration(len(latCopy))
	}

	p95 := calcP95(latCopy)

	uptime := float64(t.totalUp) / float64(t.totalChecks) * 100.0

	consecFails := 0
	for i := len(t.statusHistory) - 1; i >= 0; i-- {
		if t.statusHistory[i] == checker.StatusDown {
			consecFails++
		} else {
			break
		}
	}

	return Snapshot{
		Min:         minL,
		Avg:         avg,
		Max:         maxL,
		P95:         p95,
		Uptime:      uptime,
		TotalChecks: t.totalChecks,
		ConsecFails: consecFails,
		LastCheck:   t.lastResult.Timestamp,
		LastStatus:  t.lastResult.Status,
		CertExpiry:  t.lastResult.CertExpiry,
	}
}

// calcP95 sorts a copy of the provided durations and returns the value at the
// 95th-percentile index. Callers must ensure len(latencies) > 0.
func calcP95(latencies []time.Duration) time.Duration {
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// Index of the 95th percentile: floor((n-1) * 0.95)
	idx := int(float64(len(sorted)-1) * 0.95)
	return sorted[idx]
}
