package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoad_Valid(t *testing.T) {
	cfg, err := Load("../../testdata/valid.yaml")
	require.NoError(t, err)
	require.Len(t, cfg.Targets, 2)

	first := cfg.Targets[0]
	assert.Equal(t, "Example API", first.Name)
	assert.Equal(t, "https://httpbin.org/get", first.URL)
	assert.Equal(t, "http", first.Type)
	assert.Equal(t, "GET", first.Method)
	assert.Equal(t, 30*time.Second, first.Interval.Duration)
	assert.Equal(t, 5*time.Second, first.Timeout.Duration)
	assert.Equal(t, 200, first.Expect.Status)

	second := cfg.Targets[1]
	assert.Equal(t, "Example TCP", second.Name)
	assert.Equal(t, "tcp", second.Type)
	assert.Equal(t, 15*time.Second, second.Interval.Duration)
	assert.Equal(t, 3*time.Second, second.Timeout.Duration)

	assert.Equal(t, 30*time.Second, cfg.Defaults.Interval.Duration)
	assert.Equal(t, 5*time.Second, cfg.Defaults.Timeout.Duration)
	assert.Equal(t, "GET", cfg.Defaults.Method)
	assert.Equal(t, 14, cfg.Defaults.TLSWarnDays)
}

func TestLoad_NotFound(t *testing.T) {
	_, err := Load("../../testdata/does_not_exist.yaml")
	require.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.yaml"
	require.NoError(t, os.WriteFile(path, []byte("targets: [{"), 0o600))

	_, err := Load(path)
	require.Error(t, err)
}

func TestValidate_Valid(t *testing.T) {
	cfg, err := Load("../../testdata/valid.yaml")
	require.NoError(t, err)

	errs := Validate(cfg)
	assert.Empty(t, errs)
}

func TestValidate_EmptyTargets(t *testing.T) {
	cfg := &Config{}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "at least one target")
}

func TestValidate_MissingName(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				URL:      "https://example.com",
				Type:     "http",
				Interval: Duration{30 * time.Second},
				Timeout:  Duration{5 * time.Second},
			},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "name is required"), "expected a 'name is required' error")
}

func TestValidate_MissingURL(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:     "no-url",
				Type:     "http",
				Interval: Duration{30 * time.Second},
				Timeout:  Duration{5 * time.Second},
			},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "url is required"), "expected a 'url is required' error")
}

func TestValidate_InvalidType(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:     "bad-type",
				URL:      "https://example.com",
				Type:     "grpc",
				Interval: Duration{30 * time.Second},
				Timeout:  Duration{5 * time.Second},
			},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "unsupported type"), "expected an 'unsupported type' error")
}

func TestValidate_InvalidAlertType(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:     "alert-target",
				URL:      "https://example.com",
				Type:     "http",
				Interval: Duration{30 * time.Second},
				Timeout:  Duration{5 * time.Second},
				Alerts: []Alert{
					{Type: "email", URL: "user@example.com", After: 3},
				},
			},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "unsupported alert type"), "expected an 'unsupported alert type' error")
}

func TestValidate_AlertMissingURL(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:     "t1",
				URL:      "https://example.com",
				Type:     "http",
				Interval: Duration{30 * time.Second},
				Timeout:  Duration{5 * time.Second},
				Alerts: []Alert{
					{Type: "webhook", URL: "", After: 3},
				},
			},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "url is required"), "expected a 'url is required' error on alert")
}

func TestValidate_GlobalAlertInvalid(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:     "t1",
				URL:      "https://example.com",
				Type:     "http",
				Interval: Duration{30 * time.Second},
				Timeout:  Duration{5 * time.Second},
			},
		},
		Alerts: []Alert{
			{Type: "slack", URL: "https://hooks.slack.com/xyz", After: 1},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "unsupported alert type"), "expected 'unsupported alert type' on global alert")
}

