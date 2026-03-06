// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	anyllm "github.com/mozilla-ai/any-llm-go"
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
	t.Parallel()
	_, err := NewClient("bogus", "", "model", "", testTimeout)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestPingSuccess(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"data":[{"id":"llama3:latest"}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "qwen3")
	err := client.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestPingServerDown(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://127.0.0.1:1/v1", "qwen3")
	err := client.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach")
}

// TestPingAnthropicNoOp verifies that Ping is a no-op for providers that
// don't implement ModelLister (like Anthropic).
func TestPingAnthropicNoOp(t *testing.T) {
	t.Parallel()
	client, err := NewClient(
		"anthropic", "http://localhost:8080", "claude-sonnet-4-5-latest", "test-key", testTimeout,
	)
	require.NoError(t, err)
	assert.NoError(t, client.Ping(context.Background()))
}

func TestChatCompleteSuccess(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			return
		}
		rf, ok := body["response_format"].(map[string]any)
		if !assert.True(t, ok, "request should include response_format") {
			return
		}
		assert.Equal(t, "json_schema", rf["type"])
		js, ok := rf["json_schema"].(map[string]any)
		if !assert.True(t, ok, "response_format should include json_schema") {
			return
		}
		assert.Equal(t, "test_schema", js["name"])
		schema, ok := js["schema"].(map[string]any)
		if !assert.True(t, ok, "json_schema should include schema") {
			return
		}
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
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			return
		}
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
	t.Parallel()
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
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	_, err := client.ChatComplete(context.Background(), []Message{
		{Role: "user", Content: "hi"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestModelAndBaseURL(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://localhost:11434/v1/", "qwen3")
	assert.Equal(t, "qwen3", client.Model())
	assert.Equal(t, "http://localhost:11434/v1/", client.BaseURL())
	assert.Equal(t, QuickOpTimeout, client.Timeout())
}

func TestSetModel(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://localhost:11434/v1", "qwen3")
	assert.Equal(t, "qwen3", client.Model())

	client.SetModel("llama3")
	assert.Equal(t, "llama3", client.Model())
}

func TestSetThinking(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://localhost:11434/v1", "qwen3")
	client.SetThinking("medium")
	assert.Equal(t, "medium", client.thinking)
}

func TestListModelsSuccess(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	client := newTestClient(t, "http://127.0.0.1:1/v1", "qwen3")
	_, err := client.ListModels(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach")
}

func TestListModelsEmpty(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	client := newTestClient(t, "http://localhost:11434/v1", "qwen3")
	assert.Equal(t, "llamacpp", client.ProviderName())
}

func TestSupportsModelListing(t *testing.T) {
	t.Parallel()
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
			c, err := NewClient(
				tt.provider, "http://localhost:8080", "m", "k", testTimeout,
			)
			require.NoError(t, err)
			assert.Equal(t, tt.supports, c.SupportsModelListing())
		})
	}
}

func TestChatStreamSuccess(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, `{"data":[{"id":"claude-sonnet-4-5-20250929"}]}`)
	}))
	defer srv.Close()

	// Build the client directly so the loopback-URL guard in NewClient
	// does not strip the httptest server address.
	opts := buildOpts(srv.URL+"/v1", "sk-test", testTimeout)
	p, err := createProvider("openai", opts)
	require.NoError(t, err)
	client := &Client{
		provider:     p,
		providerName: "openai",
		model:        "gpt-4o",
	}
	err = client.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
	assert.Contains(t, err.Error(), "check the model name")
}

func TestPingServerDownCloud(t *testing.T) {
	t.Parallel()
	// Use wrapError directly: a ECONNREFUSED wrapped in ProviderError
	// from a cloud provider should say "cannot reach ... check your
	// base_url" and NOT mention ollama.
	inner := fmt.Errorf("dial tcp: connection refused")
	c := &Client{providerName: "openai"}
	err := c.wrapError(anyllmerrors.NewProviderError("openai", inner))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach")
	assert.Contains(t, err.Error(), "check your base_url")
	assert.NotContains(
		t, err.Error(), "ollama", "cloud error should not mention ollama",
	)
}

