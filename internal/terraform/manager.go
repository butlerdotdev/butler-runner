// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package terraform

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const defaultVersion = "1.9.8"

// ResolveVersion returns the path to a terraform binary for the requested version.
func ResolveVersion(ctx context.Context, logger *slog.Logger, version string) (string, error) {
	if version == "" {
		version = defaultVersion
	}

	// Check if terraform is already on PATH and matches version
	if path, err := exec.LookPath("terraform"); err == nil {
		if installedVersion, err := getInstalledVersion(ctx, path); err == nil {
			if installedVersion == version {
				logger.Info("using system terraform", "version", version, "path", path)
				return path, nil
			}
		}
	}

	// Check cache
	cacheDir := getCacheDir()
	cachedPath := filepath.Join(cacheDir, version, "terraform")
	if runtime.GOOS == "windows" {
		cachedPath += ".exe"
	}
	if _, err := os.Stat(cachedPath); err == nil {
		logger.Info("using cached terraform", "version", version, "path", cachedPath)
		return cachedPath, nil
	}

	// Download
	logger.Info("downloading terraform", "version", version)
	if err := downloadTerraform(ctx, version, cacheDir); err != nil {
		return "", fmt.Errorf("downloading terraform %s: %w", version, err)
	}

	logger.Info("terraform downloaded", "version", version, "path", cachedPath)
	return cachedPath, nil
}

func getCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".butler-runner", "terraform")
	}
	return filepath.Join(home, ".butler-runner", "terraform")
}

func getInstalledVersion(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "version", "-json")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: parse text output
		cmd = exec.CommandContext(ctx, path, "version")
		output, err = cmd.Output()
		if err != nil {
			return "", err
		}
		// Parse "Terraform v1.9.8\n..."
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 {
			parts := strings.Fields(lines[0])
			if len(parts) >= 2 {
				return strings.TrimPrefix(parts[1], "v"), nil
			}
		}
		return "", fmt.Errorf("could not parse terraform version")
	}
	// JSON output contains {"terraform_version": "1.9.8", ...}
	_ = output
	return "", fmt.Errorf("version parsing not implemented for JSON output")
}

func downloadTerraform(ctx context.Context, version, cacheDir string) error {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	versionDir := filepath.Join(cacheDir, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	url := fmt.Sprintf(
		"https://releases.hashicorp.com/terraform/%s/terraform_%s_%s_%s.zip",
		version, version, osName, arch,
	)

	// Download zip
	zipPath := filepath.Join(versionDir, "terraform.zip")
	cmd := exec.CommandContext(ctx, "curl", "-sSL", "-o", zipPath, url)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("downloading %s: %s: %w", url, string(output), err)
	}

	// Unzip
	cmd = exec.CommandContext(ctx, "unzip", "-o", "-d", versionDir, zipPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("unzipping: %s: %w", string(output), err)
	}

	// Cleanup zip
	os.Remove(zipPath)

	// Make executable
	tfPath := filepath.Join(versionDir, "terraform")
	if err := os.Chmod(tfPath, 0o755); err != nil {
		return fmt.Errorf("chmod terraform: %w", err)
	}

	return nil
}