func TestValidate_GlobalAlertMissingURL(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:     "t1",
				URL:      "https://example.com",
				Type:     "http",
				Interval: Duration{30 * time.Second},
				Timeout:  Duration{5 * time.Second},
			},
		},
		Alerts: []Alert{
			{Type: "webhook", URL: "", After: 1},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "url is required"), "expected 'url is required' on global alert")
}

func TestValidate_ZeroInterval(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:    "t1",
				URL:     "https://example.com",
				Type:    "http",
				Timeout: Duration{5 * time.Second},
				// Interval deliberately left at zero.
			},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "interval must be greater than zero"))
}

func TestValidate_ZeroTimeout(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:     "t1",
				URL:      "https://example.com",
				Type:     "http",
				Interval: Duration{30 * time.Second},
				// Timeout deliberately left at zero.
			},
		},
	}
	errs := Validate(cfg)
	require.NotEmpty(t, errs)
	assert.True(t, containsError(errs, "timeout must be greater than zero"))
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{Name: "t1", URL: "https://example.com"},
		},
		Defaults: Defaults{
			Interval:    Duration{60 * time.Second},
			Timeout:     Duration{10 * time.Second},
			Method:      "HEAD",
			TLSWarnDays: 30,
		},
	}
	cfg.ApplyDefaults()

	t1 := cfg.Targets[0]
	assert.Equal(t, 60*time.Second, t1.Interval.Duration)
	assert.Equal(t, 10*time.Second, t1.Timeout.Duration)
	assert.Equal(t, "HEAD", t1.Method)
	assert.Equal(t, 30, t1.TLSWarnDays)
	assert.Equal(t, "http", t1.Type)
}

func TestApplyDefaults_BuiltinFallback(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{Name: "t1", URL: "https://example.com"},
		},
	}
	cfg.ApplyDefaults()

	t1 := cfg.Targets[0]
	assert.Equal(t, 30*time.Second, t1.Interval.Duration)
	assert.Equal(t, 5*time.Second, t1.Timeout.Duration)
	assert.Equal(t, "GET", t1.Method)
	assert.Equal(t, 14, t1.TLSWarnDays)
	assert.Equal(t, "http", t1.Type)
}

func TestApplyDefaults_ExistingValuesPreserved(t *testing.T) {
	cfg := &Config{
		Targets: []Target{
			{
				Name:        "t1",
				URL:         "https://example.com",
				Type:        "tls",
				Method:      "POST",
				Interval:    Duration{10 * time.Second},
				Timeout:     Duration{2 * time.Second},
				TLSWarnDays: 7,
			},
		},
	}
	cfg.ApplyDefaults()

	t1 := cfg.Targets[0]
	assert.Equal(t, "tls", t1.Type)
	assert.Equal(t, "POST", t1.Method)
	assert.Equal(t, 10*time.Second, t1.Interval.Duration)
	assert.Equal(t, 2*time.Second, t1.Timeout.Duration)
	assert.Equal(t, 7, t1.TLSWarnDays)
}

func TestDuration_UnmarshalYAML_Valid(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"d: 30s", 30 * time.Second},
		{"d: 5m", 5 * time.Minute},
		{"d: 1h", time.Hour},
		{"d: 500ms", 500 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			type wrapper struct {
				D Duration `yaml:"d"`
			}
			var w wrapper
			require.NoError(t, yaml.Unmarshal([]byte(tc.input), &w))
			assert.Equal(t, tc.expected, w.D.Duration)
		})
	}
}

func TestDuration_UnmarshalYAML_Invalid(t *testing.T) {
	type wrapper struct {
		D Duration `yaml:"d"`
	}
	var w wrapper
	err := yaml.Unmarshal([]byte("d: notaduration"), &w)
	require.Error(t, err)
}

func TestDuration_MarshalYAML(t *testing.T) {
	d := Duration{30 * time.Second}
	v, err := d.MarshalYAML()
	require.NoError(t, err)
	assert.Equal(t, "30s", v)
}

// containsError reports whether any error in errs contains the substring sub.
func containsError(errs []error, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), sub) {
			return true
		}
	}
	return false
}
