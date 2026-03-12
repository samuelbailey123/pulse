// Package cmd contains the Pulse CLI command tree.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/samuelbailey123/pulse/internal/engine"
	"github.com/samuelbailey123/pulse/internal/tui"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Start continuous health monitoring with a live dashboard",
	Long: `Watch continuously monitors every target defined in the configuration file
and displays live health results in a full-screen terminal dashboard.

The dashboard updates as each check result arrives. Press q or Ctrl+C to stop.`,
	RunE: runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

// runWatch loads the configuration, starts the engine, and hands control to
// the bubbletea TUI. It returns only when the user quits or a fatal error
// occurs.
func runWatch(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if errs := config.Validate(cfg); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "config error:", e)
		}
		return fmt.Errorf("configuration is invalid")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	eng := engine.New(cfg)
	results := eng.Start(ctx)

	model := tui.NewModel(results, len(cfg.Targets))

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	return nil
}