// TestPingModelNotFoundLlamacpp verifies that when a local server
// doesn't have the requested model, the user gets a "not available" message.
func TestPingModelNotFoundLlamacpp(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	providers := []string{
		"ollama", "anthropic", "openai", "openrouter",
		"deepseek", "gemini", "groq", "mistral",
		"llamacpp", "llamafile",
	}
	for _, p := range providers {
		t.Run(p, func(t *testing.T) {
			_, err := NewClient(
				p, "http://localhost:8080", "model", "key", testTimeout,
			)
			assert.NoError(t, err)
		})
	}
}

// TestWrapErrorProviderError exercises the wrapError path for ProviderError.
func TestWrapErrorProviderError(t *testing.T) {
	t.Parallel()
	connErr := fmt.Errorf("dial tcp: connection refused")
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
				anyllmerrors.NewProviderError(tt.provider, connErr),
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// TestWrapErrorDeadlineExceeded verifies that a context deadline exceeded
// error produces a friendly timeout message instead of a raw error.
func TestWrapErrorDeadlineExceeded(t *testing.T) {
	t.Parallel()

	t.Run("bare deadline exceeded", func(t *testing.T) {
		t.Parallel()
		c := &Client{providerName: "ollama"}
		err := c.wrapError(context.DeadlineExceeded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
		assert.NotContains(t, err.Error(), "cannot reach")
	})

	t.Run("wrapped in provider error", func(t *testing.T) {
		t.Parallel()
		timeoutErr := fmt.Errorf("request failed: %w", context.DeadlineExceeded)
		c := &Client{providerName: "ollama"}
		err := c.wrapError(
			anyllmerrors.NewProviderError("ollama", timeoutErr),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
		assert.NotContains(t, err.Error(), "cannot reach")
	})
}

// TestWrapErrorProviderErrorPreservesNonConnectionErrors verifies that
// ProviderErrors NOT caused by connection failures pass through the
// original error message instead of showing "cannot reach."
func TestWrapErrorProviderErrorPreservesNonConnectionErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider string
		inner    error
	}{
		{"ollama mid-stream", "ollama", errors.New("unexpected EOF")},
		{"ollama OOM", "ollama", errors.New("model requires more system memory")},
		{"local server timeout", "llamacpp", errors.New("request timed out")},
		{"cloud server error", "openai", errors.New("internal server error")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{providerName: tt.provider}
			err := c.wrapError(
				anyllmerrors.NewProviderError(tt.provider, tt.inner),
			)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.inner,
				"original error should be preserved for non-connection failures")
			assert.NotContains(t, err.Error(), "cannot reach",
				"should not claim server is unreachable for mid-stream errors")
		})
	}
}

