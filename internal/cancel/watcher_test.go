// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package cancel

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestWatcherDetectsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "cancelled",
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	watcher := NewWatcher(server.URL, "run-1", "token", logger)

	if !watcher.isCancelled(context.Background()) {
		t.Error("expected isCancelled to return true")
	}
}

func TestWatcherNotCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "running",
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	watcher := NewWatcher(server.URL, "run-1", "token", logger)

	if watcher.isCancelled(context.Background()) {
		t.Error("expected isCancelled to return false")
	}
}

func TestWatcherStopsOnContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "running",
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	watcher := NewWatcher(server.URL, "run-1", "token", logger)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		watcher.Start(ctx, func() {})
		close(done)
	}()

	// Cancel the context
	cancel()

	select {
	case <-done:
		// Watcher stopped as expected
	case <-time.After(2 * time.Second):
		t.Error("watcher did not stop after context cancellation")
	}
}
