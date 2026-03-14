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
		writeS3Backend(f, backend.Config)
	} else {
		writeGenericBackend(f, backend.Type, backend.Config)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("closing backend.tf: %w", err)
	}

	return nil
}

// writeS3Backend writes an S3-compatible backend block with Terraform's
// S3-specific skip flags for use with MinIO and other S3-compatible stores.
func writeS3Backend(f *os.File, cfg map[string]interface{}) {
	_, _ = fmt.Fprintf(f, "terraform {\n")
	_, _ = fmt.Fprintf(f, "  backend \"s3\" {\n")

	if v, ok := cfg["bucket"]; ok {
		_, _ = fmt.Fprintf(f, "    bucket                      = %s\n", hclValue(v))
	}
	if v, ok := cfg["key"]; ok {
		_, _ = fmt.Fprintf(f, "    key                         = %s\n", hclValue(v))
	}
	if v, ok := cfg["region"]; ok {
		_, _ = fmt.Fprintf(f, "    region                      = %s\n", hclValue(v))
	}
	if v, ok := cfg["endpoint"]; ok {
		_, _ = fmt.Fprintf(f, "    endpoints                   = { s3 = %s }\n", hclValue(v))
	}

	_, _ = fmt.Fprintf(f, "    skip_credentials_validation = true\n")
	_, _ = fmt.Fprintf(f, "    skip_requesting_account_id  = true\n")
	_, _ = fmt.Fprintf(f, "    skip_metadata_api_check     = true\n")
	_, _ = fmt.Fprintf(f, "    skip_region_validation      = true\n")
	_, _ = fmt.Fprintf(f, "    use_path_style              = true\n")

	if v, ok := cfg["access_key"]; ok {
		_, _ = fmt.Fprintf(f, "    access_key                  = %s\n", hclValue(v))
	}
	if v, ok := cfg["secret_key"]; ok {
		_, _ = fmt.Fprintf(f, "    secret_key                  = %s\n", hclValue(v))
	}

	_, _ = fmt.Fprintf(f, "  }\n")
	_, _ = fmt.Fprintf(f, "}\n")
}

// writeGenericBackend writes a backend block for any backend type, emitting
// all config keys in sorted order.
func writeGenericBackend(f *os.File, backendType string, cfg map[string]interface{}) {
	_, _ = fmt.Fprintf(f, "terraform {\n")
	_, _ = fmt.Fprintf(f, "  backend %q {\n", backendType)

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		_, _ = fmt.Fprintf(f, "    %s = %s\n", k, hclValue(cfg[k]))
	}

	_, _ = fmt.Fprintf(f, "  }\n")
	_, _ = fmt.Fprintf(f, "}\n")
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
