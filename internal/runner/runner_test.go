// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package runner

import (
	"testing"
)

func TestLocalConfigDefaults(t *testing.T) {
	cfg := LocalConfig{
		WorkingDir: ".",
		Operation:  "plan",
	}
	if cfg.WorkingDir != "." {
		t.Errorf("expected working dir '.', got %q", cfg.WorkingDir)
	}
	if cfg.Operation != "plan" {
		t.Errorf("expected operation 'plan', got %q", cfg.Operation)
	}
}

func TestManagedConfigValidation(t *testing.T) {
	cfg := ManagedConfig{
		ButlerURL: "https://butler.example.com",
		RunID:     "run-123",
		Token:     "token-abc",
	}
	if cfg.ButlerURL == "" {
		t.Error("expected non-empty ButlerURL")
	}
	if cfg.RunID == "" {
		t.Error("expected non-empty RunID")
	}
	if cfg.Token == "" {
		t.Error("expected non-empty Token")
	}
}
