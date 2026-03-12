package checker

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateSelfSignedCert creates an in-memory TLS certificate valid between notBefore and notAfter.
func generateSelfSignedCert(t *testing.T, notBefore, notAfter time.Time) tls.Certificate {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "pulse-test"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return cert
}

// startTLSServer starts a raw TLS listener using cert and returns the address.
// Accepted connections complete the TLS handshake then close immediately.
func startTLSServer(t *testing.T, cert tls.Certificate) string {
	t.Helper()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	require.NoError(t, err)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.(*tls.Conn).Handshake()
			conn.Close()
		}
	}()

	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

// skipVerifyDialFn returns a tlsDialFunc that skips certificate verification.
// This is intentional in tests to allow self-signed / expired certs to be inspected.
func skipVerifyDialFn(timeout time.Duration) tlsDialFunc {
	return func(ctx context.Context, addr string, _ *tls.Config) (net.Conn, error) {
		d := &tls.Dialer{
			NetDialer: &net.Dialer{Timeout: timeout},
			Config:    &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test-only
		}
		return d.DialContext(ctx, "tcp", addr)
	}
}

func TestTLSChecker_ExpiredCert(t *testing.T) {
	now := time.Now()
	cert := generateSelfSignedCert(t, now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	addr := startTLSServer(t, cert)

	c := &TLSChecker{dialFn: skipVerifyDialFn(5 * time.Second)}
	result := c.Check(context.Background(), config.Target{
		URL:         addr,
		TLSWarnDays: 30,
	})

	assert.Equal(t, StatusDown, result.Status, "expired cert must report DOWN")
	require.NotNil(t, result.CertExpiry)
	assert.True(t, result.CertExpiry.Before(time.Now()))
	assert.Error(t, result.Error)
}

func TestTLSChecker_DegradedCert(t *testing.T) {
	// Certificate expires in 5 days — within the 30-day warn window.
	now := time.Now()
	cert := generateSelfSignedCert(t, now.Add(-24*time.Hour), now.Add(5*24*time.Hour))
	addr := startTLSServer(t, cert)

	c := &TLSChecker{dialFn: skipVerifyDialFn(5 * time.Second)}
	result := c.Check(context.Background(), config.Target{
		URL:         addr,
		TLSWarnDays: 30,
	})

	assert.Equal(t, StatusDegraded, result.Status, "near-expiry cert must report DEGRADED")
	require.NotNil(t, result.CertExpiry)
	assert.Error(t, result.Error)
}

func TestTLSChecker_ValidCert(t *testing.T) {
	// httptest.NewTLSServer generates a cert valid for a long duration.
	srv := httptest.NewTLSServer(nil)
	defer srv.Close()

	// Build a pool that trusts only the test server's certificate.
	leaf, err := x509.ParseCertificate(srv.TLS.Certificates[0].Certificate[0])
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	// Inject a dialer that trusts our custom pool.
	trustedDialFn := func(ctx context.Context, addr string, _ *tls.Config) (net.Conn, error) {
		d := &tls.Dialer{
			NetDialer: &net.Dialer{Timeout: 5 * time.Second},
			Config:    &tls.Config{RootCAs: pool},
		}
		return d.DialContext(ctx, "tcp", addr)
	}

	c := &TLSChecker{dialFn: trustedDialFn}
	result := c.Check(context.Background(), config.Target{
		URL:         srv.Listener.Addr().String(),
		TLSWarnDays: 1, // 1-day warn window — long-lived test cert should still be UP
	})

	assert.Equal(t, StatusUp, result.Status, "valid long-lived cert must report UP")
	assert.NoError(t, result.Error)
	require.NotNil(t, result.CertExpiry)
	assert.True(t, result.CertExpiry.After(time.Now()))
}

func TestTLSChecker_DialFailure(t *testing.T) {
	// Point at a port that refuses connections.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()

	c := &TLSChecker{dialFn: skipVerifyDialFn(2 * time.Second)}
	result := c.Check(context.Background(), config.Target{
		URL: addr,
	})

	assert.Equal(t, StatusDown, result.Status)
	assert.Error(t, result.Error)
}

func TestTLSAddress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"https with port", "https://example.com:8443", "example.com:8443"},
		{"https no port", "https://example.com", "example.com:443"},
		{"bare host", "example.com", "example.com:443"},
		{"host:port", "example.com:8443", "example.com:8443"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tlsAddress(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}
