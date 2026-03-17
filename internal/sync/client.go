// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cpcloud/micasa/internal/crypto"
)

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
func (c *Client) Push(ops []OpPayload) (*PushResponse, error) {
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

	req, err := http.NewRequest("POST", c.baseURL+"/sync/push", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("push failed (status %d): %s", resp.StatusCode, respBody)
	}

	var pushResp PushResponse
	if err := json.NewDecoder(resp.Body).Decode(&pushResp); err != nil {
		return nil, fmt.Errorf("decode push response: %w", err)
	}
	return &pushResp, nil
}

// Pull fetches and decrypts remote ops from the relay.
func (c *Client) Pull(afterSeq int64, limit int) (*PullResult, error) {
	url := c.baseURL + "/sync/pull?after=" + strconv.FormatInt(afterSeq, 10)
	if limit > 0 {
		url += "&limit=" + strconv.Itoa(limit)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
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
