package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/samuelbailey123/pulse/internal/engine"
	"github.com/samuelbailey123/pulse/internal/stats"
)

// makeResult is a helper that builds an engine.TargetResult for tests.
func makeResult(name, url string, status checker.Status, latency time.Duration, uptime float64) engine.TargetResult {
	ts := time.Now()
	return engine.TargetResult{
		Target: config.Target{Name: name, URL: url},
		Result: checker.Result{
			Status:    status,
			Latency:   latency,
			Timestamp: ts,
		},
		Stats: stats.Snapshot{
			P95:         latency + 10*time.Millisecond,
			Uptime:      uptime,
			TotalChecks: 10,
		},
	}
}

// closedChannel returns a closed channel so Init/Update can drain it
// immediately without blocking in unit tests.
func closedChannel() <-chan engine.TargetResult {
	ch := make(chan engine.TargetResult)
	close(ch)
	return ch
}

// bufferedChannel returns a channel pre-loaded with results.
func bufferedChannel(results ...engine.TargetResult) <-chan engine.TargetResult {
	ch := make(chan engine.TargetResult, len(results))
	for _, r := range results {
		ch <- r
	}
	return ch
}

// --- NewModel ---

func TestNewModel(t *testing.T) {
	ch := closedChannel()
	m := NewModel(ch, 4)

	assert.Equal(t, 4, m.targetCount)
	assert.NotNil(t, m.rows)
	assert.Empty(t, m.order)
	assert.False(t, m.quitting)
}

// --- Init ---

func TestInit_ReturnsCmd(t *testing.T) {
	ch := closedChannel()
	m := NewModel(ch, 1)
	cmd := m.Init()
	// Init must return a non-nil Cmd.
	require.NotNil(t, cmd)
}

// --- waitForResult ---

func TestWaitForResult_ClosedChannel(t *testing.T) {
	ch := closedChannel()
	cmd := waitForResult(ch)
	msg := cmd()
	// A nil msg is returned when the channel is closed.
	assert.Nil(t, msg)
}

func TestWaitForResult_ReceivesResult(t *testing.T) {
	tr := makeResult("api", "https://api.example.com", checker.StatusUp, 45*time.Millisecond, 100)
	ch := bufferedChannel(tr)

	cmd := waitForResult(ch)
	msg := cmd()

	got, ok := msg.(resultMsg)
	require.True(t, ok, "expected resultMsg")
	assert.Equal(t, "api", got.Target.Name)
}

// --- Update: key handling ---

func TestUpdate_QuitOnQ(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	nextModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	tuiModel := nextModel.(Model)
	assert.True(t, tuiModel.quitting)
	assert.NotNil(t, cmd) // tea.Quit is a non-nil Cmd
}

func TestUpdate_QuitOnCtrlC(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	nextModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	tuiModel := nextModel.(Model)
	assert.True(t, tuiModel.quitting)
	assert.NotNil(t, cmd)
}

func TestUpdate_OtherKeyIgnored(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	nextModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	tuiModel := nextModel.(Model)
	assert.False(t, tuiModel.quitting)
	assert.Nil(t, cmd)
}

// --- Update: resultMsg ---

func TestUpdate_ResultMsg_AddsRow(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	tr := makeResult("svc", "https://svc.example.com", checker.StatusUp, 30*time.Millisecond, 99.0)

	nextModel, cmd := m.Update(resultMsg(tr))

	tuiModel := nextModel.(Model)
	require.Contains(t, tuiModel.rows, "svc")
	r := tuiModel.rows["svc"]
	assert.Equal(t, checker.StatusUp, r.status)
	assert.Equal(t, 30*time.Millisecond, r.latency)
	assert.True(t, r.hasLatency)
	assert.True(t, r.hasStats)
	assert.InDelta(t, 99.0, r.uptime, 0.01)
	assert.NotNil(t, cmd)                   // re-armed channel reader
	assert.Equal(t, []string{"svc"}, tuiModel.order)
}

