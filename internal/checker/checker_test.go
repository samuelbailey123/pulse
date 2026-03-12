package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusUp, "UP"},
		{StatusDown, "DOWN"},
		{StatusDegraded, "DEGRADED"},
		{Status(99), "UNKNOWN"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.status.String())
		})
	}
}

func TestNew_ValidTypes(t *testing.T) {
	types := []string{"http", "tcp", "dns", "tls"}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			c, err := New(typ)
			require.NoError(t, err)
			assert.NotNil(t, c)
		})
	}
}

func TestNew_InvalidType(t *testing.T) {
	c, err := New("grpc")
	assert.Nil(t, c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "grpc")
}
