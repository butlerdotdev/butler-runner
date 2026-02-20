// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/butlerdotdev/butler-runner/internal/config"
)

// WriteBackendOverride writes a backend.tf file into workDir based on the
// provided state backend configuration. If backend is nil, it is a no-op.
func WriteBackendOverride(workDir string, backend *config.StateBackendConfig) error {
	if backend == nil {
		return nil
	}

	path := filepath.Join(workDir, "backend.tf")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("creating backend.tf: %w", err)
	}
	defer func() { _ = f.Close() }()

	if backend.Type == "s3" {
		if err := writeS3Backend(f, backend.Config); err != nil {
			return err
		}
	} else {
		if err := writeGenericBackend(f, backend.Type, backend.Config); err != nil {
			return err
		}
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("closing backend.tf: %w", err)
	}

	return nil
}

// writeS3Backend writes an S3-compatible backend block with Terraform's
// S3-specific skip flags for use with MinIO and other S3-compatible stores.
func writeS3Backend(f *os.File, cfg map[string]interface{}) error {
	fmt.Fprintf(f, "terraform {\n")
	fmt.Fprintf(f, "  backend \"s3\" {\n")

	if v, ok := cfg["bucket"]; ok {
		fmt.Fprintf(f, "    bucket                      = %s\n", hclValue(v))
	}
	if v, ok := cfg["key"]; ok {
		fmt.Fprintf(f, "    key                         = %s\n", hclValue(v))
	}
	if v, ok := cfg["region"]; ok {
		fmt.Fprintf(f, "    region                      = %s\n", hclValue(v))
	}
	if v, ok := cfg["endpoint"]; ok {
		fmt.Fprintf(f, "    endpoints                   = { s3 = %s }\n", hclValue(v))
	}

	fmt.Fprintf(f, "    skip_credentials_validation = true\n")
	fmt.Fprintf(f, "    skip_requesting_account_id  = true\n")
	fmt.Fprintf(f, "    skip_metadata_api_check     = true\n")
	fmt.Fprintf(f, "    skip_region_validation      = true\n")
	fmt.Fprintf(f, "    use_path_style              = true\n")

	if v, ok := cfg["access_key"]; ok {
		fmt.Fprintf(f, "    access_key                  = %s\n", hclValue(v))
	}
	if v, ok := cfg["secret_key"]; ok {
		fmt.Fprintf(f, "    secret_key                  = %s\n", hclValue(v))
	}

	fmt.Fprintf(f, "  }\n")
	fmt.Fprintf(f, "}\n")
	return nil
}

// writeGenericBackend writes a backend block for any backend type, emitting
// all config keys in sorted order.
func writeGenericBackend(f *os.File, backendType string, cfg map[string]interface{}) error {
	fmt.Fprintf(f, "terraform {\n")
	fmt.Fprintf(f, "  backend %q {\n", backendType)

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(f, "    %s = %s\n", k, hclValue(cfg[k]))
	}

	fmt.Fprintf(f, "  }\n")
	fmt.Fprintf(f, "}\n")
	return nil
}

// hclValue formats a Go value as an HCL literal. Strings are quoted,
// booleans and numbers are written unquoted.
func hclValue(v interface{}) string {
	switch val := v.(type) {
	case bool:
		return fmt.Sprintf("%t", val)
	case float64:
		// JSON numbers decode as float64. If the value is integral, emit
		// without a decimal point.
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case string:
		return fmt.Sprintf("%q", val)
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", val))
	}
}
