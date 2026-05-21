// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package claudecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/micasa-dev/micasa/internal/llm"
)

// Stream event type names emitted by `claude --output-format stream-json`.
const (
	eventStreamEvent       = "stream_event"
	eventContentBlockDelta = "content_block_delta"
	eventInputJSONDelta    = "input_json_delta"
	eventMessageStop       = "message_stop"
)

// Message role names accepted by validateSingleTurn.
const (
	roleSystem = "system"
	roleUser   = "user"
)

// cmdFactory builds an exec.Cmd for a claude invocation.
type cmdFactory func(ctx context.Context, args ...string) *exec.Cmd

// Client implements llm.Provider by shelling out to the claude CLI binary.
type Client struct {
	model   string
	effort  string
	timeout time.Duration
	makeCmd cmdFactory
}

// Option configures the client.
type Option func(*Client) error

// WithBinPath overrides the default exec.LookPath("claude") resolution.
func WithBinPath(path string) Option {
	return func(c *Client) error {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return fmt.Errorf("claude binary at %s: %w", path, err)
		}
		c.makeCmd = func(ctx context.Context, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, resolved, args...) //nolint:gosec // validated above
		}
		return nil
	}
}

// withCmdFactory replaces the command factory. Used for testing.
func withCmdFactory(f cmdFactory) Option {
	return func(c *Client) error { c.makeCmd = f; return nil }
}

// NewClient creates a claude CLI client.
func NewClient(
	model string,
	timeout time.Duration,
	opts ...Option,
) (*Client, error) {
	if timeout <= 0 {
		return nil, errors.New("claude-cli: timeout must be positive")
	}
	c := &Client{model: model, timeout: timeout}
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, err
		}
	}
	if c.makeCmd == nil {
		resolved, err := exec.LookPath("claude")
		if err != nil {
			return nil, fmt.Errorf("claude CLI not found on PATH: %w", err)
		}
		c.makeCmd = func(ctx context.Context, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, resolved, args...) //nolint:gosec // validated above
		}
	}
	return c, nil
}

var _ llm.ExtractionProvider = (*Client)(nil)

func (c *Client) ProviderName() string         { return "claude-cli" }
func (c *Client) Model() string                { return c.model }
func (c *Client) SetModel(model string)        { c.model = model }
func (c *Client) SetEffort(level string)       { c.effort = level }
func (c *Client) BaseURL() string              { return "" }
func (c *Client) Timeout() time.Duration       { return llm.QuickOpTimeout }
func (c *Client) IsLocalServer() bool          { return false }
func (c *Client) SupportsModelListing() bool   { return false }
func (c *Client) Ping(_ context.Context) error { return nil }

// baseArgs returns the common CLI flags for all invocations.
func (c *Client) baseArgs(outputFormat string) []string {
	return []string{
		"-p",
		"--output-format", outputFormat,
		"--model", c.model,
		"--tools", "",
		"--disable-slash-commands",
		"--no-session-persistence",
		"--no-chrome",
		"--setting-sources", "local",
	}
}

func (c *Client) ListModels(_ context.Context) ([]string, error) {
	return nil, errors.New("claude-cli does not support model listing")
}

