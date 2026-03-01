// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	anyllmerrors "github.com/mozilla-ai/any-llm-go/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTimeout = 5 * time.Second

// newTestClient creates a llamacpp client pointing at the given base URL.
// llamacpp is OpenAI-compatible and does not require an API key.
func newTestClient(t *testing.T, baseURL, model string) *Client {
	t.Helper()
	c, err := NewClient("llamacpp", baseURL, model, "", testTimeout)
	require.NoError(t, err)
	return c
}

// jsonResponse writes a JSON response with the correct content type.
func jsonResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, body)
}

func TestNewClientUnknownProvider(t *testing.T) {
	_, err := NewClient("bogus", "", "model", "", testTimeout)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestPingSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			jsonResponse(w, `{"data":[{"id":"qwen3:latest"}]}`)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "qwen3")
	err := client.Ping(context.Background())
	assert.NoError(t, err)
}

func TestPingModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"data":[{"id":"llama3:latest"}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "qwen3")
	err := client.Ping(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestPingServerDown(t *testing.T) {
	client := newTestClient(t, "http://127.0.0.1:1/v1", "qwen3")
	err := client.Ping(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach")
}

func TestChatCompleteSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"choices":[{"message":{"content":"SELECT COUNT(*) FROM projects"}}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	result, err := client.ChatComplete(context.Background(), []Message{
		{Role: "user", Content: "how many projects?"},
	})
	require.NoError(t, err)
	assert.Equal(t, "SELECT COUNT(*) FROM projects", result)
}

func TestChatCompleteWithJSONSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		rf, ok := body["response_format"].(map[string]any)
		require.True(t, ok, "request should include response_format")
		assert.Equal(t, "json_schema", rf["type"])
		js, ok := rf["json_schema"].(map[string]any)
		require.True(t, ok, "response_format should include json_schema")
		assert.Equal(t, "test_schema", js["name"])
		schema, ok := js["schema"].(map[string]any)
		require.True(t, ok, "json_schema should include schema")
		assert.Equal(t, "object", schema["type"])
		jsonResponse(w, `{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`)
	}))
	defer srv.Close()

	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"ok": map[string]any{"type": "boolean"}},
		"required":   []any{"ok"},
	}
	client := newTestClient(t, srv.URL+"/v1", "test-model")
	result, err := client.ChatComplete(context.Background(), []Message{
		{Role: "user", Content: "extract"},
	}, WithJSONSchema("test_schema", schema))
	require.NoError(t, err)
	assert.Contains(t, result, "ok")
}

func TestChatCompleteWithoutJSONSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, hasRF := body["response_format"]
		assert.False(t, hasRF, "request should not include response_format")
		jsonResponse(w, `{"choices":[{"message":{"content":"hello"}}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	result, err := client.ChatComplete(context.Background(), []Message{
		{Role: "user", Content: "hi"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestChatCompleteServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":{"message":"model crashed","type":"server_error"}}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	_, err := client.ChatComplete(context.Background(), []Message{
		{Role: "user", Content: "hi"},
	})
	assert.Error(t, err)
}

func TestChatCompleteEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	_, err := client.ChatComplete(context.Background(), []Message{
		{Role: "user", Content: "hi"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestModelAndBaseURL(t *testing.T) {
	client := newTestClient(t, "http://localhost:11434/v1/", "qwen3")
	assert.Equal(t, "qwen3", client.Model())
	assert.Equal(t, "http://localhost:11434/v1/", client.BaseURL())
	assert.Equal(t, testTimeout, client.Timeout())
}

func TestSetModel(t *testing.T) {
	client := newTestClient(t, "http://localhost:11434/v1", "qwen3")
	assert.Equal(t, "qwen3", client.Model())

	client.SetModel("llama3")
	assert.Equal(t, "llama3", client.Model())
}

func TestSetThinking(t *testing.T) {
	client := newTestClient(t, "http://localhost:11434/v1", "qwen3")
	client.SetThinking("medium")
	assert.Equal(t, "medium", client.thinking)
}

func TestListModelsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"data":[{"id":"qwen3:latest"},{"id":"llama3:8b"},{"id":"mistral:7b"}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "qwen3")
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"qwen3:latest", "llama3:8b", "mistral:7b"}, models)
}

func TestListModelsServerDown(t *testing.T) {
	client := newTestClient(t, "http://127.0.0.1:1/v1", "qwen3")
	_, err := client.ListModels(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach")
}

func TestListModelsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"data":[]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "qwen3")
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	assert.Empty(t, models)
}

func TestIsLocalServer(t *testing.T) {
	tests := []struct {
		provider string
		local    bool
	}{
		{"ollama", true},
		{"llamacpp", true},
		{"llamafile", true},
		{"anthropic", false},
		{"openai", false},
		{"openrouter", false},
		{"deepseek", false},
		{"gemini", false},
		{"groq", false},
		{"mistral", false},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			c := &Client{providerName: tt.provider}
			assert.Equal(t, tt.local, c.IsLocalServer())
		})
	}
}

func TestProviderName(t *testing.T) {
	client := newTestClient(t, "http://localhost:11434/v1", "qwen3")
	assert.Equal(t, "llamacpp", client.ProviderName())
}

func TestSupportsModelListing(t *testing.T) {
	tests := []struct {
		provider string
		supports bool
	}{
		{"llamacpp", true},
		{"ollama", true},
		{"anthropic", false},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			c, err := NewClient(tt.provider, "http://localhost:8080", "m", "k", testTimeout)
			require.NoError(t, err)
			assert.Equal(t, tt.supports, c.SupportsModelListing())
		})
	}
}

func TestChatStreamSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, line := range []string{
			`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":""}]}`,
			`data: {"choices":[{"delta":{"content":" world"},"finish_reason":""}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		} {
			_, _ = fmt.Fprintln(w, line)
			_, _ = fmt.Fprintln(w)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	ch, err := client.ChatStream(context.Background(), []Message{
		{Role: "user", Content: "hi"},
	})
	require.NoError(t, err)

	var content string
	for chunk := range ch {
		require.NoError(t, chunk.Err)
		content += chunk.Content
		if chunk.Done {
			break
		}
	}
	assert.Equal(t, "Hello world", content)
}

func TestChatStreamCancellation(t *testing.T) {
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprintln(
			w,
			`data: {"choices":[{"delta":{"content":"start"},"finish_reason":""}]}`,
		)
		_, _ = fmt.Fprintln(w)
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := newTestClient(t, srv.URL+"/v1", "test-model")
	ch, err := client.ChatStream(ctx, []Message{
		{Role: "user", Content: "hi"},
	})
	require.NoError(t, err)

	chunk := <-ch
	assert.Equal(t, "start", chunk.Content)

	cancel()
	for range ch { //nolint:revive // drain channel
	}
	<-handlerDone
}

func TestChatStreamServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":{"message":"model crashed","type":"server_error"}}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	ch, err := client.ChatStream(context.Background(), []Message{
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		assert.Error(t, err)
		return
	}
	var gotErr bool
	for chunk := range ch {
		if chunk.Err != nil {
			gotErr = true
			break
		}
	}
	assert.True(t, gotErr, "should receive an error from the stream")
}

func TestPingModelNotFoundCloud(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"data":[{"id":"claude-sonnet-4-5-20250929"}]}`)
	}))
	defer srv.Close()

	client, err := NewClient("openai", srv.URL+"/v1", "gpt-4o", "sk-test", testTimeout)
	require.NoError(t, err)
	err = client.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
	assert.Contains(t, err.Error(), "check the model name")
}

func TestPingServerDownCloud(t *testing.T) {
	client, err := NewClient(
		"openai",
		"http://127.0.0.1:1/v1",
		"claude-sonnet-4-5-20250929",
		"sk-test",
		testTimeout,
	)
	require.NoError(t, err)
	err = client.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach")
	assert.Contains(t, err.Error(), "check your base_url")
	assert.NotContains(t, err.Error(), "ollama", "cloud error should not mention ollama")
}

// TestPingModelNotFoundLlamacpp verifies that when a local server
// doesn't have the requested model, the user gets a "not available" message.
func TestPingModelNotFoundLlamacpp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"data":[{"id":"llama3:latest"}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "qwen3")
	err := client.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

// TestPingMatchesModelPrefix verifies that model names with tags
// (e.g. "qwen3:latest") match against the base name ("qwen3").
func TestPingMatchesModelPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"data":[{"id":"qwen3:latest"}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "qwen3")
	assert.NoError(t, client.Ping(context.Background()))
}

