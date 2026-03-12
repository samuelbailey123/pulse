// Package engine orchestrates parallel health checks across all configured targets.
package engine

import (
	"context"
	"sync"
	"time"

	"github.com/samuelbailey123/pulse/internal/alert"
	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/samuelbailey123/pulse/internal/stats"
)

// TargetResult bundles a check result with the originating target and the
// current statistics snapshot for that target.
type TargetResult struct {
	Target config.Target
	Result checker.Result
	Stats  stats.Snapshot
}

// Engine orchestrates concurrent health checks and aggregates their results.
type Engine struct {
	cfg      *config.Config
	trackers map[string]*stats.Tracker
	alertMgr *alert.Manager
	mu       sync.RWMutex
}

// New constructs an Engine from the supplied configuration.
// A Tracker and an alert Manager are initialised but no goroutines are started.
func New(cfg *config.Config) *Engine {
	trackers := make(map[string]*stats.Tracker, len(cfg.Targets))
	for _, t := range cfg.Targets {
		trackers[t.Name] = stats.NewTracker()
	}

	return &Engine{
		cfg:      cfg,
		trackers: trackers,
		alertMgr: alert.NewManager(cfg.Alerts),
	}
}

// Start launches one goroutine per configured target. Each goroutine performs
// an immediate check and then repeats on the target's configured interval.
//
// Results are sent to the returned channel as they arrive. The channel is
// buffered to the number of targets to reduce back-pressure from slow readers.
// The channel is closed when all goroutines have exited (i.e. ctx is cancelled).
func (e *Engine) Start(ctx context.Context) <-chan TargetResult {
	out := make(chan TargetResult, len(e.cfg.Targets))

	var wg sync.WaitGroup
	for _, target := range e.cfg.Targets {
		wg.Add(1)
		go func(t config.Target) {
			defer wg.Done()
			e.runTarget(ctx, t, out)
		}(target)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// runTarget performs the check loop for a single target. It checks immediately
// and then on every tick of t.Interval until ctx is cancelled.
func (e *Engine) runTarget(ctx context.Context, t config.Target, out chan<- TargetResult) {
	chk, err := checker.New(t.Type)
	if err != nil {
		// Unknown type — emit a single error result and exit.
		r := checker.Result{
			Status:    checker.StatusDown,
			Error:     err,
			Timestamp: time.Now(),
		}
		out <- e.recordAndBuild(ctx, t, r)
		return
	}

	// First check runs immediately.
	out <- e.recordAndBuild(ctx, t, chk.Check(ctx, t))

	ticker := time.NewTicker(t.Interval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			out <- e.recordAndBuild(ctx, t, chk.Check(ctx, t))
		}
	}
}

// recordAndBuild records the result into the target's tracker, evaluates
// alerts, and returns a TargetResult with a current snapshot.
func (e *Engine) recordAndBuild(ctx context.Context, t config.Target, r checker.Result) TargetResult {
	e.mu.Lock()
	tracker := e.trackers[t.Name]
	e.mu.Unlock()

	tracker.Record(r)
	e.alertMgr.Evaluate(ctx, t, r)

	return TargetResult{
		Target: t,
		Result: r,
		Stats:  tracker.Snapshot(),
	}
}

// RunOnce executes a single check for every target concurrently and returns
// all results after every check has completed.
func (e *Engine) RunOnce(ctx context.Context) []TargetResult {
	results := make([]TargetResult, len(e.cfg.Targets))
	var wg sync.WaitGroup

	for i, target := range e.cfg.Targets {
		wg.Add(1)
		go func(idx int, t config.Target) {
			defer wg.Done()
			chk, err := checker.New(t.Type)
			if err != nil {
				r := checker.Result{
					Status:    checker.StatusDown,
					Error:     err,
					Timestamp: time.Now(),
				}
				results[idx] = e.recordAndBuild(ctx, t, r)
				return
			}
			results[idx] = e.recordAndBuild(ctx, t, chk.Check(ctx, t))
		}(i, target)
	}

	wg.Wait()
	return results
}

// Snapshots returns a map of target name to current statistics snapshot for
// all targets. The map is safe to read after the call returns.
func (e *Engine) Snapshots() map[string]stats.Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make(map[string]stats.Snapshot, len(e.trackers))
	for name, tracker := range e.trackers {
		out[name] = tracker.Snapshot()
	}
	return out
}