func TestUpdate_ResultMsg_UpdatesExistingRow(t *testing.T) {
	m := NewModel(closedChannel(), 1)

	// First result establishes the row.
	tr1 := makeResult("svc", "https://svc.example.com", checker.StatusUp, 30*time.Millisecond, 100)
	m2, _ := m.Update(resultMsg(tr1))

	// Second result updates the same target.
	tr2 := makeResult("svc", "https://svc.example.com", checker.StatusDown, 0, 80)
	m3, _ := m2.(Model).Update(resultMsg(tr2))

	tuiModel := m3.(Model)
	// Order slice should still have only one entry.
	assert.Equal(t, []string{"svc"}, tuiModel.order)
	r := tuiModel.rows["svc"]
	assert.Equal(t, checker.StatusDown, r.status)
}

func TestUpdate_ResultMsg_ZeroLatency_HasLatencyFalse(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	tr := makeResult("db", "db.internal:5432", checker.StatusDown, 0, 50)

	nextModel, _ := m.Update(resultMsg(tr))
	tuiModel := nextModel.(Model)
	r := tuiModel.rows["db"]
	assert.False(t, r.hasLatency)
}

func TestUpdate_ResultMsg_ZeroP95_HasP95False(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	// Build a result where P95 == 0 but TotalChecks > 0.
	tr := engine.TargetResult{
		Target: config.Target{Name: "x", URL: "http://x"},
		Result: checker.Result{Status: checker.StatusUp, Latency: 10 * time.Millisecond, Timestamp: time.Now()},
		Stats:  stats.Snapshot{P95: 0, TotalChecks: 1, Uptime: 100},
	}
	nextModel, _ := m.Update(resultMsg(tr))
	tuiModel := nextModel.(Model)
	assert.False(t, tuiModel.rows["x"].hasP95)
}

func TestUpdate_ResultMsg_ZeroTotalChecks_HasStatsFalse(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	tr := engine.TargetResult{
		Target: config.Target{Name: "y", URL: "http://y"},
		Result: checker.Result{Status: checker.StatusUp, Latency: 5 * time.Millisecond, Timestamp: time.Now()},
		Stats:  stats.Snapshot{TotalChecks: 0},
	}
	nextModel, _ := m.Update(resultMsg(tr))
	tuiModel := nextModel.(Model)
	assert.False(t, tuiModel.rows["y"].hasStats)
}

// --- Update: unknown message ---

func TestUpdate_UnknownMsgIgnored(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	nextModel, cmd := m.Update("unexpected string message")
	assert.Equal(t, m, nextModel.(Model))
	assert.Nil(t, cmd)
}

// --- View ---

func TestView_Quitting_ReturnsEmpty(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	m.quitting = true
	assert.Equal(t, "", m.View())
}

func TestView_EmptyRows_ShowsWaiting(t *testing.T) {
	m := NewModel(closedChannel(), 3)
	view := m.View()
	assert.Contains(t, view, "Waiting for check results")
	assert.Contains(t, view, "Monitoring 3 targets")
}

func TestView_WithRows_ShowsTargetData(t *testing.T) {
	m := NewModel(closedChannel(), 2)

	results := []engine.TargetResult{
		makeResult("My API", "https://api.example.com", checker.StatusUp, 45*time.Millisecond, 100),
		makeResult("Postgres", "postgres:5432", checker.StatusDown, 0, 85.2),
	}
	for _, tr := range results {
		var nextModel tea.Model
		nextModel, _ = m.Update(resultMsg(tr))
		m = nextModel.(Model)
	}

	view := m.View()

	assert.Contains(t, view, "My API")
	assert.Contains(t, view, "UP")
	assert.Contains(t, view, "45ms")
	assert.Contains(t, view, "Postgres")
	assert.Contains(t, view, "DOWN")
}

func TestView_DegradedStatus(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	tr := makeResult("TLS", "https://tls.example.com", checker.StatusDegraded, 120*time.Millisecond, 99.1)
	nextModel, _ := m.Update(resultMsg(tr))
	m = nextModel.(Model)

	view := m.View()
	assert.Contains(t, view, "DEGRADED")
}

func TestView_UnknownStatus(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	// Inject a row with a status value outside the known constants.
	m.rows["unknown-svc"] = row{
		name:      "unknown-svc",
		status:    checker.Status(99),
		lastCheck: time.Now(),
		hasStats:  false,
	}
	m.order = append(m.order, "unknown-svc")

	view := m.View()
	assert.Contains(t, view, "unknown-svc")
	assert.Contains(t, view, "UNKNOWN")
}

