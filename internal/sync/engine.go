// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/cpcloud/micasa/internal/data"
)

// Engine performs full pull+push sync cycles against the relay.
// It is safe to call Sync concurrently, though the caller is responsible
// for preventing overlapping cycles if that is undesirable.
type Engine struct {
	store       *data.Store
	client      *Client
	householdID string
}

// SyncResult summarises the outcome of a single Sync cycle.
type SyncResult struct {
	Pulled    int
	Pushed    int
	Conflicts int
	BlobsUp   int
	BlobsDown int
	BlobErrs  int
}

// NewEngine creates a sync engine.
func NewEngine(store *data.Store, client *Client, householdID string) *Engine {
	return &Engine{
		store:       store,
		client:      client,
		householdID: householdID,
	}
}

// Sync performs a full pull-then-push cycle. The context is checked before
// each phase so the caller can cancel mid-sync (e.g., on app shutdown).
func (e *Engine) Sync(ctx context.Context) (SyncResult, error) {
	if err := ctx.Err(); err != nil {
		return SyncResult{}, err
	}

	dev, err := e.store.GetSyncDevice()
	if err != nil {
		return SyncResult{}, fmt.Errorf("read sync device: %w", err)
	}

	pulled, conflicts, err := e.pullAll(ctx, dev.LastSeq)
	if err != nil {
		return SyncResult{}, err
	}

	if err := ctx.Err(); err != nil {
		return SyncResult{}, err
	}

	pushed, pushedOps, err := e.pushAll(ctx)
	if err != nil {
		return SyncResult{}, err
	}

	if err := ctx.Err(); err != nil {
		return SyncResult{}, err
	}

	blobsUp, blobErrs := e.uploadPendingBlobs(ctx, pushedOps)
	blobsDown, fetchErrs := e.fetchPendingBlobs(ctx)

	return SyncResult{
		Pulled:    pulled,
		Pushed:    pushed,
		Conflicts: conflicts,
		BlobsUp:   blobsUp,
		BlobsDown: blobsDown,
		BlobErrs:  blobErrs + fetchErrs,
	}, nil
}

func (e *Engine) pullAll(ctx context.Context, lastSeq int64) (int, int, error) {
	total := 0
	totalConflicts := 0
	seq := lastSeq
	for {
		if err := ctx.Err(); err != nil {
			return total, totalConflicts, err
		}

		result, err := e.client.Pull(seq, 100)
		if err != nil {
			return total, totalConflicts, fmt.Errorf("pull: %w", err)
		}
		if len(result.Ops) == 0 {
			break
		}

		ar := ApplyOps(e.store.GormDB(), result.Ops)
		if len(ar.Errors) > 0 {
			return total, totalConflicts, fmt.Errorf("apply ops: %w", ar.Errors[0])
		}
		total += ar.Applied + ar.Conflicts
		totalConflicts += ar.Conflicts

		// Update last seq from the highest envelope seq.
		// The relay returns ops ordered by seq (ascending), so the max
		// is always the last element, but we scan defensively.
		for _, dop := range result.Ops {
			if dop.Envelope.Seq > seq {
				seq = dop.Envelope.Seq
			}
		}
		if err := e.store.UpdateSyncDevice(map[string]any{
			"last_seq": seq,
		}); err != nil {
			return total, totalConflicts, fmt.Errorf("update last seq: %w", err)
		}

		if !result.HasMore {
			break
		}
	}
	return total, totalConflicts, nil
}

func (e *Engine) pushAll(ctx context.Context) (int, []OpPayload, error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, err
	}

	unsynced, err := e.store.UnsyncedOps()
	if err != nil {
		return 0, nil, fmt.Errorf("load unsynced ops: %w", err)
	}
	if len(unsynced) == 0 {
		return 0, nil, nil
	}

	ops := make([]OpPayload, 0, len(unsynced))
	for _, entry := range unsynced {
		ops = append(ops, OpPayload{
			ID:        entry.ID,
			TableName: entry.TableName,
			RowID:     entry.RowID,
			OpType:    entry.OpType,
			Payload:   entry.Payload,
			DeviceID:  entry.DeviceID,
			CreatedAt: entry.CreatedAt,
		})
	}

	pushResp, err := e.client.Push(ops)
	if err != nil {
		return 0, nil, fmt.Errorf("push: %w", err)
	}

	ids := make([]string, 0, len(pushResp.Confirmed))
	for _, c := range pushResp.Confirmed {
		ids = append(ids, c.ID)
	}
	if err := e.store.MarkSynced(ids); err != nil {
		return 0, nil, fmt.Errorf("mark synced: %w", err)
	}

	return len(pushResp.Confirmed), ops, nil
}

// uploadPendingBlobs uploads blobs for document ops that were just pushed.
// Returns the number of successful uploads and the number of errors.
func (e *Engine) uploadPendingBlobs(ctx context.Context, ops []OpPayload) (int, int) {
	uploaded, errCount := 0, 0
	for _, op := range ops {
		if ctx.Err() != nil {
			return uploaded, errCount
		}
		if op.TableName != data.TableDocuments {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(op.Payload), &payload); err != nil {
			slog.Warn("unmarshal payload for blob upload", "row_id", op.RowID, "error", err)
			errCount++
			continue
		}
		blobRef, _ := payload["blob_ref"].(string)
		if blobRef == "" {
			continue
		}

		exists, err := e.client.HasBlob(e.householdID, blobRef)
		if err != nil {
			slog.Warn("check blob on relay", "blob_ref", blobRef, "error", err)
			errCount++
			continue
		}
		if exists {
			continue
		}

		doc, err := e.store.GetDocument(op.RowID)
		if err != nil {
			slog.Warn("load document for blob upload", "row_id", op.RowID, "error", err)
			errCount++
			continue
		}
		if doc.Data == nil {
			continue
		}

		if err := e.client.UploadBlob(e.householdID, blobRef, doc.Data); err != nil {
			slog.Warn("upload blob", "blob_ref", blobRef, "error", err)
			errCount++
			continue
		}
		uploaded++
	}
	return uploaded, errCount
}

// fetchPendingBlobs downloads blobs for documents that have a checksum but
// no local data (arrived via sync without the blob payload).
// Returns the number of successful fetches and the number of errors.
func (e *Engine) fetchPendingBlobs(ctx context.Context) (int, int) {
	if ctx.Err() != nil {
		return 0, 0
	}

	pending, err := e.store.PendingBlobDocuments()
	if err != nil {
		slog.Warn("query pending blobs", "error", err)
		return 0, 1
	}

	fetched, errCount := 0, 0
	for _, doc := range pending {
		if ctx.Err() != nil {
			return fetched, errCount
		}

		plaintext, err := e.client.DownloadBlob(e.householdID, doc.ChecksumSHA256)
		if err != nil {
			slog.Warn("download blob", "checksum", doc.ChecksumSHA256, "error", err)
			errCount++
			continue
		}
		if err := e.store.UpdateDocumentData(doc.ID, plaintext); err != nil {
			slog.Warn("save blob", "doc_id", doc.ID, "error", err)
			errCount++
			continue
		}
		fetched++
	}
	return fetched, errCount
}
