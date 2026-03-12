package checker

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/samuelbailey123/pulse/internal/config"
)

// TCPChecker performs health checks by opening a TCP connection to the target address.
type TCPChecker struct{}

// Check dials target.URL as a TCP address (host:port) and reports StatusUp if the
// connection succeeds, StatusDown otherwise. The connection is closed immediately
// after a successful dial.
func (c *TCPChecker) Check(ctx context.Context, target config.Target) Result {
	start := time.Now()
	timestamp := start

	timeout := target.Timeout.Duration
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", target.URL)
	latency := time.Since(start)

	if err != nil {
		return Result{
			Status:    StatusDown,
			Latency:   latency,
			Error:     fmt.Errorf("tcp dial %s: %w", target.URL, err),
			Timestamp: timestamp,
		}
	}
	conn.Close()

	return Result{
		Status:    StatusUp,
		Latency:   latency,
		Timestamp: timestamp,
	}
}
