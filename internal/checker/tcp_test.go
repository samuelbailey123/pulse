package checker

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPChecker_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	// Accept connections in the background so the dial can complete.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	c := &TCPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL: ln.Addr().String(),
	})

	assert.Equal(t, StatusUp, result.Status)
	assert.NoError(t, result.Error)
	assert.Positive(t, result.Latency)
	assert.False(t, result.Timestamp.IsZero())
}

func TestTCPChecker_Refused(t *testing.T) {
	// Bind to an ephemeral port then immediately close — guarantees a refused connection.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()

	c := &TCPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:     addr,
		Timeout: config.Duration{Duration: 2 * time.Second},
	})

	assert.Equal(t, StatusDown, result.Status)
	assert.Error(t, result.Error)
}

func TestTCPChecker_Timeout(t *testing.T) {
	// Listener that accepts the connection but never responds — simulates a hanging service.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	// Accept the connection but block indefinitely so the transport hangs.
	// The TCPChecker only dials; it succeeds as soon as the OS-level accept happens.
	// To force a timeout we instead use an address that is routed but not listening
	// (TCP SYN sent, no SYN-ACK) — 192.0.2.1 (TEST-NET-1) is guaranteed non-routable.
	c := &TCPChecker{}
	result := c.Check(context.Background(), config.Target{
		URL:     "192.0.2.1:9999",
		Timeout: config.Duration{Duration: 100 * time.Millisecond},
	})

	assert.Equal(t, StatusDown, result.Status)
	assert.Error(t, result.Error)
}
