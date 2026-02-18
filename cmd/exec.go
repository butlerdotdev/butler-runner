// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/butlerdotdev/butler-runner/internal/runner"
	"github.com/spf13/cobra"
)

var (
	butlerURL  string
	runID      string
	token      string
	localMode  bool
	workingDir string
	operation  string
	tfVersion  string
)

func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:   "butler-runner",
	Short: "Universal execution adapter for Butler IaC runs",
}

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a Butler IaC run",
	Long: `Execute a Butler IaC run in managed or local mode.

Managed mode (default):
  butler-runner exec --butler-url=URL --run-id=ID --token=TOKEN

  Fetches execution config from Butler API, runs terraform,
  and reports results via callbacks.

Local mode:
  butler-runner exec --local --working-dir=./modules/vpc --operation=plan

  Runs terraform directly without Butler API interaction.`,
	RunE: runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)

	execCmd.Flags().StringVar(&butlerURL, "butler-url", os.Getenv("BUTLER_URL"), "Butler API base URL")
	execCmd.Flags().StringVar(&runID, "run-id", os.Getenv("BUTLER_RUN_ID"), "Butler run ID")
	execCmd.Flags().StringVar(&token, "token", os.Getenv("BUTLER_TOKEN"), "Butler callback token")
	execCmd.Flags().BoolVar(&localMode, "local", false, "Run in local mode (no Butler API)")
	execCmd.Flags().StringVar(&workingDir, "working-dir", ".", "Working directory for local mode")
	execCmd.Flags().StringVar(&operation, "operation", "plan", "Terraform operation (plan/apply/destroy)")
	execCmd.Flags().StringVar(&tfVersion, "tf-version", "", "Terraform version (empty = use default)")
}

func runExec(cmd *cobra.Command, args []string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Handle OS signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	if localMode {
		return runner.RunLocal(ctx, logger, runner.LocalConfig{
			WorkingDir: workingDir,
			Operation:  operation,
			TfVersion:  tfVersion,
		})
	}

	// Managed mode — validate required inputs
	if butlerURL == "" {
		return fmt.Errorf("--butler-url or BUTLER_URL is required in managed mode")
	}
	if runID == "" {
		return fmt.Errorf("--run-id or BUTLER_RUN_ID is required in managed mode")
	}
	if token == "" {
		return fmt.Errorf("--token or BUTLER_TOKEN is required in managed mode")
	}

	return runner.RunManaged(ctx, logger, runner.ManagedConfig{
		ButlerURL: butlerURL,
		RunID:     runID,
		Token:     token,
	})
}
