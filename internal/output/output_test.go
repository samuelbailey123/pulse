package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/samuelbailey123/pulse/internal/engine"
	"github.com/samuelbailey123/pulse/internal/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedTime is a stable timestamp used across test cases to keep output deterministic.
var fixedTime = time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC)

// makeResult constructs an engine.TargetResult for use in tests.
func makeResult(
	name, url string,
	status checker.Status,
	latency time.Duration,
	statusCode int,
	uptime float64,
	totalChecks int64,
	certExpiry *time.Time,
	checkErr error,
) engine.TargetResult {
	return engine.TargetResult{
		Target: config.Target{Name: name, URL: url, Type: "http"},
		Result: checker.Result{
			Status:     status,
			Latency:    latency,
			StatusCode: statusCode,
			CertExpiry: certExpiry,
			Error:      checkErr,
			Timestamp:  fixedTime,
		},
		Stats: stats.Snapshot{
			Uptime:      uptime,
			TotalChecks: totalChecks,
		},
	}
}

// stripANSI removes ANSI escape sequences so we can make plain-text assertions
// against coloured output without importing a full ANSI stripping library.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // consume 'm'
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// TestTable_Header verifies that the header row is always printed.
func TestTable_Header(t *testing.T) {
	var buf bytes.Buffer
	Table(nil, &buf)
	plain := stripANSI(buf.String())

	assert.Contains(t, plain, "TARGET")
	assert.Contains(t, plain, "STATUS")
	assert.Contains(t, plain, "LATENCY")
	assert.Contains(t, plain, "STATUS CODE")
	assert.Contains(t, plain, "UPTIME")
}

// TestTable_Rows checks that each result's fields appear correctly in the output.
func TestTable_Rows(t *testing.T) {
	expiry := fixedTime.Add(30 * 24 * time.Hour)

	results := []engine.TargetResult{
		makeResult("My API", "https://api.example.com/health",
			checker.StatusUp, 45*time.Millisecond, 200, 100.0, 5, nil, nil),
		makeResult("Postgres", "db.example.com:5432",
			checker.StatusDown, 0, 0, 85.2, 10, nil, nil),
		makeResult("TLS Check", "https://example.com",
			checker.StatusDegraded, 12*time.Millisecond, 200, 99.0, 3, &expiry, nil),
	}

	var buf bytes.Buffer
	Table(results, &buf)
	plain := stripANSI(buf.String())

	assert.Contains(t, plain, "My API")
	assert.Contains(t, plain, "UP")
	assert.Contains(t, plain, "45ms")
	assert.Contains(t, plain, "200")
	assert.Contains(t, plain, "100.0%")

	assert.Contains(t, plain, "Postgres")
	assert.Contains(t, plain, "DOWN")

	assert.Contains(t, plain, "TLS Check")
	assert.Contains(t, plain, "DEGRADED")
	assert.Contains(t, plain, "99.0%")
}

// TestTable_DashForMissingValues verifies that absent latency, status code, and
// uptime are rendered as "—" rather than zero values.
func TestTable_DashForMissingValues(t *testing.T) {
	results := []engine.TargetResult{
		makeResult("Bad Target", "tcp://db:5432",
			checker.StatusDown, 0, 0, 0.0, 0, nil, nil),
	}

	var buf bytes.Buffer
	Table(results, &buf)
	plain := stripANSI(buf.String())

	// Uptime and status code columns must show a dash when no checks have been recorded.
	assert.Contains(t, plain, "—")
	// "0ms" must not appear — zero latency on a DOWN result should render as dash.
	assert.NotContains(t, plain, "0ms")
}

