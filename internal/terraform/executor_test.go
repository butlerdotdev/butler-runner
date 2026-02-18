// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package terraform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTfvars(t *testing.T) {
	tmpDir := t.TempDir()

	variables := map[string]Variable{
		"region": {Value: "us-east-1", Sensitive: false},
		"token":  {Value: "secret-123", Sensitive: true},
	}

	upstreamOutputs := map[string]interface{}{
		"vpc_id": "vpc-abc123",
	}

	path, err := WriteTfvars(tmpDir, variables, upstreamOutputs)
	if err != nil {
		t.Fatalf("WriteTfvars failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "terraform.tfvars.json")
	if path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading tfvars file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("expected non-empty tfvars file")
	}

	// Verify file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat tfvars file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestSecureDelete(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sensitive.json")

	if err := os.WriteFile(path, []byte("secret data"), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	SecureDelete(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestParseResourceCounts(t *testing.T) {
	e := &Executor{}

	result := &RunResult{
		PlanJSON: `{
			"resource_changes": [
				{"change": {"actions": ["create"]}},
				{"change": {"actions": ["create"]}},
				{"change": {"actions": ["update"]}},
				{"change": {"actions": ["delete"]}},
				{"change": {"actions": ["delete", "create"]}}
			]
		}`,
	}

	e.parseResourceCounts(result)

	if result.ResourcesToAdd != 3 {
		t.Errorf("expected 3 resources to add, got %d", result.ResourcesToAdd)
	}
	if result.ResourcesToChange != 1 {
		t.Errorf("expected 1 resource to change, got %d", result.ResourcesToChange)
	}
	if result.ResourcesToDestroy != 2 {
		t.Errorf("expected 2 resources to destroy, got %d", result.ResourcesToDestroy)
	}
}
