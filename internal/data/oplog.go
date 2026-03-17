// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cpcloud/micasa/internal/uid"
	"gorm.io/gorm"
)

// syncApplyingKey is a context key used to suppress oplog writes when
// applying remote operations. Hooks check for this flag so that incoming
// remote ops are not re-logged and pushed back in an infinite loop.
type syncApplyingKey struct{}

// WithSyncApplying returns a context with the sync-applying flag set.
// Use this when applying remote ops to the local DB.
func WithSyncApplying(ctx context.Context) context.Context {
	return context.WithValue(ctx, syncApplyingKey{}, true)
}

// isSyncApplying checks whether the current GORM transaction was initiated
// by the sync apply path. If true, oplog hooks should not write.
func isSyncApplying(tx *gorm.DB) bool {
	return tx.Statement.Context.Value(syncApplyingKey{}) != nil
}

// syncableTable returns true if the given table should be tracked in the
// oplog. Local-only tables are excluded.
func syncableTable(table string) bool {
	switch table {
	case TableDeletionRecords,
		TableSettings,
		TableChatInputs,
		TableSyncOplogEntries,
		TableSyncDevices:
		return false
	default:
		return true
	}
}

// writeOplogEntry appends an operation to the sync oplog within the given
// GORM transaction. The payload is JSON-serialized from the provided value.
// For documents, callers should use documentOplogPayload to exclude BLOBs.
func writeOplogEntry(tx *gorm.DB, tableName, rowID, opType string, payload any) error {
	if !syncableTable(tableName) {
		return nil
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("oplog marshal %s/%s: %w", tableName, rowID, err)
	}

	now := time.Now()
	entry := SyncOplogEntry{
		ID:        uid.New(),
		TableName: tableName,
		RowID:     rowID,
		OpType:    opType,
		Payload:   string(jsonBytes),
		DeviceID:  cachedDeviceID(tx),
		CreatedAt: now,
		AppliedAt: &now,
	}
	return tx.Create(&entry).Error
}

// writeOplogEntryRaw writes an oplog entry with a pre-serialized JSON string.
func writeOplogEntryRaw(tx *gorm.DB, tableName, rowID, opType, payload string) error {
	if !syncableTable(tableName) {
		return nil
	}

	now := time.Now()
	entry := SyncOplogEntry{
		ID:        uid.New(),
		TableName: tableName,
		RowID:     rowID,
		OpType:    opType,
		Payload:   payload,
		DeviceID:  cachedDeviceID(tx),
		CreatedAt: now,
		AppliedAt: &now,
	}
	return tx.Create(&entry).Error
}

// documentOplogPayload builds a JSON payload for a document oplog entry,
// excluding the BLOB Data field to keep oplog entries small. Includes a
// blob_ref field with the SHA-256 checksum for content-addressed blob sync.
type documentOplogPayload struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	FileName        string     `json:"file_name"`
	Notes           string     `json:"notes,omitempty"`
	EntityKind      string     `json:"entity_kind,omitempty"`
	EntityID        string     `json:"entity_id,omitempty"`
	ExtractedText   string     `json:"extracted_text,omitempty"`
	ExtractData     []byte     `json:"ocr_data,omitempty"`
	ExtractionModel string     `json:"extraction_model,omitempty"`
	ExtractionOps   []byte     `json:"extraction_ops,omitempty"`
	ChecksumSHA256  string     `json:"sha256,omitempty"`
	BlobRef         string     `json:"blob_ref,omitempty"`
	MIMEType        string     `json:"mime_type,omitempty"`
	SizeBytes       int64      `json:"size_bytes,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

func newDocumentOplogPayload(doc Document) documentOplogPayload {
	p := documentOplogPayload{
		ID:              doc.ID,
		Title:           doc.Title,
		FileName:        doc.FileName,
		Notes:           doc.Notes,
		EntityKind:      doc.EntityKind,
		EntityID:        doc.EntityID,
		ExtractedText:   doc.ExtractedText,
		ExtractionModel: doc.ExtractionModel,
		ChecksumSHA256:  doc.ChecksumSHA256,
		BlobRef:         doc.ChecksumSHA256,
		MIMEType:        doc.MIMEType,
		SizeBytes:       doc.SizeBytes,
		CreatedAt:       doc.CreatedAt,
		UpdatedAt:       doc.UpdatedAt,
	}
	if doc.ExtractionOps != nil {
		p.ExtractionOps = doc.ExtractionOps
	}
	if doc.ExtractData != nil {
		p.ExtractData = doc.ExtractData
	}
	if doc.DeletedAt.Valid {
		t := doc.DeletedAt.Time
		p.DeletedAt = &t
	}
	return p
}

// cachedDeviceID returns this device's ID, lazily initializing it on first
// call. The device ID is stored in the sync_devices table (single row).
var deviceID string

func cachedDeviceID(tx *gorm.DB) string {
	if deviceID != "" {
		return deviceID
	}

	var dev SyncDevice
	if err := tx.First(&dev).Error; err == nil {
		deviceID = dev.ID
		return deviceID
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	dev = SyncDevice{
		ID:   uid.New(),
		Name: hostname,
	}
	if err := tx.Create(&dev).Error; err != nil {
		return dev.ID
	}
	deviceID = dev.ID
	return deviceID
}

// ResetCachedDeviceID clears the cached device ID. Used in tests.
func ResetCachedDeviceID() {
	deviceID = ""
}

// DeviceID returns this device's sync device ID, initializing if needed.
func (s *Store) DeviceID() string {
	return cachedDeviceID(s.db)
}

// UnsyncedOps returns all oplog entries that haven't been pushed to the relay.
func (s *Store) UnsyncedOps() ([]SyncOplogEntry, error) {
	var ops []SyncOplogEntry
	err := s.db.Where("synced_at IS NULL").Order("created_at ASC").Find(&ops).Error
	return ops, err
}

// MarkSynced sets synced_at on the given oplog entry IDs.
func (s *Store) MarkSynced(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.db.Model(&SyncOplogEntry{}).
		Where("id IN ?", ids).
		Update("synced_at", time.Now()).Error
}

// OplogEntries returns oplog entries for a specific table and row, ordered
// by creation time. Useful for debugging and testing.
func (s *Store) OplogEntries(tableName, rowID string) ([]SyncOplogEntry, error) {
	var ops []SyncOplogEntry
	err := s.db.Where("table_name = ? AND row_id = ?", tableName, rowID).
		Order("created_at ASC").Find(&ops).Error
	return ops, err
}

// AllOplogEntries returns all oplog entries ordered by creation time.
func (s *Store) AllOplogEntries() ([]SyncOplogEntry, error) {
	var ops []SyncOplogEntry
	err := s.db.Order("created_at ASC").Find(&ops).Error
	return ops, err
}
