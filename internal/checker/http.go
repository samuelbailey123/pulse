package checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/samuelbailey123/pulse/internal/config"
)

// HTTPChecker performs health checks over HTTP or HTTPS.
type HTTPChecker struct{}

// Check executes an HTTP request against target.URL and returns a Result.
//
// Success criteria:
//   - If target.Expect.Status is set, the response must match that exact code.
//   - Otherwise any 2xx status is accepted.
//   - If target.Expect.BodyContains is set, the response body must contain that substring.
//
// For HTTPS targets the TLS certificate expiry is always captured in Result.CertExpiry.
func (c *HTTPChecker) Check(ctx context.Context, target config.Target) Result {
	start := time.Now()
	timestamp := start

	timeout := target.Timeout.Duration
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	method := target.Method
	if method == "" {
		method = http.MethodGet
	}

	// Build a client that records TLS state so we can inspect the certificate.
	var certExpiry *time.Time
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			// InsecureSkipVerify is intentionally false; we want real cert validation.
		},
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, method, target.URL, nil)
	if err != nil {
		return Result{
			Status:    StatusDown,
			Latency:   time.Since(start),
			Error:     fmt.Errorf("building request: %w", err),
			Timestamp: timestamp,
		}
	}

	for k, v := range target.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return Result{
			Status:    StatusDown,
			Latency:   latency,
			Error:     fmt.Errorf("executing request: %w", err),
			Timestamp: timestamp,
		}
	}
	defer resp.Body.Close()

	// Capture TLS cert expiry when the connection is over HTTPS.
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		expiry := resp.TLS.PeerCertificates[0].NotAfter
		certExpiry = &expiry
	}

	// Read body (needed for body-contains check and to allow connection reuse).
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{
			Status:     StatusDown,
			Latency:    latency,
			Error:      fmt.Errorf("reading response body: %w", err),
			StatusCode: resp.StatusCode,
			CertExpiry: certExpiry,
			Timestamp:  timestamp,
		}
	}

	// Evaluate status code expectation.
	expectedStatus := target.Expect.Status
	if expectedStatus != 0 {
		if resp.StatusCode != expectedStatus {
			return Result{
				Status:     StatusDown,
				Latency:    latency,
				Error:      fmt.Errorf("expected status %d, got %d", expectedStatus, resp.StatusCode),
				StatusCode: resp.StatusCode,
				CertExpiry: certExpiry,
				Timestamp:  timestamp,
			}
		}
	} else {
		// Default: any 2xx is acceptable.
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return Result{
				Status:     StatusDown,
				Latency:    latency,
				Error:      fmt.Errorf("non-2xx status %d", resp.StatusCode),
				StatusCode: resp.StatusCode,
				CertExpiry: certExpiry,
				Timestamp:  timestamp,
			}
		}
	}

	// Evaluate body-contains expectation.
	if target.Expect.BodyContains != "" {
		if !strings.Contains(string(body), target.Expect.BodyContains) {
			return Result{
				Status:     StatusDown,
				Latency:    latency,
				Error:      fmt.Errorf("response body does not contain %q", target.Expect.BodyContains),
				StatusCode: resp.StatusCode,
				CertExpiry: certExpiry,
				Timestamp:  timestamp,
			}
		}
	}

	return Result{
		Status:     StatusUp,
		Latency:    latency,
		StatusCode: resp.StatusCode,
		CertExpiry: certExpiry,
		Timestamp:  timestamp,
	}
}
