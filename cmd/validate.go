package cmd

import (
	"fmt"
	"os"

	"github.com/samuelbailey123/pulse/internal/config"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the Pulse configuration file",
	Long: `Validate loads the configuration file, checks all fields for correctness,
and reports any errors found. Exits with a non-zero status if the
configuration is invalid.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		errs := config.Validate(cfg)
		if len(errs) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "config %q is valid (%d target(s))\n", configPath, len(cfg.Targets))
			return nil
		}

		fmt.Fprintf(os.Stderr, "config %q has %d error(s):\n", configPath, len(errs))
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		os.Exit(1)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)
}
