package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/samuelbailey123/pulse/internal/checker"
	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/samuelbailey123/pulse/internal/engine"
	"github.com/samuelbailey123/pulse/internal/output"
	"github.com/spf13/cobra"
)

// checkTimeout is the maximum time allowed for a one-shot check run across all
// targets. Individual per-target timeouts are enforced within this outer budget.
const checkTimeout = 30 * time.Second

var checkJSON bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run a one-shot health check of all targets",
	Long: `Check loads the configuration file, runs a single health check against every
configured target concurrently, prints the results, and exits.

Exit code is 1 if any target is DOWN, 0 otherwise.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()

		eng := engine.New(cfg)
		results := eng.RunOnce(ctx)

		if checkJSON {
			if err := output.JSON(results, cmd.OutOrStdout()); err != nil {
				return fmt.Errorf("write JSON output: %w", err)
			}
		} else {
			output.Table(results, cmd.OutOrStdout())
		}

		if anyDown(results) {
			os.Exit(1)
		}

		return nil
	},
}

// anyDown reports whether any result in the slice has a DOWN status.
func anyDown(results []engine.TargetResult) bool {
	for _, tr := range results {
		if tr.Result.Status == checker.StatusDown {
			return true
		}
	}
	return false
}

func init() {
	checkCmd.Flags().BoolVar(&checkJSON, "json", false, "output results as JSON instead of a table")
	rootCmd.AddCommand(checkCmd)
}
