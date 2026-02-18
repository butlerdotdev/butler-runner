// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// ExecutionConfig is the full execution config fetched from Butler API.
type ExecutionConfig struct {
	RunID            string                 `json:"runId"`
	Operation        string                 `json:"operation"`
	TerraformVersion string                 `json:"terraformVersion"`
	Source           SourceConfig           `json:"source"`
	Variables        map[string]Variable    `json:"variables"`
	UpstreamOutputs  map[string]interface{} `json:"upstreamOutputs"`
	StateBackend     *StateBackendConfig    `json:"stateBackend"`
	Callbacks        CallbackURLs           `json:"callbacks"`
}

type SourceConfig struct {
	Type             string `json:"type"` // "git"
	GitRepo          string `json:"gitRepo"`
	GitRef           string `json:"gitRef"`
	WorkingDirectory string `json:"workingDirectory"`
}

type Variable struct {
	Value     interface{} `json:"value"`
	Sensitive bool        `json:"sensitive"`
}

type StateBackendConfig struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config"`
}

type CallbackURLs struct {
	StatusURL  string `json:"statusUrl"`
	LogsURL    string `json:"logsUrl"`
	PlanURL    string `json:"planUrl"`
	OutputsURL string `json:"outputsUrl"`
}

// FetchConfig retrieves the execution config from Butler API.
func FetchConfig(ctx context.Context, logger *slog.Logger, butlerURL, runID, token string) (*ExecutionConfig, error) {
	url := fmt.Sprintf("%s/v1/ci/module-runs/%s/config", butlerURL, runID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating config request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	logger.Info("fetching execution config", "url", url, "runId", runID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("config endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var cfg ExecutionConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}

	// Log config metadata only — NEVER log variables/secrets
	logger.Info("execution config received",
		"runId", cfg.RunID,
		"operation", cfg.Operation,
		"terraformVersion", cfg.TerraformVersion,
		"sourceType", cfg.Source.Type,
		"variableCount", len(cfg.Variables),
	)

	return &cfg, nil
}
