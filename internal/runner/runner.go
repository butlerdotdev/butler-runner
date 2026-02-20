// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/butlerdotdev/butler-runner/internal/callback"
	"github.com/butlerdotdev/butler-runner/internal/cancel"
	"github.com/butlerdotdev/butler-runner/internal/config"
	"github.com/butlerdotdev/butler-runner/internal/logstream"
	"github.com/butlerdotdev/butler-runner/internal/source"
	"github.com/butlerdotdev/butler-runner/internal/terraform"
)

type ManagedConfig struct {
	ButlerURL string
	RunID     string
	Token     string
}

type LocalConfig struct {
	WorkingDir string
	Operation  string
	TfVersion  string
}

// RunManaged executes a Butler-managed run.
func RunManaged(ctx context.Context, logger *slog.Logger, cfg ManagedConfig) error {
	// 1. Fetch execution config
	execCfg, err := config.FetchConfig(ctx, logger, cfg.ButlerURL, cfg.RunID, cfg.Token)
	if err != nil {
		return fmt.Errorf("fetching config: %w", err)
	}

	// 2. Create callback client
	cb := callback.NewClient(cfg.ButlerURL, cfg.Token, execCfg.Callbacks)

	// Report running status
	if err := cb.ReportStatus(ctx, "running", nil); err != nil {
		logger.Warn("failed to report running status", "error", err)
	}

	// 3. Resolve terraform version
	tfPath, err := terraform.ResolveVersion(ctx, logger, execCfg.TerraformVersion)
	if err != nil {
		_ = cb.ReportStatus(ctx, "failed", &callback.StatusDetails{ExitCode: 1})
		return fmt.Errorf("resolving terraform version: %w", err)
	}

	// 4. Clone/download source
	workDir, err := source.Prepare(ctx, logger, execCfg.Source)
	if err != nil {
		_ = cb.ReportStatus(ctx, "failed", &callback.StatusDetails{ExitCode: 1})
		return fmt.Errorf("preparing source: %w", err)
	}
	defer func() { _ = os.RemoveAll(filepath.Dir(workDir)) }()

	// 5. Set cloud integration / variable set env vars
	var envVarKeys []string
	for key, v := range execCfg.EnvVars {
		val, ok := v.Value.(string)
		if !ok {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			logger.Warn("failed to set env var", "key", key, "error", err)
			continue
		}
		envVarKeys = append(envVarKeys, key)
	}
	if len(envVarKeys) > 0 {
		logger.Info("env vars set for terraform", "count", len(envVarKeys), "keys", envVarKeys)
	}
	// Clean up env vars after run completes
	defer func() {
		for _, key := range envVarKeys {
			_ = os.Unsetenv(key)
		}
	}()

	// 6. Write terraform.tfvars.json
	tfvarsPath, err := terraform.WriteTfvars(workDir, execCfg.Variables, execCfg.UpstreamOutputs)
	if err != nil {
		_ = cb.ReportStatus(ctx, "failed", &callback.StatusDetails{ExitCode: 1})
		return fmt.Errorf("writing tfvars: %w", err)
	}
	defer terraform.SecureDelete(tfvarsPath)

	// 7. Start cancellation watcher
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	watcher := cancel.NewWatcher(cfg.ButlerURL, cfg.RunID, cfg.Token, logger)
	go watcher.Start(cancelCtx, cancelFunc)

	// 8. Set up log streaming
	stdoutLog := logstream.NewWriter(ctx, cb, "stdout", logger, 2*time.Second, 0)
	stderrLog := logstream.NewWriter(ctx, cb, "stderr", logger, 2*time.Second, stdoutLog.Sequence())
	defer stderrLog.Close()
	defer stdoutLog.Close()

	// 9. Run terraform
	exec := terraform.NewExecutor(tfPath, workDir, logger)
	exec.SetLogWriters(stdoutLog, stderrLog)

	// Init
	logger.Info("running terraform init")
	if err := exec.Init(cancelCtx); err != nil {
		_ = cb.ReportStatus(ctx, "failed", &callback.StatusDetails{ExitCode: 1})
		return fmt.Errorf("terraform init: %w", err)
	}

	// Execute operation
	result, err := exec.Run(cancelCtx, execCfg.Operation)
	if err != nil {
		exitCode := 1
		if result != nil {
			exitCode = result.ExitCode
		}
		_ = cb.ReportStatus(ctx, "failed", &callback.StatusDetails{
			ExitCode:           exitCode,
			ResourcesToAdd:     result.ResourcesToAdd,
			ResourcesToChange:  result.ResourcesToChange,
			ResourcesToDestroy: result.ResourcesToDestroy,
		})
		return fmt.Errorf("terraform %s: %w", execCfg.Operation, err)
	}

	// 10. Report success
	details := &callback.StatusDetails{
		ExitCode:           result.ExitCode,
		ResourcesToAdd:     result.ResourcesToAdd,
		ResourcesToChange:  result.ResourcesToChange,
		ResourcesToDestroy: result.ResourcesToDestroy,
	}
	if result.PlanJSON != "" {
		details.PlanJSON = result.PlanJSON
	}
	if result.PlanText != "" {
		details.PlanText = result.PlanText
	}

	if err := cb.ReportStatus(ctx, "succeeded", details); err != nil {
		logger.Warn("failed to report success status", "error", err)
	}

	// 11. Report outputs if apply
	if result.Outputs != nil {
		if err := cb.ReportOutputs(ctx, result.Outputs); err != nil {
			logger.Warn("failed to report outputs", "error", err)
		}
	}

	logger.Info("run completed successfully",
		"operation", execCfg.Operation,
		"exitCode", result.ExitCode,
	)

	return nil
}

// RunLocal executes a local terraform run without Butler API.
func RunLocal(ctx context.Context, logger *slog.Logger, cfg LocalConfig) error {
	logger.Info("running in local mode",
		"workingDir", cfg.WorkingDir,
		"operation", cfg.Operation,
	)

	// Resolve terraform version
	tfPath, err := terraform.ResolveVersion(ctx, logger, cfg.TfVersion)
	if err != nil {
		return fmt.Errorf("resolving terraform version: %w", err)
	}

	absDir, err := filepath.Abs(cfg.WorkingDir)
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	exec := terraform.NewExecutor(tfPath, absDir, logger)

	// Init
	logger.Info("running terraform init")
	if err := exec.Init(ctx); err != nil {
		return fmt.Errorf("terraform init: %w", err)
	}

	// Run
	result, err := exec.Run(ctx, cfg.Operation)
	if err != nil {
		return fmt.Errorf("terraform %s: %w", cfg.Operation, err)
	}

	logger.Info("local run completed",
		"operation", cfg.Operation,
		"exitCode", result.ExitCode,
		"resourcesToAdd", result.ResourcesToAdd,
		"resourcesToChange", result.ResourcesToChange,
		"resourcesToDestroy", result.ResourcesToDestroy,
	)

	return nil
}
