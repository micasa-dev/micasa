// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
	"gorm.io/gorm"
)

// ApplyResult tracks the outcome of applying remote ops.
type ApplyResult struct {
	Applied   int
	Conflicts int
	Errors    []error
}

// allowedSyncTable returns true if tableName is a known syncable table.
// Prevents remote ops from targeting metadata or internal tables.
func allowedSyncTable(tableName string) bool {
	switch tableName {
	case data.TableAppliances,
		data.TableDocuments,
		data.TableHouseProfiles,
		data.TableIncidents,
		data.TableMaintenanceCategories,
		data.TableMaintenanceItems,
		data.TableProjectTypes,
		data.TableProjects,
		data.TableQuotes,
		data.TableServiceLogEntries,
		data.TableVendors:
		return true
	default:
		return false
	}
}

// ApplyOps applies decrypted remote operations to the local database.
// Uses LWW conflict resolution when a local unsynced op exists for the
// same (table, row_id). Sets the syncApplying context flag to suppress
// oplog hooks from re-logging applied remote ops.
func ApplyOps(ctx context.Context, db *gorm.DB, ops []DecryptedOp) ApplyResult {
	// Ensure oplog hooks are suppressed for remote op application.
	db = db.WithContext(data.WithSyncApplying(ctx))

	// Copy the slice to avoid mutating the caller's data, then sort by
	// relay seq so ops apply in causal order regardless of how the
	// relay/store returns them.
	sorted := make([]DecryptedOp, len(ops))
	copy(sorted, ops)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Envelope.Seq < sorted[j].Envelope.Seq
	})

	var result ApplyResult
	for _, dop := range sorted {
		if err := applyOne(db, dop); err != nil {
			if isConflictLoss(err) {
				result.Conflicts++
			} else {
				result.Errors = append(result.Errors, err)
			}
		} else {
			result.Applied++
		}
	}
	return result
}

var errConflictLoss = errors.New("conflict: remote op lost to local op")

func isConflictLoss(err error) bool {
	return errors.Is(err, errConflictLoss)
}

func applyOne(db *gorm.DB, dop DecryptedOp) error {
	op := dop.Payload

	if !allowedSyncTable(op.TableName) {
		return fmt.Errorf("table %q is not a valid sync target", op.TableName)
	}

	// Run the entire conflict-check + apply inside a single transaction
	// to avoid a TOCTOU race where a local op could be created between
	// the conflict check and the apply.
	return db.Transaction(func(tx *gorm.DB) error {
		// Check for LWW conflict: does a local unsynced op exist for this row?
		var localOp struct {
			ID        string
			CreatedAt time.Time
			DeviceID  string
		}
		err := tx.Table("sync_oplog_entries").
			Select("id, created_at, device_id").
			Where("table_name = ? AND row_id = ? AND synced_at IS NULL", op.TableName, op.RowID).
			Order("created_at DESC, id DESC").
			Limit(1).
			Scan(&localOp).Error
		if err != nil {
			return fmt.Errorf("check conflict for %s/%s: %w", op.TableName, op.RowID, err)
		}

		// If a local unsynced op exists, apply LWW.
		if localOp.ID != "" {
			if lwwLocalWins(localOp.CreatedAt, localOp.DeviceID, op.CreatedAt, op.DeviceID) {
				// Local wins -- record remote op with applied_at = NULL.
				return recordUnappliedOp(tx, op)
			}
			// Remote won the conflict -- clear applied_at on ALL local
			// unsynced ops for this row, not just the latest. Multiple
			// local ops can exist (e.g. insert then update) and all must
			// be marked unapplied so the row state is consistent.
			if err := tx.Table("sync_oplog_entries").
				Where("table_name = ? AND row_id = ? AND synced_at IS NULL", op.TableName, op.RowID).
				Update("applied_at", nil).Error; err != nil {
				return fmt.Errorf("clear local applied_at: %w", err)
			}
		}

		if err := applyOpToTable(tx, op); err != nil {
			return err
		}
		return recordAppliedOp(tx, op)
	})
}

// lwwLocalWins returns true if the local op should win the conflict.
// Later created_at wins; ties broken by lexicographic device_id.
func lwwLocalWins(
	localTime time.Time,
	localDevice string,
	remoteTime time.Time,
	remoteDevice string,
) bool {
	if localTime.Equal(remoteTime) {
		return localDevice >= remoteDevice
	}
	return localTime.After(remoteTime)
}

func applyOpToTable(tx *gorm.DB, op OpPayload) error {
	switch op.OpType {
	case "insert":
		return applyInsert(tx, op)
	case "update":
		return applyUpdate(tx, op)
	case "delete":
		return applyDelete(tx, op)
	case "restore":
		return applyRestore(tx, op)
	default:
		return fmt.Errorf("unknown op type: %s", op.OpType)
	}
}

