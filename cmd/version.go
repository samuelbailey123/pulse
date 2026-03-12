package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// These variables are injected at build time via -ldflags.
// Example:
//
//	go build -ldflags "-X github.com/samuelbailey123/pulse/cmd.Version=1.0.0 \
//	                   -X github.com/samuelbailey123/pulse/cmd.Commit=abc1234 \
//	                   -X github.com/samuelbailey123/pulse/cmd.Date=2026-03-12"
var (
	// Version is the semantic version string (e.g. "1.0.0").
	Version = "dev"
	// Commit is the short Git commit hash at build time.
	Commit = "none"
	// Date is the build timestamp in YYYY-MM-DD format.
	Date = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Pulse version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "pulse %s (commit %s, built %s)\n", Version, Commit, Date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
