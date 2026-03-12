package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterpolate_ReplacesEnvVar(t *testing.T) {
	t.Setenv("PULSE_TEST_HOST", "https://example.com")

	input := []byte("url: ${PULSE_TEST_HOST}/health")
	got := Interpolate(input)
	assert.Equal(t, []byte("url: https://example.com/health"), got)
}

func TestInterpolate_MissingVar(t *testing.T) {
	// Ensure the variable is definitely unset.
	t.Setenv("PULSE_UNSET_VAR_XYZ", "")

	input := []byte("url: ${PULSE_UNSET_VAR_XYZ}/path")
	got := Interpolate(input)
	// Unset vars become empty string, collapsing to just the suffix.
	assert.Equal(t, []byte("url: /path"), got)
}

func TestInterpolate_NoVars(t *testing.T) {
	input := []byte("url: https://example.com")
	got := Interpolate(input)
	assert.Equal(t, input, got)
}

func TestInterpolate_MultipleVars(t *testing.T) {
	t.Setenv("PULSE_SCHEME", "https")
	t.Setenv("PULSE_HOST", "api.example.com")
	t.Setenv("PULSE_PATH", "/v1/health")

	input := []byte("url: ${PULSE_SCHEME}://${PULSE_HOST}${PULSE_PATH}")
	got := Interpolate(input)
	assert.Equal(t, []byte("url: https://api.example.com/v1/health"), got)
}

func TestInterpolate_DoesNotMutateInput(t *testing.T) {
	t.Setenv("PULSE_VAR", "replaced")

	original := []byte("value: ${PULSE_VAR}")
	input := make([]byte, len(original))
	copy(input, original)

	_ = Interpolate(input)
	assert.Equal(t, original, input, "Interpolate must not modify the input slice")
}
