// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/butlerdotdev/butler-runner/internal/config"
)

// Prepare clones/downloads source code and returns the working directory path.
func Prepare(ctx context.Context, logger *slog.Logger, src config.SourceConfig) (string, error) {
	switch src.Type {
	case "git":
		return cloneGit(ctx, logger, src)
	default:
		return "", fmt.Errorf("unsupported source type: %s", src.Type)
	}
}

func cloneGit(ctx context.Context, logger *slog.Logger, src config.SourceConfig) (string, error) {
	tmpDir, err := os.MkdirTemp("", "butler-runner-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	cloneDir := filepath.Join(tmpDir, "source")

	logger.Info("cloning repository",
		"repo", src.GitRepo,
		"ref", src.GitRef,
	)

	cmd := exec.CommandContext(ctx, "git", "clone",
		"--depth=1",
		"--branch", src.GitRef,
		src.GitRepo,
		cloneDir,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		// If branch clone fails (ref might be a commit), try full clone + checkout
		cmd = exec.CommandContext(ctx, "git", "clone", src.GitRepo, cloneDir)
		if output2, err2 := cmd.CombinedOutput(); err2 != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("git clone failed: %s / %s: %w", string(output), string(output2), err2)
		}
		checkoutCmd := exec.CommandContext(ctx, "git", "checkout", src.GitRef)
		checkoutCmd.Dir = cloneDir
		if output3, err3 := checkoutCmd.CombinedOutput(); err3 != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("git checkout failed: %s: %w", string(output3), err3)
		}
	}

	workDir := cloneDir
	if src.WorkingDirectory != "" {
		workDir = filepath.Join(cloneDir, src.WorkingDirectory)
		if _, err := os.Stat(workDir); err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("working directory %s not found in repo: %w", src.WorkingDirectory, err)
		}
	}

	logger.Info("source prepared", "workDir", workDir)
	return workDir, nil
}
