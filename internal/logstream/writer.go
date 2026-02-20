// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package logstream

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/butlerdotdev/butler-runner/internal/callback"
)

// Writer is an io.Writer that buffers log lines and flushes them
// to the Butler callback API periodically.
type Writer struct {
	ctx       context.Context
	cb        *callback.Client
	stream    string // "stdout" or "stderr"
	logger    *slog.Logger
	mu        sync.Mutex
	buf       []callback.LogEntry
	seq       int
	flushTick *time.Ticker
	done      chan struct{}
	pr        *io.PipeReader
	pw        *io.PipeWriter
}

// NewWriter creates a log writer that streams to the callback API.
// It starts a background goroutine that reads lines and flushes every interval.
func NewWriter(ctx context.Context, cb *callback.Client, stream string, logger *slog.Logger, flushInterval time.Duration, startSeq int) *Writer {
	pr, pw := io.Pipe()
	w := &Writer{
		ctx:       ctx,
		cb:        cb,
		stream:    stream,
		logger:    logger,
		seq:       startSeq,
		flushTick: time.NewTicker(flushInterval),
		done:      make(chan struct{}),
		pr:        pr,
		pw:        pw,
	}
	go w.readLines()
	go w.flushLoop()
	return w
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	return w.pw.Write(p)
}

// Sequence returns the current sequence number (for chaining stdout → stderr).
func (w *Writer) Sequence() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seq
}

// Close flushes remaining logs and stops the background goroutines.
func (w *Writer) Close() {
	_ = w.pw.Close()
	<-w.done // wait for readLines to finish
	w.flushTick.Stop()
	w.flush() // final flush
}

func (w *Writer) readLines() {
	defer close(w.done)
	scanner := bufio.NewScanner(w.pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		w.mu.Lock()
		w.seq++
		w.buf = append(w.buf, callback.LogEntry{
			Sequence: w.seq,
			Stream:   w.stream,
			Content:  line,
		})
		w.mu.Unlock()
	}
}

func (w *Writer) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			w.flush()
		case <-w.done:
			return
		}
	}
}

func (w *Writer) flush() {
	w.mu.Lock()
	if len(w.buf) == 0 {
		w.mu.Unlock()
		return
	}
	batch := w.buf
	w.buf = nil
	w.mu.Unlock()

	// Truncate very long lines to avoid huge payloads
	for i := range batch {
		if len(batch[i].Content) > 4096 {
			batch[i].Content = batch[i].Content[:4096] + "... (truncated)"
		}
	}

	// Send in chunks of 100 to avoid request size limits
	for i := 0; i < len(batch); i += 100 {
		end := i + 100
		if end > len(batch) {
			end = len(batch)
		}
		if err := w.cb.SendLogs(w.ctx, batch[i:end]); err != nil {
			w.logger.Warn("failed to send logs",
				"stream", w.stream,
				"count", end-i,
				"error", err,
			)
		}
	}

	if w.logger.Enabled(w.ctx, slog.LevelDebug) {
		lines := make([]string, len(batch))
		for i, e := range batch {
			lines[i] = e.Content
		}
		w.logger.Debug("flushed logs",
			"stream", w.stream,
			"count", len(batch),
			"preview", strings.Join(lines[:min(len(lines), 3)], " | "),
		)
	}
}
