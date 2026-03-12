package checker

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/samuelbailey123/pulse/internal/config"
)

// DNSChecker performs health checks by resolving the hostname in target.URL.
type DNSChecker struct{}

// Check extracts the hostname from target.URL, resolves it via the system resolver,
// and returns StatusUp when at least one address is returned, StatusDown otherwise.
func (c *DNSChecker) Check(ctx context.Context, target config.Target) Result {
	start := time.Now()
	timestamp := start

	timeout := target.Timeout.Duration
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	hostname := extractHostname(target.URL)

	resolveCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resolver := &net.Resolver{}
	addrs, err := resolver.LookupHost(resolveCtx, hostname)
	latency := time.Since(start)

	if err != nil {
		return Result{
			Status:    StatusDown,
			Latency:   latency,
			Error:     fmt.Errorf("dns lookup %s: %w", hostname, err),
			Timestamp: timestamp,
		}
	}

	if len(addrs) == 0 {
		return Result{
			Status:    StatusDown,
			Latency:   latency,
			Error:     fmt.Errorf("dns lookup %s: no addresses returned", hostname),
			Timestamp: timestamp,
		}
	}

	return Result{
		Status:    StatusUp,
		Latency:   latency,
		Timestamp: timestamp,
	}
}

// extractHostname returns just the hostname portion of rawURL.
// If rawURL cannot be parsed as a URL the raw value is returned as-is,
// allowing plain hostnames to pass through without wrapping in a scheme.
func extractHostname(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// url.Parse treats "example.com" (no scheme) as a path, not a host.
	if parsed.Host != "" {
		h, _, err := net.SplitHostPort(parsed.Host)
		if err == nil {
			return h
		}
		return parsed.Host
	}
	// No scheme — treat the whole string as a hostname.
	return rawURL
}
