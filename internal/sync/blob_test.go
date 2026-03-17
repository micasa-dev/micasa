// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync_test

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cpcloud/micasa/internal/crypto"
	"github.com/cpcloud/micasa/internal/relay"
	"github.com/cpcloud/micasa/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBlobTestSetup(t *testing.T) (*sync.Client, string) {
	t.Helper()

	store := relay.NewMemStore()
	handler := relay.NewHandler(store, slog.Default())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Create household.
	resp, err := sync.NewManagementClient(srv.URL, "").CreateHousehold(sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  []byte("fake-public-key-32-bytes-paddin!"),
	})
	require.NoError(t, err)

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	client := sync.NewClient(srv.URL, resp.DeviceToken, key)
	return client, resp.HouseholdID
}

// sha256Hex returns the lowercase hex SHA-256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestBlobRoundTrip(t *testing.T) {
	t.Parallel()
	client, hhID := newBlobTestSetup(t)

	plaintext := []byte("hello world this is a document blob")
	hash := sha256Hex(plaintext)

	// Upload.
	err := client.UploadBlob(hhID, hash, plaintext)
	require.NoError(t, err)

	// Download.
	got, err := client.DownloadBlob(hhID, hash)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestBlobDedupTreatedAsSuccess(t *testing.T) {
	t.Parallel()
	client, hhID := newBlobTestSetup(t)

	plaintext := []byte("dedup test content")
	hash := sha256Hex(plaintext)

	// First upload.
	require.NoError(t, client.UploadBlob(hhID, hash, plaintext))

	// Second upload -- should succeed (409 treated as success).
	require.NoError(t, client.UploadBlob(hhID, hash, plaintext))
}

func TestBlobDownloadNotFound(t *testing.T) {
	t.Parallel()
	client, hhID := newBlobTestSetup(t)

	hash := sha256Hex([]byte("nonexistent"))
	_, err := client.DownloadBlob(hhID, hash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestBlobHasBlob(t *testing.T) {
	t.Parallel()
	client, hhID := newBlobTestSetup(t)

	data := []byte("data")
	hash := sha256Hex(data)

	// Before upload.
	exists, err := client.HasBlob(hhID, hash)
	require.NoError(t, err)
	assert.False(t, exists)

	// After upload.
	require.NoError(t, client.UploadBlob(hhID, hash, data))

	exists, err = client.HasBlob(hhID, hash)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestBlobWrongKeyCannotDecrypt(t *testing.T) {
	t.Parallel()

	store := relay.NewMemStore()
	handler := relay.NewHandler(store, slog.Default())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := sync.NewManagementClient(srv.URL, "").CreateHousehold(sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  []byte("fake-public-key-32-bytes-paddin!"),
	})
	require.NoError(t, err)

	key1, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)
	key2, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	client1 := sync.NewClient(srv.URL, resp.DeviceToken, key1)
	client2 := sync.NewClient(srv.URL, resp.DeviceToken, key2)

	plaintext := []byte("secret document content")
	hash := sha256Hex(plaintext)
	require.NoError(t, client1.UploadBlob(resp.HouseholdID, hash, plaintext))

	// Download with wrong key -- should fail decryption.
	_, err = client2.DownloadBlob(resp.HouseholdID, hash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt")
}

func TestBlobURLHandlesTrailingSlash(t *testing.T) {
	t.Parallel()

	store := relay.NewMemStore()
	handler := relay.NewHandler(store, slog.Default())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := sync.NewManagementClient(srv.URL, "").CreateHousehold(sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  []byte("fake-public-key-32-bytes-paddin!"),
	})
	require.NoError(t, err)

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	// Use trailing slash in base URL.
	client := sync.NewClient(srv.URL+"/", resp.DeviceToken, key)
	plaintext := []byte("trailing slash blob test")
	hash := sha256Hex(plaintext)

	err = client.UploadBlob(resp.HouseholdID, hash, plaintext)
	require.NoError(t, err)

	got, err := client.DownloadBlob(resp.HouseholdID, hash)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)

	exists, err := client.HasBlob(resp.HouseholdID, hash)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestBlobUploadRejectsHashMismatch(t *testing.T) {
	t.Parallel()
	client, hhID := newBlobTestSetup(t)

	plaintext := []byte("integrity test content")
	fakeHash := sha256Hex([]byte("different content"))

	// Upload with a hash that doesn't match the plaintext.
	err := client.UploadBlob(hhID, fakeHash, plaintext)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")
}

func TestBlobDownloadIntegrityCheckFailsOnTamperedContent(t *testing.T) {
	t.Parallel()

	// Encrypt different content under the household key so decryption
	// succeeds but the plaintext hash won't match the requested hash.
	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	realPlaintext := []byte("the real content")
	tamperedPlaintext := []byte("tampered content!")
	realHash := sha256Hex(realPlaintext)

	sealed, err := crypto.Encrypt(key, tamperedPlaintext)
	require.NoError(t, err)

	// Serve the tampered ciphertext for any GET request.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(sealed)
	}))
	t.Cleanup(srv.Close)

	client := sync.NewClient(srv.URL, "fake-token", key)
	_, err = client.DownloadBlob("hh-id", realHash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integrity")
	assert.Contains(t, err.Error(), realHash)
}
