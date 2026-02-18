// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package callback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/butlerdotdev/butler-runner/internal/config"
)

// StatusDetails contains details for a status update.
type StatusDetails struct {
	ExitCode           int    `json:"exit_code,omitempty"`
	ResourcesToAdd     int    `json:"resources_to_add,omitempty"`
	ResourcesToChange  int    `json:"resources_to_change,omitempty"`
	ResourcesToDestroy int    `json:"resources_to_destroy,omitempty"`
	PlanJSON           string `json:"plan_json,omitempty"`
	PlanText           string `json:"plan_text,omitempty"`
}

// Client posts results back to Butler API via callback URLs.
type Client struct {
	baseURL   string
	token     string
	callbacks config.CallbackURLs
	client    *http.Client
}

// NewClient creates a new callback client.
func NewClient(baseURL, token string, callbacks config.CallbackURLs) *Client {
	return &Client{
		baseURL:   baseURL,
		token:     token,
		callbacks: callbacks,
		client:    &http.Client{},
	}
}

// ReportStatus posts a status update.
func (c *Client) ReportStatus(ctx context.Context, status string, details *StatusDetails) error {
	body := map[string]interface{}{
		"status": status,
	}
	if details != nil {
		body["exit_code"] = details.ExitCode
		body["resources_to_add"] = details.ResourcesToAdd
		body["resources_to_change"] = details.ResourcesToChange
		body["resources_to_destroy"] = details.ResourcesToDestroy
		if details.PlanJSON != "" {
			body["plan_json"] = details.PlanJSON
		}
		if details.PlanText != "" {
			body["plan_text"] = details.PlanText
		}
	}

	return c.post(ctx, c.callbacks.StatusURL, body)
}

// ReportOutputs posts terraform outputs.
func (c *Client) ReportOutputs(ctx context.Context, outputs map[string]interface{}) error {
	return c.post(ctx, c.callbacks.OutputsURL, map[string]interface{}{
		"outputs": outputs,
	})
}

func (c *Client) post(ctx context.Context, path string, body interface{}) error {
	url := c.baseURL + path

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("posting to %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("callback %s returned %d", path, resp.StatusCode)
	}

	return nil
}
