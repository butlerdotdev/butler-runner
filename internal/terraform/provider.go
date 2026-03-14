// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteProviderOverrides inspects environment variables to determine which
// cloud providers are in use and writes any required provider configuration
// blocks that modules may not include themselves. This is necessary for
// providers like azurerm that require an explicit features {} block.
func WriteProviderOverrides(workDir string, envVarKeys []string) error {
	needsAzure := false
	for _, key := range envVarKeys {
		if key == "ARM_CLIENT_ID" || key == "ARM_TENANT_ID" {
			needsAzure = true
			break
		}
	}

	if !needsAzure {
		return nil
	}

	path := filepath.Join(workDir, "_butler_providers.tf")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("creating provider override: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, _ = fmt.Fprintf(f, "provider \"azurerm\" {\n")
	_, _ = fmt.Fprintf(f, "  features {}\n")
	_, _ = fmt.Fprintf(f, "}\n")

	if err := f.Close(); err != nil {
		return fmt.Errorf("closing provider override: %w", err)
	}

	return nil
}