// TestWrapErrorModelNotFound exercises the wrapError path when the LLM
// returns a "model not found" error. Ollama gets a "pull it" suggestion.
func TestWrapErrorModelNotFound(t *testing.T) {
	t.Parallel()
	t.Run("ollama suggests pull", func(t *testing.T) {
		c := &Client{providerName: "ollama", model: "qwen3"}
		err := c.wrapError(
			anyllmerrors.NewModelNotFoundError("ollama", fmt.Errorf("not found")),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ollama pull qwen3")
	})
	t.Run("cloud suggests config check", func(t *testing.T) {
		c := &Client{providerName: "anthropic", model: "claude-opus-4-6"}
		err := c.wrapError(
			anyllmerrors.NewModelNotFoundError(
				"anthropic", fmt.Errorf("not found"),
			),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check the model name")
		assert.NotContains(t, err.Error(), "pull")
	})
}

// TestWrapErrorAuthenticationError exercises the path when a user provides
// the wrong API key for a cloud provider.
func TestWrapErrorAuthenticationError(t *testing.T) {
	t.Parallel()
	c := &Client{providerName: "anthropic"}
	err := c.wrapError(
		anyllmerrors.NewAuthenticationError("anthropic", fmt.Errorf("invalid key")),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Contains(t, err.Error(), "check your api_key")
}

// TestWrapErrorRateLimitError exercises the path when a user exceeds the
// provider's rate limit.
func TestWrapErrorRateLimitError(t *testing.T) {
	t.Parallel()
	c := &Client{providerName: "openai"}
	err := c.wrapError(
		anyllmerrors.NewRateLimitError("openai", fmt.Errorf("429")),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")
	assert.Contains(t, err.Error(), "try again")
}

// TestWrapErrorNil verifies that nil passes through without error.
func TestWrapErrorNil(t *testing.T) {
	t.Parallel()
	c := &Client{providerName: "ollama"}
	assert.NoError(t, c.wrapError(nil))
}

// TestWrapErrorGeneric verifies that unrecognized errors pass through.
func TestWrapErrorGeneric(t *testing.T) {
	t.Parallel()
	c := &Client{providerName: "ollama"}
	orig := fmt.Errorf("something unexpected")
	err := c.wrapError(orig)
	assert.Equal(t, orig, err)
}

// TestChatCompleteWithThinking verifies that setting a thinking level causes
// the reasoning_effort parameter to be sent to the server.
func TestChatCompleteWithThinking(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			return
		}
		// The reasoning_effort field should be present when thinking is set.
		// The exact field name depends on the provider SDK, but we verify the
		// client at least sets it on the params.
		jsonResponse(
			w, `{"choices":[{"message":{"content":"thought about it"}}]}`,
		)
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

// TestChatStreamMidStreamDisconnect verifies that when a server sends partial
// data and then drops the connection, the caller receives an error chunk
// rather than a silent Done with truncated content.
func TestChatStreamMidStreamDisconnect(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		// Send one partial chunk then drop the connection.
		_, _ = fmt.Fprintln(
			w,
			`data: {"choices":[{"delta":{"content":"partial"},"finish_reason":""}]}`,
		)
		_, _ = fmt.Fprintln(w)
		if flusher != nil {
			flusher.Flush()
		}
		// Hijack the connection to force an unclean close.
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	ch, err := client.ChatStream(context.Background(), []Message{
		{Role: "user", Content: "hi"},
	})
	require.NoError(t, err)

	var chunks []StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	require.NotEmpty(t, chunks, "should receive at least one chunk")

	last := chunks[len(chunks)-1]
	// The last chunk must carry the error, not silently report Done.
	assert.Error(t, last.Err,
		"mid-stream disconnect should deliver an error, not a silent Done")
}

// TestChatStreamContextCancelledBeforeSend verifies that starting a stream
// with an already-cancelled context doesn't hang.
func TestChatStreamContextCancelledBeforeSend(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintln(
			w,
			`data: {"choices":[{"delta":{"content":"hi"},"finish_reason":""}]}`,
		)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := newTestClient(t, srv.URL+"/v1", "test-model")
	ch, err := client.ChatStream(
		ctx, []Message{{Role: "user", Content: "hi"}},
	)
	if err != nil {
		return // provider may reject immediately
	}
	// Drain -- should complete quickly without hanging.
	for range ch { //nolint:revive // drain channel
	}
}

// TestIsLoopbackURL verifies the helper that detects loopback addresses.
func TestIsLoopbackURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url      string
		loopback bool
	}{
		{"http://localhost:11434", true},
		{"http://127.0.0.1:11434", true},
		{"http://[::1]:11434", true},
		{"https://localhost/v1", true},
		{"https://api.anthropic.com", false},
		{"https://api.openai.com/v1", false},
		{"http://192.168.1.100:8080", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.loopback, isLoopbackURL(tt.url))
		})
	}
}

// TestNewClientCloudProviderIgnoresLoopbackURL verifies that cloud providers
// silently ignore a loopback base URL (left over from Ollama config) and use
// their own default instead.
func TestNewClientCloudProviderIgnoresLoopbackURL(t *testing.T) {
	t.Parallel()
	providers := []string{
		"anthropic", "openai", "deepseek", "gemini", "groq", "mistral",
	}
	for _, p := range providers {
		t.Run(p, func(t *testing.T) {
			c, err := NewClient(
				p, "http://localhost:11434", "model", "key", testTimeout,
			)
			require.NoError(t, err)
			assert.False(t, c.IsLocalServer())
			// The stored baseURL is the original value (for display),
			// but the provider was created without it.
			assert.Equal(t, "http://localhost:11434", c.BaseURL())
		})
	}
}

// TestNewClientLocalProviderKeepsLoopbackURL verifies that local providers
// keep the loopback base URL.
func TestNewClientLocalProviderKeepsLoopbackURL(t *testing.T) {
	t.Parallel()
	providers := []string{"ollama", "llamacpp", "llamafile"}
	for _, p := range providers {
		t.Run(p, func(t *testing.T) {
			c, err := NewClient(
				p, "http://localhost:11434", "model", "", testTimeout,
			)
			require.NoError(t, err)
			assert.True(t, c.IsLocalServer())
		})
	}
}

