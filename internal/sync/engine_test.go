// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/micasa-dev/micasa/internal/crypto"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/relay"
	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/micasa-dev/micasa/internal/uid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupEngineRelay creates a test relay server with a household and returns
// the server, household ID, and a sync client connected to that household.
func setupEngineRelay(
	t *testing.T,
) (*httptest.Server, *relay.MemStore, string, string, crypto.HouseholdKey) {
	t.Helper()

	ms := relay.NewMemStore()
	handler := relay.NewHandler(ms, slog.Default())
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	key, err := crypto.GenerateHouseholdKey()
	require.NoError(t, err)

	resp, err := ms.CreateHousehold(t.Context(), sync.CreateHouseholdRequest{
		DeviceName: "test-device",
		PublicKey:  make([]byte, 32),
	})
	require.NoError(t, err)

	return srv, ms, resp.HouseholdID, resp.DeviceToken, key
}

// openTestStore opens an in-memory SQLite store pre-wired with a SyncDevice
// record pointing at the given relay URL and household.
func openTestStore(t *testing.T, relayURL, householdID, deviceID string) *data.Store {
	t.Helper()

	store, err := data.Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate())
	t.Cleanup(func() { _ = store.Close() })

	// Seed the SyncDevice row directly — UpdateSyncDevice only updates
	// existing rows, so we must create it first before setting fields.
	err = store.GormDB().Create(&data.SyncDevice{
		ID:          deviceID,
		Name:        "test-device",
		HouseholdID: householdID,
		RelayURL:    relayURL,
		LastSeq:     0,
	}).Error
	require.NoError(t, err)

	// Wire the device ID into the oplog cell so hooks write the right ID.
	store.SetDeviceID(deviceID)

	return store
}

// seedRelayOps pushes ops from a second device into the relay so the engine
// under test can pull them.  Returns the number of ops pushed.
func seedRelayOps(
	t *testing.T,
	ms *relay.MemStore,
	srv *httptest.Server,
	householdID string,
	key crypto.HouseholdKey,
	n int,
) int {
	t.Helper()

	// Register a second device to act as the remote sender.
	regResp, err := ms.RegisterDevice(t.Context(), sync.RegisterDeviceRequest{
		HouseholdID: householdID,
		Name:        "remote-device",
		PublicKey:   make([]byte, 32),
	})
	require.NoError(t, err)

	remoteClient := sync.NewClient(srv.URL, regResp.DeviceToken, key)

	ops := make([]sync.OpPayload, n)
	for i := range ops {
		rowID := uid.New()
		payload, err := json.Marshal(map[string]any{
			"id":   rowID,
			"name": "Remote Vendor " + rowID, // unique per ULID
		})
		require.NoError(t, err)
		ops[i] = sync.OpPayload{
			ID:        uid.New(),
			TableName: "vendors",
			RowID:     rowID,
			OpType:    "insert",
			Payload:   string(payload),
			DeviceID:  regResp.DeviceToken,
			CreatedAt: time.Now(),
		}
	}

	pushResp, err := remoteClient.Push(t.Context(), ops)
	require.NoError(t, err)
	return len(pushResp.Confirmed)
}

func TestEngineSyncPullsThenPushes(t *testing.T) {
	t.Parallel()

	srv, ms, householdID, token, key := setupEngineRelay(t)

	// Seed ops from a remote device so pull has something to fetch.
	pulled := seedRelayOps(t, ms, srv, householdID, key, 3)
	require.Equal(t, 3, pulled)

	// Set up local store with a pending op to push.
	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	// Insert a vendor so the oplog gets a local op to push.
	require.NoError(t, store.CreateVendor(&data.Vendor{
		ID:   uid.New(),
		Name: "Local Vendor",
	}))

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	// Should have pulled the 3 remote ops.
	assert.Equal(t, 3, result.Pulled, "expected 3 pulled ops")

	// Should have pushed the 1 local vendor insert.
	assert.Equal(t, 1, result.Pushed, "expected 1 pushed op")

	// No blob activity for vendor ops.
	assert.Zero(t, result.BlobsUp)
	assert.Zero(t, result.BlobsDown)
	assert.Zero(t, result.BlobErrs)
}

