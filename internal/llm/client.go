// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"context"
	"errors"
	"fmt"
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

// ErrPingNotSupported is returned by Ping for providers that don't implement
// model listing (e.g. Anthropic). Callers can check this with errors.Is to
// distinguish "verified OK" from "could not verify".
var ErrPingNotSupported = errors.New("provider does not support ping")

// ErrModelListingNotSupported is returned by ListModels for providers that
// don't implement the ModelLister interface.
var ErrModelListingNotSupported = errors.New("provider does not support listing models")

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
// get killed by a short inference timeout.
func NewClient(
	providerName, baseURL, model, apiKey string,
	timeout time.Duration,
) (*Client, error) {
	// Cloud providers should not inherit a local base URL left over from
	// a different provider's config (e.g. Ollama's localhost URL).
	effectiveBase := baseURL
	if !localProviders[providerName] && isLoopbackURL(baseURL) {
		effectiveBase = ""
	}

	httpTimeout := max(timeout, QuickOpTimeout)
	opts := buildOpts(effectiveBase, apiKey, httpTimeout)
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

func buildOpts(baseURL, apiKey string, responseTimeout time.Duration) []anyllm.Option {
	// responseTimeout caps a single HTTP request (including streaming body
	// reads). Quick operations enforce tighter deadlines via context.
	opts := []anyllm.Option{
		anyllm.WithHTTPClient(&http.Client{Timeout: responseTimeout}),
	}
	if baseURL != "" {
		opts = append(opts, anyllm.WithBaseURL(baseURL))
	}
	if apiKey != "" {
		opts = append(opts, anyllm.WithAPIKey(apiKey))
	}
	return opts
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
	if c.thinking != "" {
		params.ReasoningEffort = anyllm.ReasoningEffort(c.thinking)
	}

	var cp chatParams
	for _, opt := range opts {
		opt(&cp)
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
			"%s: %w",
			c.providerName, ErrModelListingNotSupported,
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
// available. For providers without model listing it returns
// ErrPingNotSupported so callers can distinguish "verified OK" from
// "could not verify".
func (c *Client) Ping(ctx context.Context) error {
	lister, ok := c.provider.(anyllm.ModelLister)
	if !ok {
		return ErrPingNotSupported
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

	if len(resp.Data) == 0 {
		return fmt.Errorf("no models found at %s", c.baseURL)
	}

	return fmt.Errorf("model %q not found at %s", c.model, c.baseURL)
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
		return "", fmt.Errorf(
			"%s returned no response for model %q",
			c.providerName, c.model,
		)
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

	// Network errors and model-not-found get the base URL appended so the
	// user can see which endpoint failed. Everything else passes through
	// as-is -- the upstream library already formats provider + code + detail.
	var providerErr *anyllmerrors.ProviderError
	if errors.As(err, &providerErr) && isNetworkError(err) {
		return fmt.Errorf(
			"cannot reach %s at %s: %w",
			c.providerName, c.baseURL, providerErr.Err,
		)
	}

	var modelErr *anyllmerrors.ModelNotFoundError
	if errors.As(err, &modelErr) {
		return fmt.Errorf("model %q not found at %s: %w", c.model, c.baseURL, err)
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
