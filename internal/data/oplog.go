// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	gosync "sync"
	"time"

	"github.com/micasa-dev/micasa/internal/uid"
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

// errNoDeviceIDCell is returned when a GORM session lacks the per-Store
// device ID cell in its context. This indicates a programming error:
// the operation was performed on a raw *gorm.DB instead of one obtained
// through a Store.
var errNoDeviceIDCell = errors.New("device ID cell not in context")

// ErrNoSyncDevice is returned when no sync device record exists.
var ErrNoSyncDevice = errors.New("no sync device")

// resolveDeviceID extracts the device ID from the GORM transaction's
// context (set per-Store at Open time).
func resolveDeviceID(tx *gorm.DB) (string, error) {
	cell := deviceIDCellFromContext(tx.Statement.Context)
	if cell == nil {
		return "", errNoDeviceIDCell
	}
	return cell.resolve(tx)
}

// writeOplogEntry appends an operation to the sync oplog within the given
// GORM transaction. The payload is JSON-serialized from the provided value.
// For documents, callers should use documentOplogPayload to exclude BLOBs.
func writeOplogEntry(tx *gorm.DB, tableName, rowID, opType string, payload any) error {
	if !syncableTable(tableName) {
		return nil
	}

	deviceID, err := resolveDeviceID(tx)
	if err != nil {
		return fmt.Errorf("oplog %s/%s: %w", tableName, rowID, err)
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
		DeviceID:  deviceID,
		CreatedAt: now,
		AppliedAt: &now,
	}
	return tx.Create(&entry).Error
}

// writeOplogEntryRaw writes an oplog entry with a pre-serialized JSON string.
func writeOplogEntryRaw(tx *gorm.DB, tableName, rowID, opType string) error {
	if !syncableTable(tableName) {
		return nil
	}

	deviceID, err := resolveDeviceID(tx)
	if err != nil {
		return fmt.Errorf("oplog %s/%s: %w", tableName, rowID, err)
	}

	now := time.Now()
	entry := SyncOplogEntry{
		ID:        uid.New(),
		TableName: tableName,
		RowID:     rowID,
		OpType:    opType,
		Payload:   "{}",
		DeviceID:  deviceID,
		CreatedAt: now,
		AppliedAt: &now,
	}
	return tx.Create(&entry).Error
}

// documentOplogPayload builds a JSON payload for a document oplog entry,
// excluding the BLOB Data field to keep oplog entries small. Includes a
// blob_ref field set to the SHA-256 checksum — this is the relay's
// content-addressed storage key. It equals ChecksumSHA256 today but is
// a separate field so the relay storage scheme can evolve independently
// of the integrity checksum.
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
		MIMEType:        doc.MIMEType,
		SizeBytes:       doc.SizeBytes,
		CreatedAt:       doc.CreatedAt,
		UpdatedAt:       doc.UpdatedAt,
	}
	if doc.ChecksumSHA256 != "" {
		p.BlobRef = doc.ChecksumSHA256
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

// deviceIDCtxKey is a context key for the per-Store device ID cell.
// GORM hooks read the cell from tx.Statement.Context to resolve the
// device ID without a package-level global.
type deviceIDCtxKey struct{}

// withDeviceIDCell returns a context carrying the given cell.
func withDeviceIDCell(ctx context.Context, cell *deviceIDCell) context.Context {
	return context.WithValue(ctx, deviceIDCtxKey{}, cell)
}

// deviceIDCellFromContext retrieves the device ID cell from a context.
func deviceIDCellFromContext(ctx context.Context) *deviceIDCell {
	v, _ := ctx.Value(deviceIDCtxKey{}).(*deviceIDCell)
	return v
}

// deviceIDCell is a per-Store lazy cache for the local device ID.
// Each Store instance gets its own cell, eliminating the previous
// package-level global that leaked state across parallel tests.
type deviceIDCell struct {
	mu    gosync.Mutex
	value string
}

// resolve returns the cached device ID, lazily initializing it by
// querying or creating the sync_devices row via tx.
func (c *deviceIDCell) resolve(tx *gorm.DB) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.value != "" {
		return c.value, nil
	}

	var dev SyncDevice
	err := tx.First(&dev).Error
	if err == nil {
		c.value = dev.ID
		return c.value, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("query sync device: %w", err)
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
		return "", fmt.Errorf("create sync device: %w", err)
	}
	c.value = dev.ID
	return c.value, nil
}

func (c *deviceIDCell) set(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = id
}

// DeviceID returns this device's sync device ID, initializing if needed.
// This is a best-effort accessor for display and comparison purposes only.
// Oplog writes use resolveDeviceID which properly propagates errors.
func (s *Store) DeviceID() string {
	id, err := s.deviceCell.resolve(s.db)
	if err != nil {
		slog.Error("oplog: failed to resolve device ID", "error", err)
		return ""
	}
	return id
}

// SetDeviceID updates the cached device ID. Used by pro init/join
// after the relay assigns a new device ID.
func (s *Store) SetDeviceID(id string) {
	s.deviceCell.set(id)
}

// UnsyncedOps returns all oplog entries that haven't been pushed to the relay.
func (s *Store) UnsyncedOps() ([]SyncOplogEntry, error) {
	var ops []SyncOplogEntry
	err := s.db.Where("synced_at IS NULL").Order("created_at ASC, id ASC").Find(&ops).Error
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
		Order("created_at ASC, id ASC").Find(&ops).Error
	return ops, err
}

// AllOplogEntries returns all oplog entries ordered by creation time.
func (s *Store) AllOplogEntries() ([]SyncOplogEntry, error) {
	var ops []SyncOplogEntry
	err := s.db.Order("created_at ASC, id ASC").Find(&ops).Error
	return ops, err
}

// GetSyncDevice returns the single local sync device record.
// Returns ErrNoSyncDevice when no device has been registered.
func (s *Store) GetSyncDevice() (SyncDevice, error) {
	var dev SyncDevice
	if err := s.db.First(&dev).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SyncDevice{}, ErrNoSyncDevice
		}
		return SyncDevice{}, fmt.Errorf("get sync device: %w", err)
	}
	return dev, nil
}

// UpdateSyncDevice updates the single local sync device record.
// The table is a singleton: cachedDeviceID auto-creates the row on
// first access and no other code path inserts additional rows.
func (s *Store) UpdateSyncDevice(updates map[string]any) error {
	return s.db.Model(&SyncDevice{}).Where("1 = 1").Updates(updates).Error
}

// ConflictLosers returns oplog entries that lost LWW conflict resolution:
// synced from the relay (synced_at IS NOT NULL) but not applied locally
// (applied_at IS NULL). Ordered newest first with deterministic tiebreaker.
func (s *Store) ConflictLosers() ([]SyncOplogEntry, error) {
	var ops []SyncOplogEntry
	err := s.db.Where("applied_at IS NULL AND synced_at IS NOT NULL").
		Order("created_at DESC, id DESC").
		Find(&ops).Error
	return ops, err
}

// UpdateOplogDeviceIDs rewrites all oplog entries that reference oldID
// to use newID. Called during pro init when the relay assigns a new
// device ID that replaces the auto-generated local one.
func (s *Store) UpdateOplogDeviceIDs(oldID, newID string) error {
	if oldID == "" || newID == "" {
		return errors.New("update oplog device IDs: both old and new IDs must be non-empty")
	}
	return s.db.Model(&SyncOplogEntry{}).
		Where("device_id = ?", oldID).
		Update("device_id", newID).Error
}
