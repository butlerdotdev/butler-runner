// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package callback

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/butlerdotdev/butler-runner/internal/config"
)

func TestReportStatus(t *testing.T) {
	var receivedBody map[string]interface{}
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", config.CallbackURLs{
		StatusURL: "/v1/ci/module-runs/run-1/status",
	})

	err := client.ReportStatus(context.Background(), "running", nil)
	if err != nil {
		t.Fatalf("ReportStatus failed: %v", err)
	}

	if receivedAuth != "Bearer test-token" {
		t.Errorf("expected auth header 'Bearer test-token', got %q", receivedAuth)
	}

	if receivedBody["status"] != "running" {
		t.Errorf("expected status 'running', got %v", receivedBody["status"])
	}
}

func TestReportStatusWithDetails(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", config.CallbackURLs{
		StatusURL: "/v1/ci/module-runs/run-1/status",
	})

	err := client.ReportStatus(context.Background(), "succeeded", &StatusDetails{
		ExitCode:       0,
		ResourcesToAdd: 3,
	})
	if err != nil {
		t.Fatalf("ReportStatus failed: %v", err)
	}

	if receivedBody["status"] != "succeeded" {
		t.Errorf("expected status 'succeeded', got %v", receivedBody["status"])
	}
}

func TestReportStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", config.CallbackURLs{
		StatusURL: "/v1/ci/module-runs/run-1/status",
	})

	err := client.ReportStatus(context.Background(), "running", nil)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestReportOutputs(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", config.CallbackURLs{
		OutputsURL: "/v1/ci/module-runs/run-1/outputs",
	})

	outputs := map[string]interface{}{
		"vpc_id": "vpc-abc123",
	}
	err := client.ReportOutputs(context.Background(), outputs)
	if err != nil {
		t.Fatalf("ReportOutputs failed: %v", err)
	}

	if receivedBody["outputs"] == nil {
		t.Error("expected outputs in body")
	}
}
