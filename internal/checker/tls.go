package checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/samuelbailey123/pulse/internal/config"
)

// tlsDialFunc is the signature used by TLSChecker when opening a TLS connection.
// Tests inject a custom implementation to use self-signed or expired certificates.
type tlsDialFunc func(ctx context.Context, addr string, cfg *tls.Config) (net.Conn, error)

// defaultTLSDial wraps tls.Dialer for use as a tlsDialFunc.
func defaultTLSDial(timeout time.Duration) tlsDialFunc {
	return func(ctx context.Context, addr string, cfg *tls.Config) (net.Conn, error) {
		d := &tls.Dialer{
			NetDialer: &net.Dialer{Timeout: timeout},
			Config:    cfg,
		}
		return d.DialContext(ctx, "tcp", addr)
	}
}

// TLSChecker performs health checks by establishing a TLS connection and inspecting
// the server certificate's expiry date.
//
// The zero value is ready to use and connects with standard TLS verification.
// To override the dialer (e.g. in tests), set dialFn before calling Check.
type TLSChecker struct {
	// dialFn is called to open the TLS connection. When nil the default dialer is used.
	dialFn tlsDialFunc
}

// Check dials target.URL over TLS, retrieves the leaf certificate, and returns:
//   - StatusDown     when the certificate has already expired or the dial fails.
//   - StatusDegraded when the certificate expires within target.TLSWarnDays days.
//   - StatusUp       otherwise.
//
// The default port is 443 when target.URL does not specify one.
// The default warn threshold is 30 days when target.TLSWarnDays is zero.
func (c *TLSChecker) Check(ctx context.Context, target config.Target) Result {
	start := time.Now()
	timestamp := start

	timeout := target.Timeout.Duration
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	addr := tlsAddress(target.URL)

	dial := c.dialFn
	if dial == nil {
		dial = defaultTLSDial(timeout)
	}

	tlsCfg := &tls.Config{}
	conn, err := dial(ctx, addr, tlsCfg)
	latency := time.Since(start)

	if err != nil {
		return Result{
			Status:    StatusDown,
			Latency:   latency,
			Error:     fmt.Errorf("tls dial %s: %w", addr, err),
			Timestamp: timestamp,
		}
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return Result{
			Status:    StatusDown,
			Latency:   latency,
			Error:     fmt.Errorf("unexpected connection type from tls dial"),
			Timestamp: timestamp,
		}
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Result{
			Status:    StatusDown,
			Latency:   latency,
			Error:     fmt.Errorf("no peer certificates received from %s", addr),
			Timestamp: timestamp,
		}
	}

	expiry := certs[0].NotAfter
	now := time.Now()

	warnDays := target.TLSWarnDays
	if warnDays == 0 {
		warnDays = 30
	}

	var status Status
	var checkErr error

	switch {
	case now.After(expiry):
		status = StatusDown
		checkErr = fmt.Errorf("tls certificate expired on %s", expiry.Format(time.RFC3339))
	case now.Add(time.Duration(warnDays) * 24 * time.Hour).After(expiry):
		status = StatusDegraded
		checkErr = fmt.Errorf("tls certificate expires soon: %s", expiry.Format(time.RFC3339))
	default:
		status = StatusUp
	}

	return Result{
		Status:     status,
		Latency:    latency,
		Error:      checkErr,
		CertExpiry: &expiry,
		Timestamp:  timestamp,
	}
}

// tlsAddress derives a host:port address from rawURL.
// It defaults to port 443 when no port is present.
func tlsAddress(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	host := parsed.Hostname()
	port := parsed.Port()

	if host == "" {
		// No scheme present — treat the raw value as host or host:port.
		h, p, err := net.SplitHostPort(rawURL)
		if err == nil {
			return net.JoinHostPort(h, p)
		}
		return net.JoinHostPort(rawURL, "443")
	}

	if port == "" {
		port = "443"
	}

	return net.JoinHostPort(host, port)
}
