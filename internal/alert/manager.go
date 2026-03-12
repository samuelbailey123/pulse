package alert

import (
	"context"
	"sync"

	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/config"
)

// targetState tracks alert state for a single monitored target.
type targetState struct {
	// consecFails is the running count of consecutive Down results.
	consecFails int
	// wasDown is true after an alert has fired for a down transition and
	// before a recovery alert has been sent.
	wasDown bool
}

// Manager evaluates check results against alert rules and fires notifications
// when targets transition between up and down states.
type Manager struct {
	globalAlerts []config.Alert
	alerters     map[string]Alerter // keyed by webhook URL
	states       map[string]*targetState
	mu           sync.Mutex
}

// NewManager constructs a Manager that applies globalAlerts to every target.
func NewManager(globalAlerts []config.Alert) *Manager {
	return &Manager{
		globalAlerts: globalAlerts,
		alerters:     make(map[string]Alerter),
		states:       make(map[string]*targetState),
	}
}

// alerterFor returns (creating if necessary) the Alerter for a given URL.
// Currently only webhook alerters are supported.
func (m *Manager) alerterFor(url string) Alerter {
	if a, ok := m.alerters[url]; ok {
		return a
	}
	a := NewWebhook(url)
	m.alerters[url] = a
	return a
}

// stateFor returns (creating if necessary) the targetState for a target name.
func (m *Manager) stateFor(name string) *targetState {
	if s, ok := m.states[name]; ok {
		return s
	}
	s := &targetState{}
	m.states[name] = s
	return s
}

// Evaluate updates the failure counter for the named target and fires
// alert notifications on state transitions.
//
// Down transition: fires when consecFails first reaches an alert's "after"
// threshold and wasDown is false. Sets wasDown=true.
//
// Recovery transition: fires when the result is Up and wasDown is true.
// Resets wasDown and consecFails.
//
// Both target-level alerts and global alerts are evaluated.
func (m *Manager) Evaluate(ctx context.Context, target config.Target, result checker.Result) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.stateFor(target.Name)

	if result.Status == checker.StatusDown {
		state.consecFails++
	} else {
		// Up or Degraded — treat as recovery if we were previously down.
		if state.wasDown {
			payload := AlertPayload{
				TargetName: target.Name,
				URL:        target.URL,
				Status:     "RECOVERY",
				Latency:    result.Latency.String(),
				Timestamp:  result.Timestamp,
				ConsecFail: 0,
			}
			m.dispatch(ctx, target, payload)
			state.wasDown = false
		}
		state.consecFails = 0
		return
	}

	// Check whether any alert threshold has just been crossed.
	allAlerts := append(target.Alerts, m.globalAlerts...) //nolint:gocritic
	fired := false
	for _, a := range allAlerts {
		if a.After > 0 && state.consecFails == a.After && !state.wasDown {
			fired = true
			break
		}
	}

	if fired {
		errStr := ""
		if result.Error != nil {
			errStr = result.Error.Error()
		}
		payload := AlertPayload{
			TargetName: target.Name,
			URL:        target.URL,
			Status:     "DOWN",
			Error:      errStr,
			Latency:    result.Latency.String(),
			Timestamp:  result.Timestamp,
			ConsecFail: state.consecFails,
		}
		m.dispatch(ctx, target, payload)
		state.wasDown = true
	}
}

// dispatch sends payload to all alert channels defined on the target and
// globally. Errors from individual alerters are silently ignored to prevent
// one broken channel from blocking others.
func (m *Manager) dispatch(ctx context.Context, target config.Target, payload AlertPayload) {
	allAlerts := append(target.Alerts, m.globalAlerts...) //nolint:gocritic
	seen := make(map[string]struct{})
	for _, a := range allAlerts {
		if _, dup := seen[a.URL]; dup {
			continue
		}
		seen[a.URL] = struct{}{}
		alerter := m.alerterFor(a.URL)
		// Fire and forget — the caller holds the lock, so we don't block on network I/O.
		go func(al Alerter, p AlertPayload) {
			_ = al.Send(ctx, p)
		}(alerter, payload)
	}
}
