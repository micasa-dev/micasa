// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/micasa-dev/micasa/internal/crypto"
	"golang.org/x/crypto/nacl/secretbox"
)

// maxBlobDownload is the maximum size of an encrypted blob response.
// Plaintext limit (50 MB) + NaCl secretbox overhead (nonce + poly1305 tag).
const maxBlobDownload int64 = 50<<20 + crypto.NonceSize + secretbox.Overhead

// UploadBlob encrypts plaintext with the household key and uploads it
// to the relay. Treats HTTP 409 (blob already exists) as success (dedup).
func (c *Client) UploadBlob(ctx context.Context, householdID, hash string, plaintext []byte) error {
	got := sha256.Sum256(plaintext)
	if hex.EncodeToString(got[:]) != hash {
		return fmt.Errorf(
			"blob hash mismatch: expected %s, got %s",
			hash, hex.EncodeToString(got[:]),
		)
	}
	sealed, err := crypto.Encrypt(c.key, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt blob: %w", err)
	}

	blobURL, err := url.JoinPath(c.baseURL, "blobs", householdID, hash)
	if err != nil {
		return fmt.Errorf("construct blob upload URL: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		blobURL,
		bytes.NewReader(sealed),
	)
	if err != nil {
		return fmt.Errorf("create blob upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("upload blob: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusConflict:
		return nil // 201 = new, 409 = dedup -- both are success
	default:
		body := readErrorBody(resp.Body)
		return fmt.Errorf("upload blob failed (status %d): %s", resp.StatusCode, body)
	}
}

// DownloadBlob fetches an encrypted blob from the relay and decrypts it
// with the household key. The hash parameter is the SHA-256 of the
// plaintext; after decryption the hash is verified client-side.
func (c *Client) DownloadBlob(ctx context.Context, householdID, hash string) ([]byte, error) {
	blobURL, err := url.JoinPath(c.baseURL, "blobs", householdID, hash)
	if err != nil {
		return nil, fmt.Errorf("construct blob download URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create blob download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download blob: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body := readErrorBody(resp.Body)
		return nil, fmt.Errorf("download blob failed (status %d): %s", resp.StatusCode, body)
	}

	sealed, err := io.ReadAll(io.LimitReader(resp.Body, maxBlobDownload+1))
	if err != nil {
		return nil, fmt.Errorf("read blob body: %w", err)
	}
	if int64(len(sealed)) > maxBlobDownload {
		return nil, fmt.Errorf("blob response exceeds maximum size (%d bytes)", maxBlobDownload)
	}

	plaintext, err := crypto.Decrypt(c.key, sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypt blob: %w", err)
	}

	// Verify plaintext integrity: the hash is of the original plaintext,
	// so validation happens client-side after decryption.
	got := sha256.Sum256(plaintext)
	if hex.EncodeToString(got[:]) != hash {
		return nil, fmt.Errorf(
			"blob integrity check failed: expected sha256 %s, got %s",
			hash, hex.EncodeToString(got[:]),
		)
	}
	return plaintext, nil
}

// HasBlob checks whether a blob exists on the relay without downloading it.
func (c *Client) HasBlob(ctx context.Context, householdID, hash string) (bool, error) {
	blobURL, err := url.JoinPath(c.baseURL, "blobs", householdID, hash)
	if err != nil {
		return false, fmt.Errorf("construct blob check URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, blobURL, nil)
	if err != nil {
		return false, fmt.Errorf("create blob check request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("check blob: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("check blob failed (status %d)", resp.StatusCode)
	}
}
