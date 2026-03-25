// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/dustin/go-humanize"
	"github.com/iancoleman/strcase"

	"gorm.io/gorm"
)

// listDocumentColumns are the columns selected when listing documents to
// avoid loading the potentially large Data BLOB.
var listDocumentColumns = []string{
	ColID, ColTitle, ColFileName, ColEntityKind, ColEntityID,
	ColMIMEType, ColSizeBytes, ColChecksumSHA256, ColExtractionModel,
	ColExtractionOps,
	ColNotes, ColCreatedAt, ColUpdatedAt, ColDeletedAt,
}

// metadataDocumentColumns includes everything listDocumentColumns has
// plus ExtractedText, which callers like afterDocumentSave need for
// extraction decisions.
var metadataDocumentColumns = append(
	append([]string(nil), listDocumentColumns...),
	ColExtractedText,
)

func (s *Store) ListDocuments(includeDeleted bool) ([]Document, error) {
	return listQuery[Document](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Select(listDocumentColumns).Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

// ListDocumentsByEntity returns documents scoped to a specific entity,
// excluding the BLOB data.
func (s *Store) ListDocumentsByEntity(
	entityKind string,
	entityID string,
	includeDeleted bool,
) ([]Document, error) {
	return listQuery[Document](s, includeDeleted, func(db *gorm.DB) *gorm.DB {
		return db.Select(listDocumentColumns).
			Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?", entityKind, entityID).
			Order(ColUpdatedAt + " desc, " + ColID + " desc")
	})
}

// CountDocumentsByEntity counts non-deleted documents grouped by entity_id
// where entity_kind matches. Uses a custom query because documents use
// two-column polymorphic keys that countByFK can't handle.
func (s *Store) CountDocumentsByEntity(
	entityKind string,
	entityIDs []string,
) (map[string]int, error) {
	if len(entityIDs) == 0 {
		return map[string]int{}, nil
	}
	type row struct {
		FK    string `gorm:"column:fk"`
		Count int    `gorm:"column:cnt"`
	}
	var results []row
	err := s.db.Model(&Document{}).
		Select(ColEntityID+" as fk, count(*) as cnt").
		Where(ColEntityKind+" = ? AND "+ColEntityID+" IN ?", entityKind, entityIDs).
		Group(ColEntityID).
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(results))
	for _, r := range results {
		counts[r.FK] = r.Count
	}
	return counts, nil
}

func (s *Store) GetDocument(id string) (Document, error) {
	return getByID[Document](s, id, identity)
}

// GetDocumentMetadata loads a document by ID without the Data BLOB,
// ExtractData, or other heavy columns. Use GetDocument when you need
// the file bytes.
func (s *Store) GetDocumentMetadata(id string) (Document, error) {
	return getByID[Document](s, id, func(db *gorm.DB) *gorm.DB {
		return db.Select(metadataDocumentColumns)
	})
}

// PendingBlobDocuments returns documents that have a SHA-256 checksum
// (meaning they had file data at some point) but currently have no Data
// (blob not yet fetched from the relay). These are candidates for blob
// download during sync pull. Soft-deleted documents are automatically
// excluded by GORM's DeletedAt scoping (Document uses gorm.DeletedAt).
func (s *Store) PendingBlobDocuments() ([]Document, error) {
	var docs []Document
	err := s.db.Select(listDocumentColumns).
		Where("sha256 != '' AND data IS NULL").
		Order("updated_at DESC, id DESC").
		Find(&docs).Error
	return docs, err
}

// UpdateDocumentData sets the Data blob on an existing document by ID.
func (s *Store) UpdateDocumentData(id string, data []byte) error {
	return s.db.Model(&Document{}).Where("id = ?", id).Update("data", data).Error
}

func (s *Store) CreateDocument(doc *Document) error {
	if doc.SizeBytes > 0 &&
		uint64(doc.SizeBytes) > s.maxDocumentSize { //nolint:gosec // SizeBytes is non-negative here
		return fmt.Errorf(
			"file is too large (%s) -- maximum allowed is %s",
			humanize.IBytes(
				uint64(doc.SizeBytes), //nolint:gosec // SizeBytes checked positive above
			),
			humanize.IBytes(s.maxDocumentSize),
		)
	}
	return s.db.Create(doc).Error
}

// UpdateDocument persists changes to a document. Entity linkage (EntityID,
// EntityKind) is always preserved -- callers must use a dedicated method to
// re-link a document. When Data is empty the existing BLOB and file metadata
// columns are also preserved, so metadata-only edits don't erase the file.
func (s *Store) UpdateDocument(doc Document) error {
	omit := []string{ColID, ColCreatedAt, ColDeletedAt}
	if len(doc.Data) == 0 {
		omit = append(omit,
			ColFileName, ColMIMEType, ColSizeBytes,
			ColChecksumSHA256, ColData,
		)
	}
	if err := s.db.Model(&Document{}).Where(ColID+" = ?", doc.ID).
		Select("*").
		Omit(omit...).
		Updates(doc).Error; err != nil {
		return err
	}
	if !isSyncApplying(s.db) {
		// Re-read the full row so the oplog entry contains all fields,
		// not just the caller-provided partial update.
		var full Document
		if err := s.db.First(&full, ColID+" = ?", doc.ID).Error; err != nil {
			return err
		}
		return writeOplogEntry(
			s.db,
			TableDocuments,
			doc.ID,
			OpUpdate,
			newDocumentOplogPayload(full),
		)
	}
	return nil
}

// UpdateDocumentExtraction persists async extraction results on a document
// without touching other fields. Called from the extraction overlay after
// async extraction completes.
func (s *Store) UpdateDocumentExtraction(
	id string,
	text string,
	data []byte,
	model string,
	ops []byte,
) error {
	updates := make(map[string]any)
	if text != "" {
		updates[ColExtractedText] = text
	}
	if len(data) > 0 {
		updates[ColExtractData] = data
	}
	if model != "" {
		updates[ColExtractionModel] = model
	}
	if len(ops) > 0 {
		updates[ColExtractionOps] = ops
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.db.Unscoped().Model(&Document{}).Where(ColID+" = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	// Extraction updates are logged so they sync to other devices.
	if !isSyncApplying(s.db) {
		var doc Document
		if err := s.db.Unscoped().First(&doc, "id = ?", id).Error; err != nil {
			return fmt.Errorf("re-read document for oplog: %w", err)
		}
		return writeOplogEntry(s.db, TableDocuments, id, OpUpdate, newDocumentOplogPayload(doc))
	}
	return nil
}

// EnsureDocumentAlive restores a soft-deleted document if necessary.
// Returns nil if the document is already alive or was successfully restored.
// Returns an error if the document does not exist or restoration fails
// (e.g. parent entity is also deleted).
func (s *Store) EnsureDocumentAlive(id string) error {
	var doc Document
	if err := s.db.First(&doc, "id = ?", id).Error; err == nil {
		return nil // already alive
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("check document %s: %w", id, err)
	}
	return s.RestoreDocument(id)
}

func (s *Store) DeleteDocument(id string) error {
	return s.softDelete(&Document{}, DeletionEntityDocument, id)
}

func (s *Store) RestoreDocument(id string) error {
	var doc Document
	if err := s.db.Unscoped().First(&doc, "id = ?", id).Error; err != nil {
		return err
	}
	if err := s.validateDocumentParent(doc); err != nil {
		return err
	}
	return s.restoreEntity(&Document{}, DeletionEntityDocument, id)
}

// validateDocumentParent checks that the document's parent entity is alive.
func (s *Store) validateDocumentParent(doc Document) error {
	switch doc.EntityKind {
	case DocumentEntityProject:
		if err := s.requireParentAlive(&Project{}, doc.EntityID); err != nil {
			return parentRestoreError("project", err)
		}
	case DocumentEntityAppliance:
		if err := s.requireParentAlive(&Appliance{}, doc.EntityID); err != nil {
			return parentRestoreError("appliance", err)
		}
	case DocumentEntityVendor:
		if err := s.requireParentAlive(&Vendor{}, doc.EntityID); err != nil {
			return parentRestoreError("vendor", err)
		}
	case DocumentEntityQuote:
		if err := s.requireParentAlive(&Quote{}, doc.EntityID); err != nil {
			return parentRestoreError("quote", err)
		}
	case DocumentEntityMaintenance:
		if err := s.requireParentAlive(&MaintenanceItem{}, doc.EntityID); err != nil {
			return parentRestoreError("maintenance item", err)
		}
	case DocumentEntityServiceLog:
		if err := s.requireParentAlive(&ServiceLogEntry{}, doc.EntityID); err != nil {
			return parentRestoreError("service log", err)
		}
	case DocumentEntityIncident:
		if err := s.requireParentAlive(&Incident{}, doc.EntityID); err != nil {
			return parentRestoreError("incident", err)
		}
	}
	return nil
}

// TitleFromFilename derives a human-friendly title from a filename by
// stripping extensions (including compound ones like .tar.gz), splitting on
// word boundaries via strcase, and title-casing each word.
func TitleFromFilename(name string) string {
	name = strings.TrimSpace(name)

	// Always strip the outermost extension (every file has one).
	if ext := filepath.Ext(name); ext != "" && ext != name {
		name = strings.TrimSuffix(name, ext)
	}

	// Continue stripping known compound-extension intermediaries
	// (e.g. .tar in .tar.gz, .tar.bz2, .tar.xz).
	for {
		ext := filepath.Ext(name)
		if ext == "" || ext == name {
			break
		}
		lower := strings.ToLower(ext)
		if lower != ".tar" {
			break
		}
		name = strings.TrimSuffix(name, ext)
	}

	// Split on word boundaries (camelCase, snake_case, kebab, dots).
	name = strcase.ToDelimited(name, ' ')
	name = strings.Join(strings.Fields(name), " ")

	// Title-case each word.
	runes := []rune(name)
	wordStart := true
	for i, r := range runes {
		if unicode.IsSpace(r) {
			wordStart = true
			continue
		}
		if wordStart {
			runes[i] = unicode.ToUpper(r)
		}
		wordStart = false
	}
	return string(runes)
}
