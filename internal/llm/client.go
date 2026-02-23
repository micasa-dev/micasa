// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

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

// Client talks to an OpenAI-compatible API endpoint (Ollama, llama.cpp,
// LM Studio, etc.) for local LLM inference.
type Client struct {
	baseURL  string // e.g. "http://localhost:11434/v1"
	model    string
	timeout  time.Duration
	thinking *bool // nil = don't send; non-nil = send enable_thinking
	http     *http.Client
}

// Message represents a single turn in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamChunk is a single piece of a streaming chat response.
type StreamChunk struct {
	Content string
	Done    bool
	Err     error
}

// --- OpenAI-compatible request/response types ---

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Stream         bool            `json:"stream"`
	Temperature    *float64        `json:"temperature,omitempty"`
	Options        map[string]any  `json:"options,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

// ChatOption configures a chat completion request.
type ChatOption func(*chatRequest)

// WithJSON constrains the model output to valid JSON.
func WithJSON() ChatOption {
	return func(r *chatRequest) {
		r.ResponseFormat = &responseFormat{Type: "json_object"}
	}
}

type chatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// chatCompletionResponse is the non-streaming response shape.
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// NewClient creates an LLM client targeting the given OpenAI-compatible
// endpoint and model. The timeout controls quick operations like ping and
// model listing.
func NewClient(baseURL, model string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		timeout: timeout,
		http:    &http.Client{},
	}
}

// Model returns the configured model name.
func (c *Client) Model() string {
	return c.model
}

// SetModel switches the active model. The caller is responsible for verifying
// the model exists (e.g. via ListModels).
func (c *Client) SetModel(model string) {
	c.model = model
}

// SetThinking enables or disables model thinking mode.
func (c *Client) SetThinking(enabled bool) {
	c.thinking = &enabled
}

// requestOptions returns Ollama options, or nil if none are configured.
func (c *Client) requestOptions() map[string]any {
	if c.thinking == nil {
		return nil
	}
	return map[string]any{"enable_thinking": *c.thinking}
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Timeout returns the configured timeout for quick operations.
func (c *Client) Timeout() time.Duration {
	return c.timeout
}

// ListModels fetches the available model IDs from the inference server.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+"/models", nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot reach %s -- start it with `ollama serve`",
			c.baseURL,
		)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, cleanErrorResponse(resp.StatusCode, errBody)
	}

	var models modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("decode model list: %w", err)
	}

	ids := make([]string, len(models.Data))
	for i, m := range models.Data {
		ids[i] = m.ID
	}
	return ids, nil
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
type PullScanner struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
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
		return nil, err
	}
	_ = ps.body.Close()
	return nil, nil // EOF
}

// PullModel initiates a model pull via the Ollama native API. The base URL
// is assumed to be the OpenAI-compatible endpoint (e.g.
// "http://localhost:11434/v1"); this method strips "/v1" to reach the Ollama
// native API at "/api/pull".
func (c *Client) PullModel(ctx context.Context, model string) (*PullScanner, error) {
	// Derive Ollama native base from OpenAI-compatible base.
	ollamaBase := strings.TrimRight(c.baseURL, "/")
	ollamaBase = strings.TrimSuffix(ollamaBase, "/v1")

	body, err := json.Marshal(map[string]string{"name": model})
	if err != nil {
		return nil, fmt.Errorf("marshal pull request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		ollamaBase+"/api/pull",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build pull request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot reach %s -- start it with `ollama serve`",
			ollamaBase,
		)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, cleanErrorResponse(resp.StatusCode, errBody)
	}

	return &PullScanner{
		body:    resp.Body,
		scanner: bufio.NewScanner(resp.Body),
	}, nil
}

// Ping checks whether the API is reachable and the configured model is
// available. Returns a user-friendly error if not.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+"/models", nil,
	)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf(
			"cannot reach %s -- start it with `ollama serve`",
			c.baseURL,
		)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return cleanErrorResponse(resp.StatusCode, errBody)
	}

	var models modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return fmt.Errorf("decode model list: %w", err)
	}

	for _, m := range models.Data {
		// Ollama model names can include :tag suffixes; match on prefix.
		if m.ID == c.model || strings.HasPrefix(m.ID, c.model+":") {
			return nil
		}
	}
	return fmt.Errorf(
		"model %q not found -- pull it with `ollama pull %s`",
		c.model, c.model,
	)
}

// ChatComplete sends a non-streaming chat completion request and returns the
// full response content. Used for structured output like SQL generation where
// the caller needs the complete result before proceeding.
func (c *Client) ChatComplete(
	ctx context.Context,
	messages []Message,
	opts ...ChatOption,
) (string, error) {
	temp := 0.0
	cr := chatRequest{
		Model:       c.model,
		Messages:    messages,
		Stream:      false,
		Temperature: &temp,
		Options:     c.requestOptions(),
	}
	for _, opt := range opts {
		opt(&cr)
	}
	body, err := json.Marshal(cr)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf(
			"cannot reach %s -- start it with `ollama serve`",
			c.baseURL,
		)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", cleanErrorResponse(resp.StatusCode, errBody)
	}

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

// ChatStream sends a chat completion request and returns a channel that emits
// streamed response chunks. The channel closes when the response completes or
// the context is cancelled. Callers must drain the channel.
func (c *Client) ChatStream(
	ctx context.Context,
	messages []Message,
	opts ...ChatOption,
) (<-chan StreamChunk, error) {
	temp := 0.0
	cr := chatRequest{
		Model:       c.model,
		Messages:    messages,
		Stream:      true,
		Temperature: &temp,
		Options:     c.requestOptions(),
	}
	for _, opt := range opts {
		opt(&cr)
	}
	body, err := json.Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot reach %s -- start it with `ollama serve`",
			c.baseURL,
		)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, cleanErrorResponse(resp.StatusCode, errBody)
	}

	ch := make(chan StreamChunk, 16)
	go sseReader(ctx, resp.Body, ch)
	return ch, nil
}

// sseReader reads Server-Sent Events from the response body, parses each
// chunk, and sends it on the channel. Closes the channel and body when done.
func sseReader(ctx context.Context, body io.ReadCloser, ch chan<- StreamChunk) {
	defer close(ch)
	defer func() { _ = body.Close() }()

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()

		// SSE format: "data: {json}" or "data: [DONE]"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		if payload == "[DONE]" {
			select {
			case ch <- StreamChunk{Done: true}:
			case <-ctx.Done():
			}
			return
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			select {
			case ch <- StreamChunk{Err: fmt.Errorf("decode chunk: %w", err)}:
			case <-ctx.Done():
			}
			return
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		content := chunk.Choices[0].Delta.Content
		done := chunk.Choices[0].FinishReason != nil

		select {
		case ch <- StreamChunk{Content: content, Done: done}:
		case <-ctx.Done():
			return
		}

		if done {
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		select {
		case ch <- StreamChunk{Err: fmt.Errorf("read stream: %w", err)}:
		case <-ctx.Done():
		}
	}
}

// cleanErrorResponse tries to extract a human-readable message from an HTTP
// error response. Handles both OpenAI-style {"error": {"message": "..."}} and
// Ollama-style {"error": "..."} responses. Falls back to the raw body if
// parsing fails.
func cleanErrorResponse(statusCode int, body []byte) error {
	// Try OpenAI-style nested error.
	var openAIErr struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &openAIErr); err == nil && openAIErr.Error.Message != "" {
		return fmt.Errorf("server error (%d): %s", statusCode, openAIErr.Error.Message)
	}

	// Try Ollama-style flat error.
	var ollamaErr struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &ollamaErr); err == nil && ollamaErr.Error != "" {
		return fmt.Errorf("server error (%d): %s", statusCode, ollamaErr.Error)
	}

	// Fallback: raw body if it's short and doesn't look like JSON noise.
	bodyStr := string(body)
	if len(bodyStr) < 100 && !strings.Contains(bodyStr, "{") {
		return fmt.Errorf("server error (%d): %s", statusCode, bodyStr)
	}

	// Last resort: generic message without dumping raw JSON.
	return fmt.Errorf("server returned %d", statusCode)
}
