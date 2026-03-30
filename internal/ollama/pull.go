// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// pullClient is a dedicated HTTP client for model pulls. It sets a
// ResponseHeaderTimeout so that a hanging Ollama server doesn't block
// forever, while allowing the streaming body transfer to take as long
// as needed.
var pullClient = &http.Client{
	Transport: func() *http.Transport {
		t, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return &http.Transport{
				ResponseHeaderTimeout: 30 * time.Second,
			}
		}
		clone := t.Clone()
		clone.ResponseHeaderTimeout = 30 * time.Second
		return clone
	}(),
}

// PullChunk is a single progress update from the Ollama pull API.
type PullChunk struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"` // Ollama streams errors in this field
}

// PullScanner wraps the streaming response from the Ollama pull API.
// Callers must call Close when they are done iterating, even if Next
// has not yet returned nil.
type PullScanner struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
}

// Close releases the underlying HTTP response body. It is safe to call
// multiple times.
func (ps *PullScanner) Close() error {
	if err := ps.body.Close(); err != nil {
		return fmt.Errorf("close pull scanner: %w", err)
	}
	return nil
}

// Next returns the next progress chunk, or nil at EOF.
func (ps *PullScanner) Next() (*PullChunk, error) {
	for ps.scanner.Scan() {
		line := strings.TrimSpace(ps.scanner.Text())
		if line == "" {
			continue
		}
		var chunk PullChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue // skip malformed lines
		}
		return &chunk, nil
	}
	if err := ps.scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning pull response: %w", err)
	}
	return nil, nil // EOF
}

// PullModel initiates a model pull via the Ollama native API at baseURL.
// The baseURL should be the Ollama server root (e.g. "http://localhost:11434").
func PullModel(ctx context.Context, baseURL, model string) (*PullScanner, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	body, err := json.Marshal(map[string]string{"name": model})
	if err != nil {
		return nil, fmt.Errorf("marshal pull request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		baseURL+"/api/pull",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build pull request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := pullClient.Do(req) //nolint:gosec // baseURL from user config
	if err != nil {
		return nil, fmt.Errorf(
			"cannot reach %s -- start it with `ollama serve`", baseURL,
		)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("pull failed (%d): %s", resp.StatusCode, string(errBody))
	}

	return &PullScanner{
		body:    resp.Body,
		scanner: bufio.NewScanner(resp.Body),
	}, nil
}
