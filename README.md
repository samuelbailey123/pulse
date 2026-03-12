# Pulse

Multi-endpoint health checker with live TUI dashboard. A single binary, zero dependencies, no code required.

Monitor HTTP, TCP, DNS, and TLS endpoints from a simple YAML config. Get response time stats, uptime tracking, and webhook alerts when things go down.

## Install

```bash
go install github.com/samuelbailey123/pulse@latest
```

Or download a binary from the [releases page](https://github.com/samuelbailey123/pulse/releases).

## Quick Start

```bash
# Generate a starter config
pulse init

# Validate the config
pulse validate

# Run a one-shot check
pulse check

# Start live monitoring dashboard
pulse watch
```

## Configuration

```yaml
defaults:
  interval: 30s
  timeout: 5s
  method: GET
  tls_warn_days: 14

targets:
  # HTTP health check
  - name: My API
    url: https://api.example.com/health
    expect:
      status: 200
      body_contains: '"status":"ok"'
    alerts:
      - type: webhook
        url: https://hooks.example.com/alert
        after: 3

  # TCP port check
  - name: Postgres
    url: db.example.com:5432
    type: tcp
    interval: 15s
    timeout: 3s

  # TLS certificate expiry
  - name: TLS Check
    url: https://example.com
    type: tls
    interval: 1h
    tls_warn_days: 30

  # DNS resolution
  - name: DNS Check
    url: api.example.com
    type: dns
    interval: 60s

# Global alerts applied to all targets
alerts:
  - type: webhook
    url: https://hooks.example.com/global-alert
    after: 5
```

### Environment Variables

Use `${VAR_NAME}` anywhere in the config to interpolate environment variables:

```yaml
targets:
  - name: My API
    url: ${API_BASE_URL}/health
    headers:
      Authorization: Bearer ${API_TOKEN}
```

## Commands

| Command | Description |
|---------|-------------|
| `pulse watch` | Start continuous monitoring with a live TUI dashboard |
| `pulse check` | Run a one-shot health check and exit |
| `pulse validate` | Check configuration file for errors |
| `pulse init` | Generate a starter configuration file |
| `pulse version` | Print version information |

### watch

```bash
pulse watch                          # Monitor with default config (pulse.yaml)
pulse watch -c production.yaml       # Custom config file
```

The TUI dashboard shows a live-updating table with status, latency, P95, uptime percentage, and last check time for each target. Press `q` to quit.

### check

```bash
pulse check                          # Table output
pulse check --json                   # JSON output for CI/CD pipelines
pulse check -c staging.yaml          # Custom config file
```

Exits with code 1 if any target is DOWN — useful in CI pipelines and scripts.

### validate

```bash
pulse validate                       # Validate pulse.yaml
pulse validate -c custom.yaml        # Validate a specific file
```

## Features

### Check Types

| Type | URL Format | What It Checks |
|------|-----------|----------------|
| `http` | `https://api.example.com/health` | HTTP response status, body content, TLS cert |
| `tcp` | `host:port` | TCP port is open and accepting connections |
| `dns` | `hostname` | DNS hostname resolves to at least one address |
| `tls` | `https://example.com` | TLS certificate validity and expiry |

### Response Expectations (HTTP)

```yaml
expect:
  status: 200                    # Exact status code (default: any 2xx)
  body_contains: '"status":"ok"' # Substring match on response body
```

### Custom Headers

```yaml
headers:
  Authorization: Bearer ${TOKEN}
  X-Custom-Header: value
```

### Defaults

Fields set in `defaults` apply to every target that doesn't override them:

| Field | Default | Description |
|-------|---------|-------------|
| `interval` | `30s` | Check frequency |
| `timeout` | `5s` | Per-check deadline |
| `method` | `GET` | HTTP method |
| `tls_warn_days` | `14` | Days before cert expiry to warn |

### Alerts

Webhook alerts fire after a configurable number of consecutive failures:

```yaml
alerts:
  - type: webhook
    url: https://hooks.slack.com/services/xxx
    after: 3    # Fire after 3 consecutive failures
```

Alerts can be set per-target or globally. When a target recovers, a recovery notification is sent.

Alert payload:

```json
{
  "target_name": "My API",
  "url": "https://api.example.com/health",
  "status": "DOWN",
  "error": "expected status 200, got 503",
  "latency": "2.5s",
  "timestamp": "2026-03-12T10:30:00Z",
  "consecutive_failures": 3
}
```

### Statistics

Pulse tracks per-target statistics over a sliding window:

- **Min/Avg/Max** response time
- **P95** latency
- **Uptime** percentage
- **Consecutive failures** count

### TLS Certificate Monitoring

TLS checks report three states:

| Status | Condition |
|--------|-----------|
| UP | Certificate valid, not expiring soon |
| DEGRADED | Certificate expires within `tls_warn_days` |
| DOWN | Certificate expired or TLS handshake failed |

## Development

```bash
make build        # Build binary with version info
make test         # Run tests
make test-race    # Run tests with race detector
make coverage     # Generate coverage report
make lint         # Run linters (requires golangci-lint)
make install      # Install to $GOPATH/bin
```

## License

MIT — see [LICENSE](LICENSE).
