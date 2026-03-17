// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/cpcloud/micasa/internal/crypto"
)

// UploadBlob encrypts plaintext with the household key and uploads it
// to the relay. Treats HTTP 409 (blob already exists) as success (dedup).
func (c *Client) UploadBlob(householdID, hash string, plaintext []byte) error {
	sealed, err := crypto.Encrypt(c.key, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt blob: %w", err)
	}

	url := c.baseURL + "/blobs/" + householdID + "/" + hash
	req, err := http.NewRequest("PUT", url, bytes.NewReader(sealed))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("upload blob: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusConflict:
		return nil // 201 = new, 409 = dedup -- both are success
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload blob failed (status %d): %s", resp.StatusCode, body)
	}
}

// DownloadBlob fetches an encrypted blob from the relay and decrypts it
// with the household key. The hash parameter is the SHA-256 of the
// plaintext; after decryption the hash is verified client-side.
func (c *Client) DownloadBlob(householdID, hash string) ([]byte, error) {
	url := c.baseURL + "/blobs/" + householdID + "/" + hash
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download blob failed (status %d): %s", resp.StatusCode, body)
	}

	sealed, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read blob body: %w", err)
	}

	plaintext, err := crypto.Decrypt(c.key, sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypt blob: %w", err)
	}

	// Verify plaintext integrity: the hash is of the original plaintext,
	// so validation happens client-side after decryption.
	got := sha256.Sum256(plaintext)
	if hex.EncodeToString(got[:]) != hash {
		return nil, fmt.Errorf("blob integrity check failed: sha256 mismatch")
	}
	return plaintext, nil
}

// HasBlob checks whether a blob exists on the relay without downloading it.
func (c *Client) HasBlob(householdID, hash string) (bool, error) {
	url := c.baseURL + "/blobs/" + householdID + "/" + hash
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("check blob: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("check blob failed (status %d)", resp.StatusCode)
	}
}
