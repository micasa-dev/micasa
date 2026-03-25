// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import "gorm.io/gorm"

// detachDocumentsAndCleanup handles the shared hard-delete bookkeeping for a
// single entity: detaching linked documents (with oplog entries), and deleting
// the entity's DeletionRecords. It operates within an existing transaction.
//
// Parameters:
//   - tx: active GORM transaction
//   - docEntityKind: the DocumentEntity* constant for the entity (e.g. DocumentEntityIncident)
//   - deletionEntity: the DeletionEntity* constant (e.g. DeletionEntityIncident)
//   - entityID: the entity's primary key
func detachDocumentsAndCleanup(tx *gorm.DB, docEntityKind, deletionEntity, entityID string) error {
	if !isSyncApplying(tx) {
		var docs []Document
		if err := tx.Unscoped().
			Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?", docEntityKind, entityID).
			Find(&docs).Error; err != nil {
			return err
		}
		for i := range docs {
			docs[i].EntityKind = DocumentEntityNone
			docs[i].EntityID = ""
			if err := writeOplogEntry(tx, TableDocuments, docs[i].ID, OpUpdate,
				newDocumentOplogPayload(docs[i])); err != nil {
				return err
			}
		}
	}

	// Persist the detachment (clear entity_kind and entity_id on linked docs).
	if err := tx.Unscoped().Model(&Document{}).
		Where(ColEntityKind+" = ? AND "+ColEntityID+" = ?", docEntityKind, entityID).
		Updates(map[string]any{
			ColEntityKind: DocumentEntityNone,
			ColEntityID:   "",
		}).Error; err != nil {
		return err
	}

	// Remove DeletionRecords for the entity.
	return tx.
		Where(ColEntity+" = ? AND "+ColTargetID+" = ?", deletionEntity, entityID).
		Delete(&DeletionRecord{}).Error
}
