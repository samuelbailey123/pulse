package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// exampleConfig is the content written by `pulse init`. It demonstrates every
// supported feature with inline comments.
const exampleConfig = `# pulse.yaml — Pulse configuration file
#
# Pulse monitors HTTP, TCP, DNS, and TLS endpoints on configurable intervals
# and fires webhook alerts after a configurable number of consecutive failures.
#
# Environment variables can be interpolated using ${VAR_NAME} syntax anywhere
# in this file.

# defaults apply to every target that does not set the field explicitly.
defaults:
  interval: 30s        # how often to check each target
  timeout: 5s          # per-check deadline
  method: GET          # HTTP method for http/tls targets
  tls_warn_days: 14    # days before TLS expiry to emit a warning

targets:
  # --- HTTP target ---
  - name: My API
    url: https://api.example.com/health
    type: http          # optional; defaults to "http"
    method: GET
    interval: 30s
    timeout: 5s
    headers:
      Authorization: Bearer ${API_TOKEN}
    expect:
      status: 200
      body_contains: '"status":"ok"'
    alerts:
      - type: webhook
        url: https://hooks.example.com/alert
        after: 3        # alert after 3 consecutive failures

  # --- TCP target ---
  - name: Postgres
    url: db.example.com:5432
    type: tcp
    interval: 15s
    timeout: 3s

  # --- TLS certificate expiry ---
  - name: TLS Expiry Check
    url: https://example.com
    type: tls
    interval: 1h
    timeout: 10s
    tls_warn_days: 30   # override the default warning window

  # --- DNS resolution ---
  - name: DNS Check
    url: api.example.com
    type: dns
    interval: 60s
    timeout: 5s

# global alerts are applied to every target in addition to per-target alerts.
alerts:
  - type: webhook
    url: https://hooks.example.com/global-alert
    after: 5
`

var initOutput string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a Pulse configuration template",
	Long: `Init writes an annotated pulse.yaml template to the current directory.
Use --output to choose a different file name. The command refuses to
overwrite an existing file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(initOutput); err == nil {
			return fmt.Errorf("file %q already exists; remove it or choose a different --output path", initOutput)
		}

		if err := os.WriteFile(initOutput, []byte(exampleConfig), 0o644); err != nil {
			return fmt.Errorf("write %q: %w", initOutput, err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "wrote %q — edit it and run `pulse validate` to check it\n", initOutput)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initOutput, "output", "o", "pulse.yaml", "path to write the generated config")
	rootCmd.AddCommand(initCmd)
}