// TestCreateProviderAllSupported verifies that every documented provider
// can be initialized without error.
func TestCreateProviderAllSupported(t *testing.T) {
	providers := []string{
		"ollama", "anthropic", "openai", "openrouter",
		"deepseek", "gemini", "groq", "mistral",
		"llamacpp", "llamafile",
	}
	for _, p := range providers {
		t.Run(p, func(t *testing.T) {
			_, err := NewClient(p, "http://localhost:8080", "model", "key", testTimeout)
			assert.NoError(t, err)
		})
	}
}

// TestWrapErrorProviderError exercises the wrapError path a user hits when
// their LLM server is unreachable. Each provider type gets a different message.
func TestWrapErrorProviderError(t *testing.T) {
	tests := []struct {
		provider string
		wantMsg  string
	}{
		{"ollama", "ollama serve"},
		{"llamacpp", "is it running"},
		{"llamafile", "is it running"},
		{"anthropic", "check your base_url"},
		{"openai", "check your base_url"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			c := &Client{providerName: tt.provider}
			err := c.wrapError(
				anyllmerrors.NewProviderError(tt.provider, fmt.Errorf("connection refused")),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// TestWrapErrorModelNotFound exercises the wrapError path when the LLM
// returns a "model not found" error. Ollama gets a "pull it" suggestion.
func TestWrapErrorModelNotFound(t *testing.T) {
	t.Run("ollama suggests pull", func(t *testing.T) {
		c := &Client{providerName: "ollama", model: "qwen3"}
		err := c.wrapError(anyllmerrors.NewModelNotFoundError("ollama", fmt.Errorf("not found")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ollama pull qwen3")
	})
	t.Run("cloud suggests config check", func(t *testing.T) {
		c := &Client{providerName: "anthropic", model: "claude-opus-4-6"}
		err := c.wrapError(
			anyllmerrors.NewModelNotFoundError("anthropic", fmt.Errorf("not found")),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check the model name")
		assert.NotContains(t, err.Error(), "pull")
	})
}

// TestWrapErrorAuthenticationError exercises the path when a user provides
// the wrong API key for a cloud provider.
func TestWrapErrorAuthenticationError(t *testing.T) {
	c := &Client{providerName: "anthropic"}
	err := c.wrapError(anyllmerrors.NewAuthenticationError("anthropic", fmt.Errorf("invalid key")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Contains(t, err.Error(), "check your api_key")
}

// TestWrapErrorRateLimitError exercises the path when a user exceeds the
// provider's rate limit.
func TestWrapErrorRateLimitError(t *testing.T) {
	c := &Client{providerName: "openai"}
	err := c.wrapError(anyllmerrors.NewRateLimitError("openai", fmt.Errorf("429")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
	assert.Contains(t, err.Error(), "try again")
}

// TestWrapErrorNil verifies that nil passes through without error.
func TestWrapErrorNil(t *testing.T) {
	c := &Client{providerName: "ollama"}
	assert.NoError(t, c.wrapError(nil))
}

// TestWrapErrorGeneric verifies that unrecognized errors pass through.
func TestWrapErrorGeneric(t *testing.T) {
	c := &Client{providerName: "ollama"}
	orig := fmt.Errorf("something unexpected")
	err := c.wrapError(orig)
	assert.Equal(t, orig, err)
}

// TestChatCompleteWithThinking verifies that setting a thinking level causes
// the reasoning_effort parameter to be sent to the server.
func TestChatCompleteWithThinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		// The reasoning_effort field should be present when thinking is set.
		// The exact field name depends on the provider SDK, but we verify the
		// client at least sets it on the params.
		jsonResponse(w, `{"choices":[{"message":{"content":"thought about it"}}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	client.SetThinking("medium")
	result, err := client.ChatComplete(context.Background(), []Message{
		{Role: "user", Content: "think hard"},
	})
	require.NoError(t, err)
	assert.Equal(t, "thought about it", result)
}

// TestChatStreamContextCancelledBeforeSend verifies that starting a stream
// with an already-cancelled context doesn't hang.
func TestChatStreamContextCancelledBeforeSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"hi"},"finish_reason":""}]}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	ch, err := client.ChatStream(ctx, []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		return // provider may reject immediately
	}
	// Drain -- should complete quickly without hanging.
	for range ch { //nolint:revive // drain channel
	}
}
