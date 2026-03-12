// Package alert provides interfaces and implementations for sending Pulse notifications.
package alert

import (
	"context"
	"time"
)

// Alerter sends an alert notification for a target state change.
type Alerter interface {
	Send(ctx context.Context, payload AlertPayload) error
}

// AlertPayload is the structured notification body sent when a target
// transitions between up and down states.
type AlertPayload struct {
	// TargetName is the human-readable label of the monitored target.
	TargetName string `json:"target_name"`
	// URL is the address of the monitored target.
	URL string `json:"url"`
	// Status is the current state string: "DOWN" or "RECOVERY".
	Status string `json:"status"`
	// Error is a human-readable description of the failure. Empty on recovery.
	Error string `json:"error,omitempty"`
	// Latency is the formatted last observed latency (e.g. "120ms").
	Latency string `json:"latency"`
	// Timestamp is the time of the triggering check result.
	Timestamp time.Time `json:"timestamp"`
	// ConsecFail is the number of consecutive failures that triggered the alert.
	ConsecFail int `json:"consecutive_failures"`
}