// validateInsertPayloadID checks that the payload contains a string "id"
// field matching the op's RowID.
func validateInsertPayloadID(row map[string]any, rowID string) error {
	raw, ok := row["id"]
	if !ok {
		return errors.New("insert payload missing string id field")
	}
	payloadID, ok := raw.(string)
	if !ok {
		return fmt.Errorf("insert payload missing string id field (got %T)", raw)
	}
	if payloadID != rowID {
		return fmt.Errorf(
			"payload id %q does not match op row_id %q",
			payloadID, rowID,
		)
	}
	return nil
}

// applyInsert creates a new row from the remote payload. The payload's
// created_at is intentionally preserved (not stripped) so that records
// maintain their original creation timestamp across devices. This differs
// from applyUpdate which strips created_at to prevent overwrites.
func applyInsert(tx *gorm.DB, op OpPayload) error {
	var row map[string]any
	if err := json.Unmarshal([]byte(op.Payload), &row); err != nil {
		return fmt.Errorf("unmarshal insert payload: %w", err)
	}
	if err := validateInsertPayloadID(row, op.RowID); err != nil {
		return err
	}
	stripNonColumnKeys(op.TableName, row)
	return tx.Table(op.TableName).Create(row).Error
}

func applyUpdate(tx *gorm.DB, op OpPayload) error {
	var updates map[string]any
	if err := json.Unmarshal([]byte(op.Payload), &updates); err != nil {
		return fmt.Errorf("unmarshal update payload: %w", err)
	}
	// All model structs use json:"snake_case" tags, so payload keys are
	// always snake_case. Strip keys that must not be modified by remote ops.
	delete(updates, "id")
	delete(updates, "created_at")
	stripNonColumnKeys(op.TableName, updates)
	result := tx.Table(op.TableName).Where("id = ?", op.RowID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update %s/%s: row not found", op.TableName, op.RowID)
	}
	return nil
}

// stripNonColumnKeys removes payload keys that should not be written to the
// database from remote operations. This includes:
//   - Keys with no corresponding DB column (e.g. blob_ref on documents)
//   - deleted_at: prevents a malicious relay from injecting soft-deletes
//
// created_at and updated_at are intentionally preserved so that records
// maintain their original timestamps across devices.
func stripNonColumnKeys(tableName string, row map[string]any) {
	if tableName == data.TableDocuments {
		delete(row, "blob_ref")
	}
	delete(row, "deleted_at")
}

// applyDelete soft-deletes a row. Uses op.CreatedAt rather than time.Now()
// so that applying the same delete on multiple devices produces identical
// deleted_at values (deterministic convergence). Note: deleted_at reflects
// the op creation time, not the wall-clock deletion time on this device.
// Returns an error if the row doesn't exist. ApplyOps sorts by relay seq
// before calling applyOne, so the corresponding insert op will always have
// been applied first.
func applyDelete(tx *gorm.DB, op OpPayload) error {
	result := tx.Table(op.TableName).Where("id = ?", op.RowID).
		Update("deleted_at", op.CreatedAt)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("delete %s/%s: row not found", op.TableName, op.RowID)
	}
	return nil
}

// applyRestore clears a row's soft-delete. Returns an error if the row
// doesn't exist (see applyDelete comment on causal ordering).
func applyRestore(tx *gorm.DB, op OpPayload) error {
	result := tx.Table(op.TableName).Where("id = ?", op.RowID).
		Update("deleted_at", nil)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("restore %s/%s: row not found", op.TableName, op.RowID)
	}
	return nil
}

func recordAppliedOp(tx *gorm.DB, op OpPayload) error {
	now := time.Now()
	return tx.Table("sync_oplog_entries").Create(map[string]any{
		"id":         op.ID,
		"table_name": op.TableName,
		"row_id":     op.RowID,
		"op_type":    op.OpType,
		"payload":    op.Payload,
		"device_id":  op.DeviceID,
		"created_at": op.CreatedAt,
		"applied_at": now,
		"synced_at":  now,
	}).Error
}

func recordUnappliedOp(db *gorm.DB, op OpPayload) error {
	now := time.Now()
	err := db.Table("sync_oplog_entries").Create(map[string]any{
		"id":         op.ID,
		"table_name": op.TableName,
		"row_id":     op.RowID,
		"op_type":    op.OpType,
		"payload":    op.Payload,
		"device_id":  op.DeviceID,
		"created_at": op.CreatedAt,
		"applied_at": nil,
		"synced_at":  now,
	}).Error
	if err != nil {
		return err
	}
	return errConflictLoss
}