// ExtractStream sends a streaming extraction via claude -p with NDJSON output.
// The schema constrains the model's structured output. Supports single-turn
// only (system + user).
func (c *Client) ExtractStream(
	ctx context.Context,
	messages []llm.Message,
	schema map[string]any,
) (<-chan llm.StreamChunk, error) {
	system, user, err := validateSingleTurn(messages)
	if err != nil {
		return nil, err
	}

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("claude-cli: marshal schema: %w", err)
	}

	args := c.baseArgs("stream-json")
	args = append(args, "--verbose", "--include-partial-messages")
	if system != "" {
		args = append(args, "--system-prompt", system)
	}
	args = append(args, "--json-schema", string(schemaJSON))
	args = append(args, c.effortArgs()...)

	// Always enforce c.timeout. If the caller also has a deadline,
	// whichever fires first wins (nested contexts).
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)

	cmd := c.makeCmd(cmdCtx, args...)
	cmd.Stdin = strings.NewReader(user)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("claude-cli: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("claude-cli: start: %w", err)
	}

	out := make(chan llm.StreamChunk, 16)
	go func() {
		defer close(out)
		defer cancel()

		dec := json.NewDecoder(stdoutPipe)
		var completionSeen bool
		var stoppedEarly bool // schema mode: killed subprocess after first turn
		var decErr error      // non-EOF decoder error (malformed JSON)

		for dec.More() {
			var ev streamEvent
			if err := dec.Decode(&ev); err != nil {
				// EOF means the pipe closed (process exited), not
				// malformed JSON. Only record real parse errors.
				if !errors.Is(err, io.EOF) {
					decErr = err
				}
				break
			}

			if ev.Type == eventStreamEvent {
				stop, err := c.handleStreamEvent(
					ctx, &ev, out, &completionSeen,
				)
				if err != nil {
					decErr = err
					break
				}
				if stop {
					stoppedEarly = true
					break
				}
			}
		}

		// Kill subprocess on decoder error or schema early stop
		// to prevent pipe hang.
		if decErr != nil || stoppedEarly {
			cancel()
		}

		waitErr := cmd.Wait()

		// Terminal chunk precedence:
		// 1. Schema early-stop (we killed it, not an error)
		// 2. Timeout/cancellation
		// 3. Process failure
		// 4. Decoder error (process exited 0 but bad JSON)
		// 5. Normal completion
		switch {
		case stoppedEarly && completionSeen:
			out <- llm.StreamChunk{Done: true}
		case decErr == nil && cmdCtx.Err() != nil:
			out <- llm.StreamChunk{Err: cmdCtx.Err()}
		case decErr != nil:
			out <- llm.StreamChunk{Err: fmt.Errorf(
				"claude-cli: malformed stream: %w", decErr,
			)}
		case waitErr != nil:
			out <- llm.StreamChunk{Err: fmt.Errorf(
				"claude-cli: %w: %s", waitErr, stderrBuf.String(),
			)}
		case completionSeen:
			out <- llm.StreamChunk{Done: true}
		default:
			out <- llm.StreamChunk{Err: errors.New(
				"claude-cli: stream ended without completion",
			)}
		}
	}()

	return out, nil
}

// streamEvent is a minimal NDJSON event from claude's stream-json output.
type streamEvent struct {
	Type  string          `json:"type"`
	Event json.RawMessage `json:"event"`
}

type innerEvent struct {
	Type  string          `json:"type"`
	Delta json.RawMessage `json:"delta"`
}

type contentDelta struct {
	Type        string `json:"type"`
	PartialJSON string `json:"partial_json"`
}

func (c *Client) handleStreamEvent(
	ctx context.Context,
	ev *streamEvent,
	out chan<- llm.StreamChunk,
	completionSeen *bool,
) (stop bool, err error) {
	var inner innerEvent
	if err := json.Unmarshal(ev.Event, &inner); err != nil {
		return false, fmt.Errorf("unmarshal stream event: %w", err)
	}

	switch inner.Type {
	case eventContentBlockDelta:
		var delta contentDelta
		if err := json.Unmarshal(inner.Delta, &delta); err != nil {
			return false, fmt.Errorf("unmarshal delta: %w", err)
		}
		// Only forward input_json_delta (structured output).
		// text_delta is the model's prose summary -- ignore it.
		if delta.Type == eventInputJSONDelta && delta.PartialJSON != "" {
			select {
			case out <- llm.StreamChunk{Content: delta.PartialJSON}:
			case <-ctx.Done():
			}
		}
	case eventMessageStop:
		*completionSeen = true
		// The CLI runs a second turn to "process" the tool result.
		// Stop reading -- the structured output from the first turn
		// is complete.
		return true, nil
	}
	return false, nil
}

// validateSingleTurn checks that messages match [system?, user] exactly.
func validateSingleTurn(
	messages []llm.Message,
) (system, user string, err error) {
	if len(messages) == 0 {
		return "", "", errors.New("claude-cli: empty message slice")
	}

	idx := 0
	if messages[0].Role == roleSystem {
		system = messages[0].Content
		idx = 1
	}

	if idx >= len(messages) {
		return "", "", errors.New(
			"claude-cli: single-turn requires exactly one user message",
		)
	}
	if messages[idx].Role != roleUser {
		return "", "", fmt.Errorf(
			"claude-cli: single-turn expected user message, got %q",
			messages[idx].Role,
		)
	}
	user = messages[idx].Content

	if idx+1 < len(messages) {
		return "", "", fmt.Errorf(
			"claude-cli: single-turn received %d messages, "+
				"expected [system?, user]",
			len(messages),
		)
	}

	return system, user, nil
}

// effortArgs returns the --effort flag args based on the effort level.
func (c *Client) effortArgs() []string {
	switch c.effort {
	case "", "none", "auto":
		return nil
	default:
		return []string{"--effort", c.effort}
	}
}
