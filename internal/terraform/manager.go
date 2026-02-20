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

// binaryNames is the ordered list of IaC binaries to search on PATH.
// OpenTofu is preferred since it is CNCF-maintained and properly code-signed.
var binaryNames = []string{"tofu", "terraform"}

// ResolveVersion returns the path to a terraform/tofu binary for the requested version.
// It checks both tofu and terraform on PATH, then falls back to downloading.
func ResolveVersion(ctx context.Context, logger *slog.Logger, version string) (string, error) {
	if version == "" {
		version = defaultVersion
	}

	// Check if tofu or terraform is on PATH and matches version
	for _, bin := range binaryNames {
		if path, err := exec.LookPath(bin); err == nil {
			if installedVersion, err := getInstalledVersion(ctx, path); err == nil {
				if installedVersion == version {
					logger.Info("using system binary", "binary", bin, "version", version, "path", path)
					return path, nil
				}
				logger.Info("system binary version mismatch", "binary", bin, "installed", installedVersion, "requested", version)
			}
		}
	}

	// If any binary is on PATH regardless of version, use it (local mode convenience).
	// This allows local testing with whatever version is installed.
	for _, bin := range binaryNames {
		if path, err := exec.LookPath(bin); err == nil {
			logger.Info("using system binary (version mismatch accepted)", "binary", bin, "path", path)
			return path, nil
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
	// Prefer RUNNER_TEMP (GitHub Actions sets this to a writable dir).
	// HOME inside Docker container actions (/github/home) is often root-owned.
	if d := os.Getenv("RUNNER_TEMP"); d != "" {
		return filepath.Join(d, ".butler-runner", "terraform")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".butler-runner", "terraform")
	}
	return filepath.Join(home, ".butler-runner", "terraform")
}

func getInstalledVersion(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Parse "Terraform v1.9.8" or "OpenTofu v1.11.5"
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 2 {
			return strings.TrimPrefix(parts[len(parts)-1], "v"), nil
		}
	}
	return "", fmt.Errorf("could not parse version output: %s", string(output))
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
	_ = os.Remove(zipPath)

	// Make executable
	tfPath := filepath.Join(versionDir, "terraform")
	if err := os.Chmod(tfPath, 0o755); err != nil {
		return fmt.Errorf("chmod terraform: %w", err)
	}

	return nil
}
