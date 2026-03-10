// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	anyllm "github.com/mozilla-ai/any-llm-go"
	anyllmerrors "github.com/mozilla-ai/any-llm-go/errors"
	"github.com/mozilla-ai/any-llm-go/providers/anthropic"
	"github.com/mozilla-ai/any-llm-go/providers/deepseek"
	"github.com/mozilla-ai/any-llm-go/providers/gemini"
	"github.com/mozilla-ai/any-llm-go/providers/groq"
	"github.com/mozilla-ai/any-llm-go/providers/llamacpp"
	"github.com/mozilla-ai/any-llm-go/providers/llamafile"
	"github.com/mozilla-ai/any-llm-go/providers/mistral"
	"github.com/mozilla-ai/any-llm-go/providers/ollama"
	"github.com/mozilla-ai/any-llm-go/providers/openai"
)

// QuickOpTimeout is the context deadline for fast LLM server operations
// (ping, model listing, auto-detect). Not user-configurable.
const QuickOpTimeout = 30 * time.Second

// Client wraps an any-llm-go provider behind a stable API for the rest
// of the application.
type Client struct {
	provider     anyllm.Provider
	providerName string
	baseURL      string
	model        string
	thinking     string // reasoning effort: none|low|medium|high|auto
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

// chatParams holds options that can be modified per-request.
type chatParams struct {
	responseFormat *anyllm.ResponseFormat
	noThinking     bool
}

// ChatOption configures a chat completion request.
type ChatOption func(*chatParams)

// WithJSONSchema constrains the model output to match the given JSON Schema.
func WithJSONSchema(name string, schema map[string]any) ChatOption {
	return func(p *chatParams) {
		p.responseFormat = &anyllm.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &anyllm.JSONSchema{
				Name:   name,
				Schema: schema,
			},
		}
	}
}

// WithNoThinking disables reasoning/thinking for this request, even if the
// client has thinking enabled globally. Useful for structured JSON output
// where thinking tokens would consume the response budget.
func WithNoThinking() ChatOption {
	return func(p *chatParams) {
		p.noThinking = true
	}
}

const providerOllama = "ollama"

// localProviders are providers that run on the user's machine.
var localProviders = map[string]bool{
	providerOllama: true,
	"llamacpp":     true,
	"llamafile":    true,
}

// NewClient creates an LLM client for the named provider. The timeout is the
// inference context deadline for this pipeline. The HTTP client timeout is
// derived as max(timeout, QuickOpTimeout) to ensure quick operations don't
// get killed by a short inference timeout. contextLength sets the Ollama
// context window size (num_ctx); 0 uses the library default.
// Ignored for non-Ollama providers.
func NewClient(
	providerName, baseURL, model, apiKey string,
	timeout time.Duration,
	contextLength int,
) (*Client, error) {
	// Cloud providers should not inherit a local base URL left over from
	// a different provider's config (e.g. Ollama's localhost URL).
	effectiveBase := baseURL
	if !localProviders[providerName] && isLoopbackURL(baseURL) {
		effectiveBase = ""
	}

	httpTimeout := max(timeout, QuickOpTimeout)
	opts := buildOpts(effectiveBase, apiKey, httpTimeout, providerName, contextLength)
	p, err := createProvider(providerName, opts)
	if err != nil {
		return nil, fmt.Errorf("create %s provider: %w", providerName, err)
	}
	return &Client{
		provider:     p,
		providerName: providerName,
		baseURL:      baseURL,
		model:        model,
	}, nil
}

func buildOpts(
	baseURL, apiKey string,
	responseTimeout time.Duration,
	providerName string,
	contextLength int,
) []anyllm.Option {
	// responseTimeout caps a single HTTP request (including streaming body
	// reads). Quick operations enforce tighter deadlines via context.
	httpClient := &http.Client{Timeout: responseTimeout}

	// Ollama: inject num_ctx into API requests when configured.
	// The any-llm-go library hardcodes num_ctx=32000; this transport
	// wrapper overrides it with the user's configured value.
	if providerName == providerOllama && contextLength > 0 {
		httpClient.Transport = &numCtxTransport{
			base:   http.DefaultTransport,
			numCtx: contextLength,
		}
	}

	opts := []anyllm.Option{
		anyllm.WithHTTPClient(httpClient),
	}
	if baseURL != "" {
		opts = append(opts, anyllm.WithBaseURL(baseURL))
	}
	if apiKey != "" {
		opts = append(opts, anyllm.WithAPIKey(apiKey))
	}
	return opts
}