func TestEngineSyncCancelledContext(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	_, err := engine.Sync(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEngineSyncReadsLastSeqFromStore(t *testing.T) {
	t.Parallel()

	srv, ms, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	// Seed 5 remote ops.
	seedRelayOps(t, ms, srv, householdID, key, 5)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	// First sync: should pull all 5.
	result1, err := engine.Sync(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 5, result1.Pulled)

	// Second sync: last_seq was updated, so nothing new to pull.
	result2, err := engine.Sync(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 0, result2.Pulled, "second sync should pull nothing (already seen)")
}

func TestEngineSyncEmptyIsNoop(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Zero(t, result.Pulled)
	assert.Zero(t, result.Pushed)
	assert.Zero(t, result.Conflicts)
	assert.Zero(t, result.BlobsUp)
	assert.Zero(t, result.BlobsDown)
	assert.Zero(t, result.BlobErrs)
}

func TestEngineSyncUploadsBlobForDocument(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)
	require.NoError(t, store.SetMaxDocumentSize(1<<20))

	// Create a document with blob data so the oplog entry has blob_ref.
	blobData := []byte("this is the document file content")
	checksum := fmt.Sprintf("%x", sha256.Sum256(blobData))
	docID := uid.New()
	require.NoError(t, store.CreateDocument(&data.Document{
		ID:             docID,
		Title:          "Test Doc",
		FileName:       "test.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(blobData)),
		ChecksumSHA256: checksum,
		Data:           blobData,
	}))

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 1, result.Pushed, "document insert op should be pushed")
	assert.Equal(t, 1, result.BlobsUp, "blob should be uploaded")
	assert.Zero(t, result.BlobErrs)

	// Verify the blob is actually on the relay.
	exists, err := client.HasBlob(t.Context(), householdID, checksum)
	require.NoError(t, err)
	assert.True(t, exists, "blob should exist on relay after upload")
}

func TestEngineSyncSkipsBlobUploadWhenAlreadyOnRelay(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)
	require.NoError(t, store.SetMaxDocumentSize(1<<20))

	blobData := []byte("dedup blob content for engine test")
	checksum := fmt.Sprintf("%x", sha256.Sum256(blobData))

	// Pre-upload the blob so the relay already has it.
	client := sync.NewClient(srv.URL, token, key)
	require.NoError(t, client.UploadBlob(t.Context(), householdID, checksum, blobData))

	// Create the document locally.
	docID := uid.New()
	require.NoError(t, store.CreateDocument(&data.Document{
		ID:             docID,
		Title:          "Already Uploaded",
		FileName:       "dup.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(blobData)),
		ChecksumSHA256: checksum,
		Data:           blobData,
	}))

	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 1, result.Pushed, "op should be pushed")
	assert.Zero(t, result.BlobsUp, "blob already on relay, no upload needed")
	assert.Zero(t, result.BlobErrs)
}

