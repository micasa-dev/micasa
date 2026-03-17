// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cpcloud/micasa/internal/data"
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
func ApplyOps(db *gorm.DB, ops []DecryptedOp) ApplyResult {
	// Ensure oplog hooks are suppressed for remote op application.
	db = db.WithContext(data.WithSyncApplying(db.Statement.Context))

	var result ApplyResult
	for _, dop := range ops {
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

var errConflictLoss = fmt.Errorf("conflict: remote op lost to local op")

func isConflictLoss(err error) bool {
	return errors.Is(err, errConflictLoss)
}

func applyOne(db *gorm.DB, dop DecryptedOp) error {
	op := dop.Payload

	if !allowedSyncTable(op.TableName) {
		return fmt.Errorf("table %q is not a valid sync target", op.TableName)
	}

	// Check for LWW conflict: does a local unsynced op exist for this row?
	var localOp struct {
		ID        string
		CreatedAt time.Time
		DeviceID  string
	}
	err := db.Table("sync_oplog_entries").
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
			return recordUnappliedOp(db, op)
		}
	}

	// Apply the remote op (atomically with conflict resolution).
	return db.Transaction(func(tx *gorm.DB) error {
		// If remote won the conflict, clear local op's applied_at
		// inside the same transaction for atomicity.
		if localOp.ID != "" {
			if err := tx.Table("sync_oplog_entries").
				Where("id = ?", localOp.ID).
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

func applyInsert(tx *gorm.DB, op OpPayload) error {
	var row map[string]any
	if err := json.Unmarshal([]byte(op.Payload), &row); err != nil {
		return fmt.Errorf("unmarshal insert payload: %w", err)
	}
	return tx.Table(op.TableName).Create(row).Error
}

func applyUpdate(tx *gorm.DB, op OpPayload) error {
	var updates map[string]any
	if err := json.Unmarshal([]byte(op.Payload), &updates); err != nil {
		return fmt.Errorf("unmarshal update payload: %w", err)
	}
	// Remove ID from updates to prevent primary key modification.
	delete(updates, "id")
	delete(updates, "ID")
	return tx.Table(op.TableName).Where("id = ?", op.RowID).Updates(updates).Error
}

func applyDelete(tx *gorm.DB, op OpPayload) error {
	now := time.Now()
	return tx.Table(op.TableName).Where("id = ?", op.RowID).
		Update("deleted_at", now).Error
}

func applyRestore(tx *gorm.DB, op OpPayload) error {
	return tx.Table(op.TableName).Where("id = ?", op.RowID).
		Update("deleted_at", nil).Error
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
