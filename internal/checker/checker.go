// Package checker provides health-check implementations for HTTP, TCP, DNS, and TLS targets.
package checker

import (
	"context"
	"fmt"
	"time"

	"github.com/samuelbailey123/pulse/internal/config"
)

// Status represents the health state of a checked target.
type Status int

const (
	// StatusUp means the target responded successfully within expectations.
	StatusUp Status = iota
	// StatusDown means the target is unreachable or failed its expectations.
	StatusDown
	// StatusDegraded means the target is reachable but in a warning state (e.g. cert nearing expiry).
	StatusDegraded
)

// String returns the human-readable representation of a Status.
func (s Status) String() string {
	switch s {
	case StatusUp:
		return "UP"
	case StatusDown:
		return "DOWN"
	case StatusDegraded:
		return "DEGRADED"
	default:
		return "UNKNOWN"
	}
}

// Result holds the outcome of a single health check.
type Result struct {
	// Status is the assessed health state.
	Status Status
	// Latency is the elapsed time for the check operation.
	Latency time.Duration
	// Error is non-nil when the check could not be completed or the target failed expectations.
	Error error
	// StatusCode is the HTTP response status code. Zero for non-HTTP checks.
	StatusCode int
	// CertExpiry is the TLS certificate expiry time. Nil for non-TLS checks.
	CertExpiry *time.Time
	// Timestamp records when the check was performed.
	Timestamp time.Time
}

// Checker performs a health check against a single target.
type Checker interface {
	Check(ctx context.Context, target config.Target) Result
}

// New returns the appropriate Checker implementation for the given target type.
// Valid types are "http", "tcp", "dns", and "tls".
func New(targetType string) (Checker, error) {
	switch targetType {
	case "http":
		return &HTTPChecker{}, nil
	case "tcp":
		return &TCPChecker{}, nil
	case "dns":
		return &DNSChecker{}, nil
	case "tls":
		return &TLSChecker{}, nil
	default:
		return nil, fmt.Errorf("unknown checker type %q: must be one of http, tcp, dns, tls", targetType)
	}
}