func TestView_MissingValues_ShowsDash(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	// A DOWN result with zero latency and no stats yields "—" cells.
	tr := engine.TargetResult{
		Target: config.Target{Name: "db", URL: "db:5432"},
		Result: checker.Result{
			Status:    checker.StatusDown,
			Latency:   0,
			Timestamp: time.Now(),
		},
		Stats: stats.Snapshot{TotalChecks: 0},
	}
	nextModel, _ := m.Update(resultMsg(tr))
	m = nextModel.(Model)

	view := m.View()
	// There should be at least two "—" placeholder cells (latency + p95 + uptime).
	dashCount := strings.Count(view, "—")
	assert.GreaterOrEqual(t, dashCount, 2)
}

func TestView_ZeroLastCheck_ShowsDash(t *testing.T) {
	m := NewModel(closedChannel(), 1)
	m.rows["svc"] = row{
		name:      "svc",
		status:    checker.StatusUp,
		hasLatency: false,
		hasP95:    false,
		hasStats:  false,
		lastCheck: time.Time{}, // zero time
	}
	m.order = []string{"svc"}

	view := m.View()
	// At least one "—" should appear for last check.
	assert.Contains(t, view, "—")
}

func TestView_ContainsBorders(t *testing.T) {
	m := NewModel(closedChannel(), 0)
	view := m.View()
	assert.Contains(t, view, "┌")
	assert.Contains(t, view, "┐")
	assert.Contains(t, view, "└")
	assert.Contains(t, view, "┘")
	assert.Contains(t, view, "├")
	assert.Contains(t, view, "┤")
}

func TestView_ContainsHelpText(t *testing.T) {
	m := NewModel(closedChannel(), 0)
	view := m.View()
	assert.Contains(t, view, "q quit")
}

func TestView_ContainsColumnHeaders(t *testing.T) {
	m := NewModel(closedChannel(), 0)
	view := m.View()
	for _, col := range []string{"TARGET", "STATUS", "LATENCY", "P95", "UPTIME", "LAST CHECK"} {
		assert.Contains(t, view, col, "missing column header: %s", col)
	}
}

// TestView_GapClamp exercises the branch where the combined title+count string
// exceeds totalWidth, forcing gap to be clamped to zero rather than going
// negative.
// TestView_GapClamp exercises the branch where the combined title+count string
// exceeds totalWidth, forcing gap to be clamped to zero rather than going
// negative. A 12-digit target count causes the formatted count string to exceed
// totalWidth (63) when added to the byte-length of the title (31), so gap
// evaluates to -1 before clamping.
func TestView_GapClamp(t *testing.T) {
	m := NewModel(closedChannel(), 999999999999)
	view := m.View()
	// The view must still render without panicking, and the border must be present.
	assert.Contains(t, view, "┌")
	assert.Contains(t, view, "Monitoring 999999999999 targets")
}

// --- fmtDuration ---

func TestFmtDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Nanosecond, "0µs"},
		{1 * time.Microsecond, "1µs"},
		{999 * time.Microsecond, "999µs"},
		{1 * time.Millisecond, "1ms"},
		{45 * time.Millisecond, "45ms"},
		{999 * time.Millisecond, "999ms"},
		{1 * time.Second, "1.0s"},
		{2500 * time.Millisecond, "2.5s"},
	}
	for _, tc := range tests {
		t.Run(tc.d.String(), func(t *testing.T) {
			assert.Equal(t, tc.want, fmtDuration(tc.d))
		})
	}
}

// --- fmtAgo ---

func TestFmtAgo(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "just now"},
		{2 * time.Second, "2s ago"},
		{59 * time.Second, "59s ago"},
		{1 * time.Minute, "1m ago"},
		{90 * time.Second, "1m ago"},
		{1 * time.Hour, "1h ago"},
		{48 * time.Hour, "48h ago"},
	}
	for _, tc := range tests {
		t.Run(tc.d.String(), func(t *testing.T) {
			assert.Equal(t, tc.want, fmtAgo(tc.d))
		})
	}
}

// --- pad ---

func TestPad(t *testing.T) {
	assert.Equal(t, "hi   ", pad("hi", 5))
	assert.Equal(t, "hello", pad("hello", 5))
	assert.Equal(t, "toolong", pad("toolong", 5)) // no truncation in pad
}

// --- truncate ---

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hello", truncate("hello", 5))
	assert.Equal(t, "hell", truncate("hello", 4))
	// Unicode: each rune counts as one character.
	assert.Equal(t, "héll", truncate("héllo", 4))
}
