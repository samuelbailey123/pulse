// Package cmd contains the Pulse CLI command tree.
package cmd

import (
	"github.com/spf13/cobra"
)

// configPath is the path to the Pulse YAML configuration file, set by --config.
var configPath string

var rootCmd = &cobra.Command{
	Use:   "pulse",
	Short: "Multi-endpoint health checker",
	Long: `Pulse is a lightweight, configuration-driven health monitoring tool.

It reads a YAML configuration file describing HTTP, TCP, DNS, and TLS
endpoints, checks each one on a configurable interval, and fires webhook
alerts after a configurable number of consecutive failures.

Get started:

  pulse init               # generate a pulse.yaml template
  pulse validate           # verify your configuration
  pulse check              # one-shot health check
  pulse watch              # live monitoring dashboard`,
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(
		&configPath,
		"config", "c",
		"pulse.yaml",
		"path to the Pulse configuration file",
	)
}
