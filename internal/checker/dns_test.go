package checker

import (
	"context"
	"testing"

	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestDNSChecker_Success(t *testing.T) {
	// "localhost" is guaranteed to resolve on any well-configured system.
	c := &DNSChecker{}
	result := c.Check(context.Background(), config.Target{
		URL: "localhost",
	})

	assert.Equal(t, StatusUp, result.Status)
	assert.NoError(t, result.Error)
	assert.Positive(t, result.Latency)
	assert.False(t, result.Timestamp.IsZero())
}

func TestDNSChecker_Failure(t *testing.T) {
	// The .invalid TLD is reserved by RFC 2606 and must never resolve.
	c := &DNSChecker{}
	result := c.Check(context.Background(), config.Target{
		URL: "this.does.not.exist.invalid.",
	})

	assert.Equal(t, StatusDown, result.Status)
	assert.Error(t, result.Error)
}

func TestDNSChecker_URLWithScheme(t *testing.T) {
	// When a full URL is given the checker must extract just the hostname.
	c := &DNSChecker{}
	result := c.Check(context.Background(), config.Target{
		URL: "http://localhost",
	})

	assert.Equal(t, StatusUp, result.Status)
	assert.NoError(t, result.Error)
}

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bare hostname", "localhost", "localhost"},
		{"http URL", "http://example.com", "example.com"},
		{"https URL with port", "https://example.com:443", "example.com"},
		{"http URL with path", "http://example.com/health", "example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractHostname(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}
