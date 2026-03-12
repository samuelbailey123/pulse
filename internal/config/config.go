// Package config handles loading, interpolating, and validating Pulse configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration to support YAML unmarshalling from human-readable
// strings such as "30s" or "5m".
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses a YAML string node as a time.Duration.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	d.Duration = parsed
	return nil
}

// MarshalYAML formats the duration as a human-readable string (e.g. "30s").
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
}

// Expectation defines what a successful response looks like for an HTTP target.
type Expectation struct {
	// Status is the expected HTTP status code (e.g. 200).
	Status int `yaml:"status"`
	// BodyContains is a substring that must appear in the response body.
	BodyContains string `yaml:"body_contains"`
}

// Alert defines a notification channel to trigger after consecutive failures.
type Alert struct {
	// Type is the alert mechanism. Currently only "webhook" is supported.
	Type string `yaml:"type"`
	// URL is the destination for the alert payload.
	URL string `yaml:"url"`
	// After is the number of consecutive failures before the alert fires.
	After int `yaml:"after"`
}

// Target represents a single monitored endpoint.
type Target struct {
	// Name is a human-readable label for the target.
	Name string `yaml:"name"`
	// URL is the endpoint address. For HTTP/TLS: full URL. For TCP: host:port. For DNS: hostname.
	URL string `yaml:"url"`
	// Type is the probe protocol: "http", "tcp", "dns", or "tls". Defaults to "http".
	Type string `yaml:"type"`
	// Method is the HTTP request method (HTTP targets only). Defaults to "GET".
	Method string `yaml:"method"`
	// Interval is how frequently the target is checked.
	Interval Duration `yaml:"interval"`
	// Timeout is the maximum time allowed for a single check.
	Timeout Duration `yaml:"timeout"`
	// Headers are additional HTTP request headers (HTTP targets only).
	Headers map[string]string `yaml:"headers"`
	// Expect defines the success criteria for HTTP targets.
	Expect Expectation `yaml:"expect"`
	// Alerts is a list of notification channels specific to this target.
	Alerts []Alert `yaml:"alerts"`
	// TLSWarnDays is the number of days before TLS certificate expiry to warn.
	TLSWarnDays int `yaml:"tls_warn_days"`
}

// Defaults provides fallback values applied to every target that omits them.
type Defaults struct {
	// Interval is the default check frequency.
	Interval Duration `yaml:"interval"`
	// Timeout is the default per-check deadline.
	Timeout Duration `yaml:"timeout"`
	// Method is the default HTTP method.
	Method string `yaml:"method"`
	// TLSWarnDays is the default number of days before expiry to warn.
	TLSWarnDays int `yaml:"tls_warn_days"`
}

// Config is the top-level structure parsed from a Pulse YAML configuration file.
type Config struct {
	// Targets is the list of endpoints to monitor.
	Targets []Target `yaml:"targets"`
	// Defaults provides fallback values for all targets.
	Defaults Defaults `yaml:"defaults"`
	// Alerts is a list of global notification channels applied to every target.
	Alerts []Alert `yaml:"alerts"`
}

// Load reads the YAML configuration file at path, interpolates environment
// variables, unmarshals the result, applies defaults, and returns the config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	data = Interpolate(data)

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	cfg.ApplyDefaults()
	return &cfg, nil
}

// ApplyDefaults fills zero-value fields on every target using the config's
// Defaults section. If the Defaults section itself is zero, built-in defaults
// are used: interval=30s, timeout=5s, method=GET, tls_warn_days=14, type=http.
func (c *Config) ApplyDefaults() {
	const (
		builtinInterval    = 30 * time.Second
		builtinTimeout     = 5 * time.Second
		builtinMethod      = "GET"
		builtinTLSWarnDays = 14
		builtinType        = "http"
	)

	// Resolve effective defaults: prefer the config Defaults, then built-ins.
	effectiveInterval := c.Defaults.Interval.Duration
	if effectiveInterval == 0 {
		effectiveInterval = builtinInterval
	}

	effectiveTimeout := c.Defaults.Timeout.Duration
	if effectiveTimeout == 0 {
		effectiveTimeout = builtinTimeout
	}

	effectiveMethod := c.Defaults.Method
	if effectiveMethod == "" {
		effectiveMethod = builtinMethod
	}

	effectiveTLSWarnDays := c.Defaults.TLSWarnDays
	if effectiveTLSWarnDays == 0 {
		effectiveTLSWarnDays = builtinTLSWarnDays
	}

	for i := range c.Targets {
		t := &c.Targets[i]

		if t.Interval.Duration == 0 {
			t.Interval.Duration = effectiveInterval
		}
		if t.Timeout.Duration == 0 {
			t.Timeout.Duration = effectiveTimeout
		}
		if t.Method == "" {
			t.Method = effectiveMethod
		}
		if t.TLSWarnDays == 0 {
			t.TLSWarnDays = effectiveTLSWarnDays
		}
		if t.Type == "" {
			t.Type = builtinType
		}
	}
}

// validTargetTypes is the set of recognised probe protocols.
var validTargetTypes = map[string]bool{
	"http": true,
	"tcp":  true,
	"dns":  true,
	"tls":  true,
}

// Validate checks the config for logical correctness and returns all
// validation errors found. An empty slice means the config is valid.
func Validate(c *Config) []error {
	var errs []error

	if len(c.Targets) == 0 {
		errs = append(errs, fmt.Errorf("config must define at least one target"))
	}

	for i, t := range c.Targets {
		prefix := fmt.Sprintf("target[%d]", i)

		if t.Name == "" {
			errs = append(errs, fmt.Errorf("%s: name is required", prefix))
		} else {
			prefix = fmt.Sprintf("target %q", t.Name)
		}

		if t.URL == "" {
			errs = append(errs, fmt.Errorf("%s: url is required", prefix))
		}

		if t.Type != "" && !validTargetTypes[t.Type] {
			errs = append(errs, fmt.Errorf("%s: unsupported type %q (must be http, tcp, dns, or tls)", prefix, t.Type))
		}

		if t.Interval.Duration <= 0 {
			errs = append(errs, fmt.Errorf("%s: interval must be greater than zero", prefix))
		}

		if t.Timeout.Duration <= 0 {
			errs = append(errs, fmt.Errorf("%s: timeout must be greater than zero", prefix))
		}

		for j, a := range t.Alerts {
			errs = append(errs, validateAlert(a, fmt.Sprintf("%s alert[%d]", prefix, j))...)
		}
	}

	for i, a := range c.Alerts {
		errs = append(errs, validateAlert(a, fmt.Sprintf("global alert[%d]", i))...)
	}

	return errs
}

// validateAlert checks a single Alert value and returns any errors found.
func validateAlert(a Alert, prefix string) []error {
	var errs []error
	if a.Type != "webhook" {
		errs = append(errs, fmt.Errorf("%s: unsupported alert type %q (must be webhook)", prefix, a.Type))
	}
	if a.URL == "" {
		errs = append(errs, fmt.Errorf("%s: url is required", prefix))
	}
	return errs
}