func TestEngineSyncSkipsBlobUploadForDocumentWithoutData(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	blobData := []byte("content that will be nil locally")
	checksum := fmt.Sprintf("%x", sha256.Sum256(blobData))
	docID := uid.New()

	// Insert the document row via raw SQL to bypass GORM AfterCreate hooks.
	// This simulates a metadata-only record (checksum set, Data is NULL).
	require.NoError(t, store.GormDB().Exec(
		"INSERT INTO documents (id, title, file_name, mime_type, size_bytes, sha256) VALUES (?, ?, ?, ?, ?, ?)",
		docID, "Metadata Only", "nodata.pdf", "application/pdf", len(blobData), checksum,
	).
		Error)

	// Create a single oplog entry referencing this document so pushAll
	// picks it up. The payload includes blob_ref but the doc has no Data.
	payload, err := json.Marshal(map[string]any{
		"id":       docID,
		"title":    "Metadata Only",
		"blob_ref": checksum,
	})
	require.NoError(t, err)
	require.NoError(t, store.GormDB().Create(&data.SyncOplogEntry{
		ID:        uid.New(),
		TableName: "documents",
		RowID:     docID,
		OpType:    "insert",
		Payload:   string(payload),
		DeviceID:  deviceID,
		CreatedAt: time.Now(),
	}).Error)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	// The op was pushed but blob upload should be skipped (nil Data).
	assert.Equal(t, 1, result.Pushed)
	assert.Zero(t, result.BlobsUp, "no blob data to upload")

	// fetchPendingBlobs will attempt to download this blob (checksum set,
	// data NULL) but fail because the blob is not on the relay.
	assert.Equal(t, 1, result.BlobErrs, "fetch attempt for missing blob should count as error")
}

func TestEngineSyncSkipsBlobUploadForNonDocumentOps(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	// Create a vendor (non-document entity).
	require.NoError(t, store.CreateVendor(&data.Vendor{
		ID:   uid.New(),
		Name: "Just A Vendor",
	}))

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 1, result.Pushed)
	assert.Zero(t, result.BlobsUp)
	assert.Zero(t, result.BlobErrs)
}

func TestEngineSyncDownloadsPendingBlobs(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	client := sync.NewClient(srv.URL, token, key)

	// Upload a blob to the relay directly.
	blobData := []byte("remote blob content to fetch")
	checksum := fmt.Sprintf("%x", sha256.Sum256(blobData))
	require.NoError(t, client.UploadBlob(t.Context(), householdID, checksum, blobData))

	// Create a local document with a checksum but no data, simulating a
	// document record that arrived via sync without the blob payload.
	docID := uid.New()
	require.NoError(t, store.GormDB().Create(&data.Document{
		ID:             docID,
		Title:          "Remote Doc",
		FileName:       "remote.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(blobData)),
		ChecksumSHA256: checksum,
		Data:           nil,
	}).Error)

	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 1, result.BlobsDown, "should have downloaded 1 blob")
	assert.Zero(t, result.BlobErrs)

	// Verify the local document now has the blob data.
	doc, err := store.GetDocument(docID)
	require.NoError(t, err)
	assert.Equal(t, blobData, doc.Data, "downloaded blob should match original")
}

func TestEngineSyncDownloadsMultiplePendingBlobs(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	client := sync.NewClient(srv.URL, token, key)

	// Upload 3 blobs and create matching local docs without data.
	const n = 3
	docIDs := make([]string, n)
	blobContents := make([][]byte, n)
	for i := range n {
		content := fmt.Appendf(nil, "blob content number %d", i)
		checksum := fmt.Sprintf("%x", sha256.Sum256(content))
		require.NoError(t, client.UploadBlob(t.Context(), householdID, checksum, content))

		docID := uid.New()
		require.NoError(t, store.GormDB().Create(&data.Document{
			ID:             docID,
			Title:          fmt.Sprintf("Doc %d", i),
			FileName:       fmt.Sprintf("file%d.pdf", i),
			MIMEType:       "application/pdf",
			SizeBytes:      int64(len(content)),
			ChecksumSHA256: checksum,
			Data:           nil,
		}).Error)

		docIDs[i] = docID
		blobContents[i] = content
	}

	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Equal(t, n, result.BlobsDown, "should download all pending blobs")
	assert.Zero(t, result.BlobErrs)

	for i, docID := range docIDs {
		doc, err := store.GetDocument(docID)
		require.NoError(t, err)
		assert.Equal(t, blobContents[i], doc.Data)
	}
}