// numCtxTransport wraps an HTTP transport to override the num_ctx option
// in Ollama API chat requests. This is necessary because the any-llm-go
// library hardcodes num_ctx=32000 and doesn't expose a way to override it.
type numCtxTransport struct {
	base   http.RoundTripper
	numCtx int
}

func (t *numCtxTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost && req.Body != nil &&
		strings.HasSuffix(req.URL.Path, "/api/chat") {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err == nil {
			if opts, ok := m["options"].(map[string]any); ok {
				opts["num_ctx"] = t.numCtx
			}
			if modified, err := json.Marshal(m); err == nil {
				body = modified
			}
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("num_ctx transport: %w", err)
	}
	return resp, nil
}

func createProvider(name string, opts []anyllm.Option) (anyllm.Provider, error) {
	var (
		p   anyllm.Provider
		err error
	)
	switch name {
	case providerOllama:
		p, err = ollama.New(opts...)
	case "anthropic":
		p, err = anthropic.New(opts...)
	case "openai", "openrouter":
		p, err = openai.New(opts...)
	case "deepseek":
		p, err = deepseek.New(opts...)
	case "gemini":
		p, err = gemini.New(opts...)
	case "groq":
		p, err = groq.New(opts...)
	case "mistral":
		p, err = mistral.New(opts...)
	case "llamacpp":
		p, err = llamacpp.New(opts...)
	case "llamafile":
		p, err = llamafile.New(opts...)
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	if err != nil {
		return nil, fmt.Errorf("creating %s provider: %w", name, err)
	}
	return p, nil
}

// ProviderName returns the provider identifier (e.g. "ollama", "anthropic").
func (c *Client) ProviderName() string {
	return c.providerName
}

// IsLocalServer returns true for providers that run on the user's machine
// (ollama, llamacpp, llamafile).
func (c *Client) IsLocalServer() bool {
	return localProviders[c.providerName]
}

// Model returns the configured model name.
func (c *Client) Model() string {
	return c.model
}

// SetModel switches the active model.
func (c *Client) SetModel(model string) {
	c.model = model
}

// SetThinking sets the reasoning effort level.
func (c *Client) SetThinking(level string) {
	c.thinking = level
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Timeout returns the deadline for quick operations (ping, model listing).
func (c *Client) Timeout() time.Duration {
	return QuickOpTimeout
}

// SupportsModelListing returns true if the provider implements the
// ModelLister interface. Cloud providers like Anthropic do not.
func (c *Client) SupportsModelListing() bool {
	_, ok := c.provider.(anyllm.ModelLister)
	return ok
}

// toMessages converts internal Message types to any-llm-go Messages.
func toMessages(msgs []Message) []anyllm.Message {
	out := make([]anyllm.Message, len(msgs))
	for i, m := range msgs {
		out[i] = anyllm.Message{Role: m.Role, Content: m.Content}
	}
	return out
}

// completionParams builds a CompletionParams from the client state and options.
func (c *Client) completionParams(messages []Message, opts []ChatOption) anyllm.CompletionParams {
	temp := 0.0
	params := anyllm.CompletionParams{
		Model:       c.model,
		Messages:    toMessages(messages),
		Temperature: &temp,
	}
	var cp chatParams
	for _, opt := range opts {
		opt(&cp)
	}
	if c.thinking != "" && !cp.noThinking {
		params.ReasoningEffort = anyllm.ReasoningEffort(c.thinking)
	}
	if cp.responseFormat != nil {
		params.ResponseFormat = cp.responseFormat
	}
	return params
}

// ListModels fetches the available model IDs. Returns an error if the
// provider does not support model listing.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, QuickOpTimeout)
	defer cancel()

	lister, ok := c.provider.(anyllm.ModelLister)
	if !ok {
		return nil, fmt.Errorf(
			"%s provider does not support listing models",
			c.providerName,
		)
	}

	resp, err := lister.ListModels(ctx)
	if err != nil {
		return nil, c.wrapError(err)
	}

	ids := make([]string, len(resp.Data))
	for i, m := range resp.Data {
		ids[i] = m.ID
	}
	return ids, nil
}

// Ping checks whether the API is reachable and the configured model is
// available. For providers without model listing, it's a no-op.
func (c *Client) Ping(ctx context.Context) error {
	lister, ok := c.provider.(anyllm.ModelLister)
	if !ok {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, QuickOpTimeout)
	defer cancel()

	resp, err := lister.ListModels(ctx)
	if err != nil {
		return c.wrapError(err)
	}

	for _, m := range resp.Data {
		if m.ID == c.model || strings.HasPrefix(m.ID, c.model+":") {
			return nil
		}
	}
	if c.providerName == providerOllama {
		return fmt.Errorf(
			"model %q not found -- pull it with `ollama pull %s`",
			c.model, c.model,
		)
	}
	return fmt.Errorf(
		"model %q not available -- check the model name in your config",
		c.model,
	)
}

// ChatComplete sends a non-streaming chat completion request and returns the
// full response content.
func (c *Client) ChatComplete(
	ctx context.Context,
	messages []Message,
	opts ...ChatOption,
) (string, error) {
	params := c.completionParams(messages, opts)

	resp, err := c.provider.Completion(ctx, params)
	if err != nil {
		return "", c.wrapError(err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return resp.Choices[0].Message.ContentString(), nil
}

// ChatStream sends a streaming chat completion request and returns a channel
// of StreamChunk values. The channel closes when the response completes or
// the context is cancelled. Callers must drain the channel.
func (c *Client) ChatStream(
	ctx context.Context,
	messages []Message,
	opts ...ChatOption,
) (<-chan StreamChunk, error) {
	params := c.completionParams(messages, opts)

	chunks, errs := c.provider.CompletionStream(ctx, params)

	out := make(chan StreamChunk, 16)
	go func() {
		defer close(out)
		for {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					if e, eOK := <-errs; eOK && e != nil {
						select {
						case out <- StreamChunk{Err: c.wrapError(e)}:
						case <-ctx.Done():
						}
					}
					return
				}
				content := ""
				done := false
				if len(chunk.Choices) > 0 {
					content = chunk.Choices[0].Delta.Content
					done = chunk.Choices[0].FinishReason != ""
				}
				select {
				case out <- StreamChunk{Content: content, Done: done}:
				case <-ctx.Done():
					return
				}
				if done {
					return
				}
			case err, ok := <-errs:
				if ok && err != nil {
					select {
					case out <- StreamChunk{Err: c.wrapError(err)}:
					case <-ctx.Done():
					}
				}
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// wrapError converts any-llm-go errors to user-friendly messages.
func (c *Client) wrapError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf(
			"%s timed out -- the server may be overloaded or the model is too slow; "+
				"check timeout settings (llm.timeout, llm.chat.timeout, llm.extraction.timeout) "+
				"or try a smaller model",
			c.providerName,
		)
	}

	var providerErr *anyllmerrors.ProviderError
	if errors.As(err, &providerErr) {
		if isNetworkError(err) {
			if c.providerName == providerOllama {
				return fmt.Errorf(
					"cannot reach ollama -- start it with `ollama serve`",
				)
			}
			if c.IsLocalServer() {
				return fmt.Errorf(
					"cannot reach %s server -- is it running?",
					c.providerName,
				)
			}
			return fmt.Errorf(
				"cannot reach %s -- check your base_url and network",
				c.providerName,
			)
		}
		return fmt.Errorf("%s: %w", c.providerName, providerErr.Err)
	}

	var modelErr *anyllmerrors.ModelNotFoundError
	if errors.As(err, &modelErr) {
		if c.providerName == providerOllama {
			return fmt.Errorf(
				"model %q not found -- pull it with `ollama pull %s`",
				c.model, c.model,
			)
		}
		return fmt.Errorf(
			"model %q not available -- check the model name in your config",
			c.model,
		)
	}

	var authErr *anyllmerrors.AuthenticationError
	if errors.As(err, &authErr) {
		return fmt.Errorf(
			"authentication failed for %s -- check your api_key",
			c.providerName,
		)
	}

	var rateLimitErr *anyllmerrors.RateLimitError
	if errors.As(err, &rateLimitErr) {
		return fmt.Errorf(
			"rate limited by %s -- try again shortly",
			c.providerName,
		)
	}

	return err
}

// isNetworkError reports whether err represents a connection-level failure
// (connection refused, unreachable host) as opposed to an application-level
// error from a server that was reachable. Uses both syscall error matching
// and string fallbacks for cross-platform compatibility (Windows connectex
// errors don't always unwrap to syscall.ECONNREFUSED through provider chains).
func isNetworkError(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "actively refused") {
		return true
	}
	if strings.Contains(msg, "host is unreachable") ||
		strings.Contains(msg, "network is unreachable") {
		return true
	}
	return false
}

// isLoopbackURL returns true if the URL points to a loopback address.
func isLoopbackURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" ||
		host == "127.0.0.1" ||
		host == "::1" ||
		host == "[::1]"
}
