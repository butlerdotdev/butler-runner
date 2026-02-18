// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package cancel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const pollInterval = 30 * time.Second

// Watcher polls the Butler API for run cancellation.
type Watcher struct {
	butlerURL string
	runID     string
	token     string
	logger    *slog.Logger
}

// NewWatcher creates a new cancellation watcher.
func NewWatcher(butlerURL, runID, token string, logger *slog.Logger) *Watcher {
	return &Watcher{
		butlerURL: butlerURL,
		runID:     runID,
		token:     token,
		logger:    logger,
	}
}

// Start begins polling for cancellation. When cancelled, calls cancelFunc.
func (w *Watcher) Start(ctx context.Context, cancelFunc context.CancelFunc) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if w.isCancelled(ctx) {
				w.logger.Info("run cancelled by user, initiating shutdown")
				cancelFunc()
				return
			}
		}
	}
}

func (w *Watcher) isCancelled(ctx context.Context) bool {
	url := fmt.Sprintf("%s/v1/ci/module-runs/%s/status", w.butlerURL, w.runID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+w.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	return result.Status == "cancelled"
}
