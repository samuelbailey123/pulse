// Package tui provides the live dashboard for Pulse health monitoring.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/engine"
)

// resultMsg carries a single TargetResult from the engine channel into the
// bubbletea update loop.
type resultMsg engine.TargetResult

// waitForResult returns a tea.Cmd that blocks until the next result arrives on
// the channel, then wraps it as a resultMsg. When the channel is closed the
// command returns nil, which is a no-op in bubbletea.
func waitForResult(ch <-chan engine.TargetResult) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return nil
		}
		return resultMsg(r)
	}
}

// row is a denormalised, display-ready record for one monitored target.
type row struct {
	name      string
	status    checker.Status
	latency   time.Duration
	hasLatency bool
	p95       time.Duration
	hasP95    bool
	uptime    float64
	hasStats  bool
	lastCheck time.Time
}

// Model is the bubbletea model for the Pulse dashboard.
type Model struct {
	results     <-chan engine.TargetResult
	targetCount int
	// rows maps target name to its latest display row so that updates for
	// one target never clobber another target's data.
	rows  map[string]row
	// order preserves the first-seen insertion order for stable rendering.
	order []string
	quitting bool
}

// NewModel constructs a Model that reads from the supplied results channel.
// targetCount is used only for the header count display.
func NewModel(results <-chan engine.TargetResult, targetCount int) Model {
	return Model{
		results:     results,
		targetCount: targetCount,
		rows:        make(map[string]row),
	}
}

// Init starts the first channel-read command.
func (m Model) Init() tea.Cmd {
	return waitForResult(m.results)
}

// Update processes incoming messages and re-schedules the channel reader.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case resultMsg:
		tr := engine.TargetResult(msg)
		name := tr.Target.Name

		r := row{
			name:      name,
			status:    tr.Result.Status,
			latency:   tr.Result.Latency,
			hasLatency: tr.Result.Latency > 0,
			p95:       tr.Stats.P95,
			hasP95:    tr.Stats.TotalChecks > 0 && tr.Stats.P95 > 0,
			uptime:    tr.Stats.Uptime,
			hasStats:  tr.Stats.TotalChecks > 0,
			lastCheck: tr.Result.Timestamp,
		}
		if _, seen := m.rows[name]; !seen {
			m.order = append(m.order, name)
		}
		m.rows[name] = r

		// Re-arm the reader for the next result.
		return m, waitForResult(m.results)
	}

	return m, nil
}

// View renders the full dashboard as a string.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	const (
		colName    = 16
		colStatus  = 8
		colLatency = 9
		colP95     = 8
		colUptime  = 8
		colLast    = 12
	)

	totalWidth := colName + colStatus + colLatency + colP95 + colUptime + colLast + 2

	// Styles.
	bold := lipgloss.NewStyle().Bold(true)
	up := lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	down := lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	degraded := lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	dim := lipgloss.NewStyle().Faint(true)

	border := strings.Repeat("─", totalWidth)

	var sb strings.Builder

	// Top border.
	sb.WriteString("┌" + border + "┐\n")

	// Title bar.
	title := fmt.Sprintf("  Pulse — Health Dashboard")
	count := fmt.Sprintf("Monitoring %d targets  ", m.targetCount)
	gap := totalWidth - len(title) - len(count)
	if gap < 0 {
		gap = 0
	}
	sb.WriteString("│" + title + strings.Repeat(" ", gap) + count + "│\n")

	// Section divider.
	sb.WriteString("├" + border + "┤\n")

	// Header row.
	header := fmt.Sprintf("  %-*s%-*s%-*s%-*s%-*s%-*s",
		colName, "TARGET",
		colStatus, "STATUS",
		colLatency, "LATENCY",
		colP95, "P95",
		colUptime, "UPTIME",
		colLast, "LAST CHECK",
	)
	sb.WriteString("│" + bold.Render(header) + "│\n")

	// Data rows.
	for _, name := range m.order {
		r := m.rows[name]

		// Status cell with colour.
		statusStr := r.status.String()
		var statusStyled string
		switch r.status {
		case checker.StatusUp:
			statusStyled = up.Render(pad(statusStr, colStatus))
		case checker.StatusDown:
			statusStyled = down.Render(pad(statusStr, colStatus))
		case checker.StatusDegraded:
			statusStyled = degraded.Render(pad(statusStr, colStatus))
		default:
			statusStyled = pad(statusStr, colStatus)
		}

		// Latency cell.
		var latencyStr string
		if r.hasLatency {
			latencyStr = fmtDuration(r.latency)
		} else {
			latencyStr = "—"
		}

		// P95 cell.
		var p95Str string
		if r.hasP95 {
			p95Str = fmtDuration(r.p95)
		} else {
			p95Str = "—"
		}

		// Uptime cell.
		var uptimeStr string
		if r.hasStats {
			uptimeStr = fmt.Sprintf("%.1f%%", r.uptime)
		} else {
			uptimeStr = "—"
		}

		// Last check cell.
		var lastStr string
		if !r.lastCheck.IsZero() {
			lastStr = fmtAgo(time.Since(r.lastCheck))
		} else {
			lastStr = "—"
		}

		line := fmt.Sprintf("  %-*s%s%-*s%-*s%-*s%-*s",
			colName, truncate(name, colName-2),
			statusStyled,
			colLatency, latencyStr,
			colP95, p95Str,
			colUptime, uptimeStr,
			colLast, lastStr,
		)
		sb.WriteString("│" + line + "│\n")
	}

	// Show a placeholder row while waiting for the first results.
	if len(m.order) == 0 {
		waiting := dim.Render("  Waiting for check results...")
		sb.WriteString("│" + waiting + strings.Repeat(" ", totalWidth-len("  Waiting for check results...")) + "│\n")
	}

	// Footer divider.
	sb.WriteString("├" + border + "┤\n")

	// Help line.
	help := "  q quit"
	sb.WriteString("│" + help + strings.Repeat(" ", totalWidth-len(help)) + "│\n")

	// Bottom border.
	sb.WriteString("└" + border + "┘\n")

	return sb.String()
}

// fmtDuration renders a duration as a compact human-readable string such as
// "45ms", "1.2s", suitable for table cells.
func fmtDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
}

// fmtAgo renders an elapsed duration as "Xs ago", "Xm ago", etc.
func fmtAgo(d time.Duration) string {
	switch {
	case d < time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// pad right-pads s with spaces to exactly width characters. If s is longer
// than width it is left as-is — the caller should truncate first.
func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// truncate shortens s to at most maxLen runes.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
