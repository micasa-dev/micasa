// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/micasa-dev/micasa/internal/crypto"
)

// maxErrorBody caps how much of an error response body we read to
// prevent a malicious relay from exhausting client memory.
const maxErrorBody = 4096

// readErrorBody reads up to maxErrorBody bytes from r.
func readErrorBody(r io.Reader) []byte {
	b, _ := io.ReadAll(io.LimitReader(r, maxErrorBody))
	return b
}

// Client talks to the sync relay server.
type Client struct {
	baseURL string
	token   string
	key     crypto.HouseholdKey
	http    *http.Client
}

// NewClient creates a sync client for the given relay URL.
func NewClient(baseURL, token string, key crypto.HouseholdKey) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		key:     key,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewManagementClient creates a client for management-only calls (status,
// invite, devices) that don't need the household encryption key. Push/Pull
// will fail if called on this client since the key is zero-valued.
func NewManagementClient(baseURL, token string) *Client {
	return NewClient(baseURL, token, crypto.HouseholdKey{})
}

// Push encrypts and sends local oplog entries to the relay.
func (c *Client) Push(ctx context.Context, ops []OpPayload) (*PushResponse, error) {
	envelopes := make([]Envelope, 0, len(ops))
	for _, op := range ops {
		plaintext, err := json.Marshal(op)
		if err != nil {
			return nil, fmt.Errorf("marshal op %s: %w", op.ID, err)
		}
		sealed, err := crypto.Encrypt(c.key, plaintext)
		if err != nil {
			return nil, fmt.Errorf("encrypt op %s: %w", op.ID, err)
		}
		// sealed = nonce || ciphertext
		envelopes = append(envelopes, Envelope{
			ID:         op.ID,
			Nonce:      sealed[:crypto.NonceSize],
			Ciphertext: sealed[crypto.NonceSize:],
			CreatedAt:  op.CreatedAt,
		})
	}

	body, err := json.Marshal(PushRequest{Ops: envelopes})
	if err != nil {
		return nil, fmt.Errorf("marshal push request: %w", err)
	}

	pushURL, err := url.JoinPath(c.baseURL, "sync", "push")
	if err != nil {
		return nil, fmt.Errorf("construct push URL: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		pushURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create push request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("push request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody := readErrorBody(resp.Body)
		return nil, fmt.Errorf("push failed (status %d): %s", resp.StatusCode, respBody)
	}

	var pushResp PushResponse
	if err := json.NewDecoder(resp.Body).Decode(&pushResp); err != nil {
		return nil, fmt.Errorf("decode push response: %w", err)
	}
	return &pushResp, nil
}

// Pull fetches and decrypts remote ops from the relay.
func (c *Client) Pull(ctx context.Context, afterSeq int64, limit int) (*PullResult, error) {
	pullURL, err := url.JoinPath(c.baseURL, "sync", "pull")
	if err != nil {
		return nil, fmt.Errorf("construct pull URL: %w", err)
	}
	pullURL += "?after=" + strconv.FormatInt(afterSeq, 10)
	if limit > 0 {
		pullURL += "&limit=" + strconv.Itoa(limit)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create pull request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pull request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody := readErrorBody(resp.Body)
		return nil, fmt.Errorf("pull failed (status %d): %s", resp.StatusCode, respBody)
	}

	var pullResp PullResponse
	if err := json.NewDecoder(resp.Body).Decode(&pullResp); err != nil {
		return nil, fmt.Errorf("decode pull response: %w", err)
	}

	ops := make([]DecryptedOp, 0, len(pullResp.Ops))
	for _, env := range pullResp.Ops {
		// Reconstruct sealed = nonce || ciphertext
		sealed := make([]byte, 0, len(env.Nonce)+len(env.Ciphertext))
		sealed = append(sealed, env.Nonce...)
		sealed = append(sealed, env.Ciphertext...)

		plaintext, err := crypto.Decrypt(c.key, sealed)
		if err != nil {
			return nil, fmt.Errorf("decrypt op %s: %w", env.ID, err)
		}

		var op OpPayload
		if err := json.Unmarshal(plaintext, &op); err != nil {
			return nil, fmt.Errorf("unmarshal op %s: %w", env.ID, err)
		}
		ops = append(ops, DecryptedOp{
			Envelope: env,
			Payload:  op,
		})
	}

	return &PullResult{
		Ops:     ops,
		HasMore: pullResp.HasMore,
	}, nil
}

// OpPayload is the plaintext content of an encrypted sync operation.
// It mirrors the fields of data.SyncOplogEntry relevant for sync.
type OpPayload struct {
	ID        string    `json:"id"`
	TableName string    `json:"table_name"`
	RowID     string    `json:"row_id"`
	OpType    string    `json:"op_type"`
	Payload   string    `json:"payload"`
	DeviceID  string    `json:"device_id"`
	CreatedAt time.Time `json:"created_at"`
}

// DecryptedOp pairs a relay envelope with its decrypted payload.
type DecryptedOp struct {
	Envelope Envelope
	Payload  OpPayload
}

// PullResult holds decrypted ops from a pull request.
type PullResult struct {
	Ops     []DecryptedOp
	HasMore bool
}
