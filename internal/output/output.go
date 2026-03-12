// Package output provides formatters for rendering health check results to the terminal.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/engine"
)

// column widths for the table layout.
const (
	colTarget     = -20 // left-aligned, 20 chars
	colStatus     = -10
	colLatency    = -10
	colStatusCode = -14
	colUptime     = -8
)

var (
	styleUp       = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)   // bright green
	styleDown     = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)    // bright red
	styleDegraded = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)   // bright yellow
	styleHeader   = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)   // white + bold
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))               // dark grey
)

// statusStyle returns the lipgloss style that corresponds to the given status.
func statusStyle(s checker.Status) lipgloss.Style {
	switch s {
	case checker.StatusUp:
		return styleUp
	case checker.StatusDown:
		return styleDown
	case checker.StatusDegraded:
		return styleDegraded
	default:
		return styleDim
	}
}

// Table writes a coloured, human-readable table of results to w.
// Each row represents one target with its current status, latency, HTTP status
// code, and uptime percentage. Absent values are rendered as a dash.
func Table(results []engine.TargetResult, w io.Writer) {
	header := fmt.Sprintf("%-20s %-10s %-10s %-14s %-8s",
		styleHeader.Render("TARGET"),
		styleHeader.Render("STATUS"),
		styleHeader.Render("LATENCY"),
		styleHeader.Render("STATUS CODE"),
		styleHeader.Render("UPTIME"),
	)
	fmt.Fprintln(w, header)

	for _, tr := range results {
		r := tr.Result
		st := tr.Stats

		status := statusStyle(r.Status).Render(r.Status.String())

		latency := styleDim.Render("—")
		if r.Status != checker.StatusDown && r.Latency > 0 {
			latency = fmt.Sprintf("%dms", r.Latency.Milliseconds())
		}

		code := styleDim.Render("—")
		if r.StatusCode != 0 {
			code = fmt.Sprintf("%d", r.StatusCode)
		}

		uptime := styleDim.Render("—")
		if st.TotalChecks > 0 {
			uptime = fmt.Sprintf("%.1f%%", st.Uptime)
		}

		fmt.Fprintf(w, "%-20s %-10s %-10s %-14s %-8s\n",
			tr.Target.Name,
			status,
			latency,
			code,
			uptime,
		)
	}
}

// jsonResult is the shape of each element in the JSON output array.
type jsonResult struct {
	Name       string     `json:"name"`
	URL        string     `json:"url"`
	Status     string     `json:"status"`
	LatencyMs  int64      `json:"latency_ms"`
	StatusCode *int       `json:"status_code,omitempty"`
	Uptime     *float64   `json:"uptime,omitempty"`
	CertExpiry *time.Time `json:"cert_expiry,omitempty"`
	Error      *string    `json:"error,omitempty"`
	Timestamp  time.Time  `json:"timestamp"`
}

// JSON encodes results as a JSON array and writes it to w.
// Fields that are absent for a particular result type (e.g. status_code for a
// TCP check, cert_expiry for an HTTP check) are omitted from the output.
func JSON(results []engine.TargetResult, w io.Writer) error {
	out := make([]jsonResult, 0, len(results))

	for _, tr := range results {
		r := tr.Result
		st := tr.Stats

		jr := jsonResult{
			Name:      tr.Target.Name,
			URL:       tr.Target.URL,
			Status:    r.Status.String(),
			LatencyMs: r.Latency.Milliseconds(),
			Timestamp: r.Timestamp,
		}

		if r.StatusCode != 0 {
			code := r.StatusCode
			jr.StatusCode = &code
		}

		if st.TotalChecks > 0 {
			uptime := st.Uptime
			jr.Uptime = &uptime
		}

		if r.CertExpiry != nil {
			jr.CertExpiry = r.CertExpiry
		}

		if r.Error != nil {
			msg := r.Error.Error()
			jr.Error = &msg
		}

		out = append(out, jr)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode results as JSON: %w", err)
	}
	return nil
}