// TestJSON_ValidArray verifies that JSON output decodes into the expected slice.
func TestJSON_ValidArray(t *testing.T) {
	results := []engine.TargetResult{
		makeResult("My API", "https://api.example.com/health",
			checker.StatusUp, 45*time.Millisecond, 200, 100.0, 5, nil, nil),
		makeResult("Postgres", "db.example.com:5432",
			checker.StatusDown, 0, 0, 80.0, 10, nil, nil),
	}

	var buf bytes.Buffer
	err := JSON(results, &buf)
	require.NoError(t, err)

	var decoded []map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Len(t, decoded, 2)

	api := decoded[0]
	assert.Equal(t, "My API", api["name"])
	assert.Equal(t, "https://api.example.com/health", api["url"])
	assert.Equal(t, "UP", api["status"])
	assert.InDelta(t, 45.0, api["latency_ms"], 0.01)
	assert.InDelta(t, 200.0, api["status_code"], 0.01)
	assert.InDelta(t, 100.0, api["uptime"], 0.01)
	assert.Nil(t, api["error"])
	assert.Nil(t, api["cert_expiry"])
}

// TestJSON_ErrorField verifies that a non-nil error is serialised into the output.
func TestJSON_ErrorField(t *testing.T) {
	results := []engine.TargetResult{
		makeResult("Dead Service", "http://gone",
			checker.StatusDown, 0, 0, 0.0, 1,
			nil, &testError{"connection refused"}),
	}

	var buf bytes.Buffer
	require.NoError(t, JSON(results, &buf))

	var decoded []map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Len(t, decoded, 1)
	assert.Equal(t, "connection refused", decoded[0]["error"])
}

// TestJSON_CertExpiryField verifies that a non-nil CertExpiry is serialised.
func TestJSON_CertExpiryField(t *testing.T) {
	expiry := fixedTime.Add(30 * 24 * time.Hour)
	results := []engine.TargetResult{
		makeResult("TLS Check", "https://example.com",
			checker.StatusDegraded, 10*time.Millisecond, 200, 99.0, 2, &expiry, nil),
	}

	var buf bytes.Buffer
	require.NoError(t, JSON(results, &buf))

	var decoded []map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Len(t, decoded, 1)
	assert.NotNil(t, decoded[0]["cert_expiry"])
}

// TestJSON_OmittedFields verifies that optional fields are omitted when absent.
func TestJSON_OmittedFields(t *testing.T) {
	// A TCP DOWN result: no status code, no cert expiry, no error, no uptime (0 checks).
	results := []engine.TargetResult{
		makeResult("TCP Target", "db:5432",
			checker.StatusDown, 0, 0, 0.0, 0, nil, nil),
	}

	var buf bytes.Buffer
	require.NoError(t, JSON(results, &buf))

	var decoded []map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Len(t, decoded, 1)

	row := decoded[0]
	_, hasCode := row["status_code"]
	assert.False(t, hasCode, "status_code must be absent when zero")

	_, hasUptime := row["uptime"]
	assert.False(t, hasUptime, "uptime must be absent when no checks have been recorded")

	_, hasCert := row["cert_expiry"]
	assert.False(t, hasCert, "cert_expiry must be absent when nil")

	_, hasErr := row["error"]
	assert.False(t, hasErr, "error must be absent when nil")
}

// TestJSON_EmptyResults verifies that an empty result set encodes as an empty array.
func TestJSON_EmptyResults(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, JSON(nil, &buf))

	var decoded []map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Empty(t, decoded)
}

// TestStatusStyle_AllStatuses exercises statusStyle for every known Status value
// to ensure no panics and that a non-zero style is returned.
func TestStatusStyle_AllStatuses(t *testing.T) {
	statuses := []checker.Status{
		checker.StatusUp,
		checker.StatusDown,
		checker.StatusDegraded,
		checker.Status(99), // unknown value — must not panic
	}
	for _, s := range statuses {
		// Just calling Render must not panic.
		result := statusStyle(s).Render(s.String())
		assert.NotEmpty(t, result)
	}
}

// TestJSON_WriterError verifies that a write failure is propagated as an error.
func TestJSON_WriterError(t *testing.T) {
	results := []engine.TargetResult{
		makeResult("My API", "https://api.example.com", checker.StatusUp, 10*time.Millisecond, 200, 100.0, 1, nil, nil),
	}
	err := JSON(results, &failWriter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encode results as JSON")
}

// failWriter is an io.Writer that always returns an error.
type failWriter struct{}

func (f *failWriter) Write(_ []byte) (int, error) {
	return 0, &testError{"write failed"}
}

// testError is a minimal error implementation used in tests.
type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