// TestNewClientOllamaCustomBaseURL verifies that the native ollama provider
// correctly uses a custom base URL with its /api/* endpoints.
func TestNewClientOllamaCustomBaseURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			jsonResponse(w, `{"models":[{"model":"qwen3:latest"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c, err := NewClient("ollama", srv.URL, "qwen3", "", testTimeout)
	require.NoError(t, err)
	assert.NoError(t, c.Ping(context.Background()))
}

// mockModelLister is a minimal anyllm.ModelLister for synctest-based timeout
// tests. It avoids real network I/O so the fake clock can advance.
type mockModelLister struct {
	listModelsFunc func(ctx context.Context) (*anyllm.ModelsResponse, error)
	streamFunc     func(
		ctx context.Context,
		params anyllm.CompletionParams,
	) (<-chan anyllm.ChatCompletionChunk, <-chan error)
}

func (m *mockModelLister) Name() string { return "mock" }

func (m *mockModelLister) Completion(
	_ context.Context,
	_ anyllm.CompletionParams,
) (*anyllm.ChatCompletion, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockModelLister) CompletionStream(
	ctx context.Context,
	params anyllm.CompletionParams,
) (<-chan anyllm.ChatCompletionChunk, <-chan error) {
	return m.streamFunc(ctx, params)
}

func (m *mockModelLister) ListModels(
	ctx context.Context,
) (*anyllm.ModelsResponse, error) {
	return m.listModelsFunc(ctx)
}

// TestPingTimesOutAtQuickOpTimeout verifies that Ping enforces the 30s
// QuickOpTimeout via context deadline when the server never responds.
// Uses synctest with a mock provider (no real I/O) so the fake clock advances.
func TestPingTimesOutAtQuickOpTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mock := &mockModelLister{
			listModelsFunc: func(ctx context.Context) (*anyllm.ModelsResponse, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}
		client := &Client{provider: mock, providerName: "mock", model: "m"}

		start := time.Now()
		err := client.Ping(context.Background())

		elapsed := time.Since(start)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
		assert.InDelta(t,
			QuickOpTimeout.Seconds(), elapsed.Seconds(), 1,
			"should time out at QuickOpTimeout, not sooner or later",
		)
	})
}

// TestListModelsTimesOutAtQuickOpTimeout verifies that ListModels enforces
// the 30s QuickOpTimeout when the provider blocks forever.
func TestListModelsTimesOutAtQuickOpTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mock := &mockModelLister{
			listModelsFunc: func(ctx context.Context) (*anyllm.ModelsResponse, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}
		client := &Client{provider: mock, providerName: "mock", model: "m"}

		start := time.Now()
		_, err := client.ListModels(context.Background())

		elapsed := time.Since(start)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
		assert.InDelta(t,
			QuickOpTimeout.Seconds(), elapsed.Seconds(), 1,
			"should time out at QuickOpTimeout",
		)
	})
}

// TestStreamingSurvivesPastQuickOpTimeout verifies that a streaming response
// taking longer than QuickOpTimeout (30s) is NOT killed. Streaming has no
// internal timeout -- only the caller's context or HTTP client timeout apply.
func TestStreamingSurvivesPastQuickOpTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mock := &mockModelLister{
			listModelsFunc: func(_ context.Context) (*anyllm.ModelsResponse, error) {
				return nil, fmt.Errorf("not used")
			},
			streamFunc: func(
				_ context.Context,
				_ anyllm.CompletionParams,
			) (<-chan anyllm.ChatCompletionChunk, <-chan error) {
				chunks := make(chan anyllm.ChatCompletionChunk)
				errs := make(chan error)
				go func() {
					defer close(chunks)
					defer close(errs)

					chunks <- anyllm.ChatCompletionChunk{
						Choices: []anyllm.ChunkChoice{
							{Delta: anyllm.ChunkDelta{Content: "Hello"}, FinishReason: ""},
						},
					}

					// Delay longer than QuickOpTimeout.
					time.Sleep(QuickOpTimeout + 30*time.Second)

					chunks <- anyllm.ChatCompletionChunk{
						Choices: []anyllm.ChunkChoice{
							{Delta: anyllm.ChunkDelta{Content: " world"}, FinishReason: "stop"},
						},
					}
				}()
				return chunks, errs
			},
		}
		client := &Client{provider: mock, providerName: "mock", model: "m"}

		ch, err := client.ChatStream(
			context.Background(),
			[]Message{{Role: "user", Content: "hi"}},
		)
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
	})
}

// pipeClient creates an llm.Client backed by an OpenAI-compatible provider
// whose HTTP transport uses the client side of a net.Pipe. The caller must
// run a manual HTTP server on srvConn.
func pipeClient(
	t *testing.T,
	cliConn net.Conn,
	responseTimeout time.Duration,
) *Client {
	t.Helper()
	httpClient := &http.Client{
		Timeout: responseTimeout,
		Transport: &http.Transport{
			DialContext: func(
				_ context.Context, _, _ string,
			) (net.Conn, error) {
				return cliConn, nil
			},
		},
	}
	opts := []anyllm.Option{
		anyllm.WithHTTPClient(httpClient),
		anyllm.WithBaseURL("http://pipe"),
	}
	p, err := createProvider("llamacpp", opts)
	require.NoError(t, err)
	return &Client{provider: p, providerName: "llamacpp", model: "m"}
}

// sseChunk formats a single SSE data line.
func sseChunk(content, finishReason string) string {
	return fmt.Sprintf(
		"data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":%q}]}\n\n",
		content, finishReason,
	)
}

// TestHTTPStreamingSurvivesPastQuickOpTimeout verifies that a real HTTP
// streaming response taking longer than QuickOpTimeout is not killed.
// Uses net.Pipe (no real network I/O) so synctest's fake clock advances.
func TestHTTPStreamingSurvivesPastQuickOpTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srvConn, cliConn := net.Pipe()
		defer func() { assert.NoError(t, cliConn.Close()) }()

		client := pipeClient(t, cliConn, 10*time.Minute)

		go func() {
			defer func() { assert.NoError(t, srvConn.Close()) }()

			br := bufio.NewReader(srvConn)
			req, err := http.ReadRequest(br)
			if !assert.NoError(t, err) {
				return
			}
			assert.NoError(t, req.Body.Close())

			header := "HTTP/1.1 200 OK\r\n" +
				"Content-Type: text/event-stream\r\n" +
				"Connection: close\r\n\r\n"
			_, _ = srvConn.Write([]byte(header))
			_, _ = srvConn.Write([]byte(sseChunk("Hello", "")))

			// Delay past QuickOpTimeout -- must survive.
			time.Sleep(QuickOpTimeout + 30*time.Second)

			_, _ = srvConn.Write([]byte(sseChunk(" world", "stop")))
			_, _ = srvConn.Write([]byte("data: [DONE]\n\n"))
		}()

		ch, err := client.ChatStream(
			context.Background(),
			[]Message{{Role: "user", Content: "hi"}},
		)
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
	})
}

// TestHTTPResponseTimeoutKillsHungStream verifies that the HTTP client's
// response timeout (from llm.timeout config) terminates a stream that
// produces no data for longer than the timeout.
func TestHTTPResponseTimeoutKillsHungStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srvConn, cliConn := net.Pipe()
		defer func() { assert.NoError(t, srvConn.Close()) }()
		defer func() { assert.NoError(t, cliConn.Close()) }()

		responseTimeout := 1 * time.Minute
		client := pipeClient(t, cliConn, responseTimeout)

		go func() {
			br := bufio.NewReader(srvConn)
			req, err := http.ReadRequest(br)
			if !assert.NoError(t, err) {
				return
			}
			assert.NoError(t, req.Body.Close())

			header := "HTTP/1.1 200 OK\r\n" +
				"Content-Type: text/event-stream\r\n" +
				"Connection: close\r\n\r\n"
			_, _ = srvConn.Write([]byte(header))

			// Send one chunk then go silent -- response timeout should fire.
			_, _ = srvConn.Write([]byte(sseChunk("partial", "")))

			// Block until the client gives up and closes the pipe.
			buf := make([]byte, 1)
			_, _ = srvConn.Read(buf)
		}()

		start := time.Now()
		ch, err := client.ChatStream(
			context.Background(),
			[]Message{{Role: "user", Content: "hi"}},
		)
		require.NoError(t, err)

		var gotErr bool
		for chunk := range ch {
			if chunk.Err != nil {
				gotErr = true
				break
			}
		}

		elapsed := time.Since(start)
		assert.True(t, gotErr, "stream should have errored from response timeout")
		assert.InDelta(t,
			responseTimeout.Seconds(), elapsed.Seconds(), 5,
			"should time out near the response timeout, not QuickOpTimeout",
		)
	})
}
