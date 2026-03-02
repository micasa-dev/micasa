// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package llm

import (
	"context"
	"errors"
	"fmt"
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

// Client wraps an any-llm-go provider behind a stable API for the rest
// of the application.
type Client struct {
	provider     anyllm.Provider
	providerName string
	baseURL      string
	model        string
	timeout      time.Duration
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

// NewClient creates an LLM client for the named provider. Returns an error
// if the provider cannot be initialized.
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

	opts := buildOpts(effectiveBase, apiKey, timeout)
	p, err := createProvider(providerName, opts)
	if err != nil {
		return nil, fmt.Errorf("create %s provider: %w", providerName, err)
	}
	return &Client{
		provider:     p,
		providerName: providerName,
		baseURL:      baseURL,
		model:        model,
		timeout:      timeout,
	}, nil
}

func buildOpts(baseURL, apiKey string, timeout time.Duration) []anyllm.Option {
	var opts []anyllm.Option
	if baseURL != "" {
		opts = append(opts, anyllm.WithBaseURL(baseURL))
	}
	if apiKey != "" {
		opts = append(opts, anyllm.WithAPIKey(apiKey))
	}
	if timeout > 0 {
		opts = append(opts, anyllm.WithTimeout(timeout))
	}
	return opts
}

func createProvider(name string, opts []anyllm.Option) (anyllm.Provider, error) {
	switch name {
	case providerOllama:
		return ollama.New(opts...)
	case "anthropic":
		return anthropic.New(opts...)
	case "openai", "openrouter":
		return openai.New(opts...)
	case "deepseek":
		return deepseek.New(opts...)
	case "gemini":
		return gemini.New(opts...)
	case "groq":
		return groq.New(opts...)
	case "mistral":
		return mistral.New(opts...)
	case "llamacpp":
		return llamacpp.New(opts...)
	case "llamafile":
		return llamafile.New(opts...)
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
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

// Timeout returns the configured timeout for quick operations.
func (c *Client) Timeout() time.Duration {
	return c.timeout
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
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
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

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
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

	var providerErr *anyllmerrors.ProviderError
	if errors.As(err, &providerErr) {
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