func TestEngineSyncNoPendingBlobsIsNoop(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	// Create a document with both checksum and data (not pending).
	blobData := []byte("already have this blob locally")
	checksum := fmt.Sprintf("%x", sha256.Sum256(blobData))
	require.NoError(t, store.GormDB().Create(&data.Document{
		ID:             uid.New(),
		Title:          "Complete Doc",
		FileName:       "complete.pdf",
		ChecksumSHA256: checksum,
		Data:           blobData,
	}).Error)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Zero(t, result.BlobsDown, "no pending blobs to download")
	assert.Zero(t, result.BlobErrs)
}

func TestEngineSyncBlobUploadAndDownloadSameCycle(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)
	require.NoError(t, store.SetMaxDocumentSize(1<<20))

	client := sync.NewClient(srv.URL, token, key)

	// Scenario: one document has data to upload, another needs a blob download.

	// Document to upload.
	uploadData := []byte("local document blob for upload")
	uploadChecksum := fmt.Sprintf("%x", sha256.Sum256(uploadData))
	require.NoError(t, store.CreateDocument(&data.Document{
		ID:             uid.New(),
		Title:          "Upload Me",
		FileName:       "upload.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(uploadData)),
		ChecksumSHA256: uploadChecksum,
		Data:           uploadData,
	}))

	// Document to download: blob is on relay, local doc has checksum but no data.
	downloadData := []byte("remote blob for download")
	downloadChecksum := fmt.Sprintf("%x", sha256.Sum256(downloadData))
	require.NoError(
		t,
		client.UploadBlob(t.Context(), householdID, downloadChecksum, downloadData),
	)
	downloadDocID := uid.New()
	require.NoError(t, store.GormDB().Create(&data.Document{
		ID:             downloadDocID,
		Title:          "Download Me",
		FileName:       "download.pdf",
		MIMEType:       "application/pdf",
		SizeBytes:      int64(len(downloadData)),
		ChecksumSHA256: downloadChecksum,
		Data:           nil,
	}).Error)

	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 1, result.BlobsUp, "should upload 1 blob")
	assert.Equal(t, 1, result.BlobsDown, "should download 1 blob")
	assert.Zero(t, result.BlobErrs)

	// Verify the uploaded blob is on the relay.
	exists, err := client.HasBlob(t.Context(), householdID, uploadChecksum)
	require.NoError(t, err)
	assert.True(t, exists)

	// Verify the downloaded blob is in the local store.
	doc, err := store.GetDocument(downloadDocID)
	require.NoError(t, err)
	assert.Equal(t, downloadData, doc.Data)
}

func TestEngineSyncBlobDownloadCountsErrorForMissingRemoteBlob(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	// Create a local document with a checksum but no data. The blob is NOT
	// on the relay, so the download should fail and count as an error.
	missingChecksum := fmt.Sprintf("%x", sha256.Sum256([]byte("does not exist on relay")))
	require.NoError(t, store.GormDB().Create(&data.Document{
		ID:             uid.New(),
		Title:          "Missing Blob",
		FileName:       "missing.pdf",
		ChecksumSHA256: missingChecksum,
		Data:           nil,
	}).Error)

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Zero(t, result.BlobsDown, "download should not succeed")
	assert.Equal(t, 1, result.BlobErrs, "failed download should count as error")
}

func TestEngineSyncBlobUploadSkipsDocWithoutBlobRef(t *testing.T) {
	t.Parallel()

	srv, _, householdID, token, key := setupEngineRelay(t)

	deviceID := uid.New()
	store := openTestStore(t, srv.URL, householdID, deviceID)

	// Create a document without file data (no checksum, no blob_ref).
	require.NoError(t, store.CreateDocument(&data.Document{
		ID:    uid.New(),
		Title: "Text-Only Note",
		Notes: "no file attachment",
	}))

	client := sync.NewClient(srv.URL, token, key)
	engine := sync.NewEngine(store, client, householdID)

	result, err := engine.Sync(t.Context())
	require.NoError(t, err)

	assert.Equal(t, 1, result.Pushed, "document insert op should be pushed")
	assert.Zero(t, result.BlobsUp, "no blob_ref means no upload")
	assert.Zero(t, result.BlobErrs)
}
