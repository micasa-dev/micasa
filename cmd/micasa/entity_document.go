// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/micasa-dev/micasa/internal/config"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func documentEntityDef() entityDef[data.Document] {
	return entityDef[data.Document]{
		name:        "document",
		singular:    "document",
		tableHeader: "DOCUMENTS",
		cols:        documentCols,
		toMap:       documentToMap,
		list: func(s *data.Store, deleted bool) ([]data.Document, error) {
			return s.ListDocuments(deleted)
		},
		get: func(s *data.Store, id string) (data.Document, error) {
			return s.GetDocumentMetadata(id)
		},
		decodeAndCreate: nil, // overridden in newDocumentCmd
		decodeAndUpdate: documentUpdate,
		del: func(s *data.Store, id string) error {
			return s.DeleteDocument(id)
		},
		restore: func(s *data.Store, id string) error {
			return s.RestoreDocument(id)
		},
		deletedAt: func(d data.Document) gorm.DeletedAt {
			return d.DeletedAt
		},
	}
}

func newDocumentCmd() *cobra.Command {
	def := documentEntityDef()

	cmd := &cobra.Command{
		Use:           def.name,
		Short:         fmt.Sprintf("Manage %ss", def.singular),
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(buildListCmd(def))
	cmd.AddCommand(buildGetCmd(def))
	cmd.AddCommand(buildDocumentAddCmd(def))
	cmd.AddCommand(buildEditCmd(def))
	cmd.AddCommand(buildDeleteCmd(def))
	cmd.AddCommand(buildRestoreCmd(def))

	return cmd
}

func buildDocumentAddCmd(def entityDef[data.Document]) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "add [database-path]",
		Short:         "Add a document",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if err := store.SetMaxDocumentSize(cfg.Documents.MaxFileSize.Bytes()); err != nil {
				return fmt.Errorf("set max document size: %w", err)
			}

			raw, err := readInputData(cmd)
			if err != nil {
				return err
			}

			filePath, _ := cmd.Flags().GetString("file")
			doc, err := documentCreate(store, raw, filePath)
			if err != nil {
				return err
			}
			return encodeJSON(cmd.OutOrStdout(), def.toMap(doc))
		},
	}

	cmd.Flags().String("data", "", "JSON object with field values")
	cmd.Flags().String("data-file", "", "Path to JSON file with field values")
	cmd.Flags().String("file", "", "Path to file to upload")
	return cmd
}

func documentCreate(
	store *data.Store,
	raw json.RawMessage,
	filePath string,
) (data.Document, error) {
	var doc data.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return data.Document{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if doc.EntityKind == "" {
		return data.Document{}, errors.New("entity_kind is required")
	}
	if doc.EntityID == "" {
		return data.Document{}, errors.New("entity_id is required")
	}

	if filePath != "" {
		fileData, err := os.ReadFile(filePath) //nolint:gosec // user-specified file
		if err != nil {
			return data.Document{}, fmt.Errorf("read file: %w", err)
		}
		doc.Data = fileData
		doc.SizeBytes = int64(len(fileData))
		h := sha256.Sum256(fileData)
		doc.ChecksumSHA256 = hex.EncodeToString(h[:])
		doc.MIMEType = http.DetectContentType(fileData)
		doc.FileName = filepath.Base(filePath)
		if doc.Title == "" {
			doc.Title = data.TitleFromFilename(doc.FileName)
		}
	}

	if err := store.CreateDocument(&doc); err != nil {
		return data.Document{}, err
	}
	return store.GetDocumentMetadata(doc.ID)
}

func documentUpdate(
	store *data.Store,
	id string,
	raw json.RawMessage,
) (data.Document, error) {
	existing, err := store.GetDocumentMetadata(id)
	if err != nil {
		return data.Document{}, fmt.Errorf("get document: %w", err)
	}

	fields, err := parseFields(raw)
	if err != nil {
		return data.Document{}, err
	}

	for _, pair := range []struct {
		key string
		dst any
	}{
		{data.ColTitle, &existing.Title},
		{data.ColNotes, &existing.Notes},
	} {
		if err := mergeField(fields, pair.key, pair.dst); err != nil {
			return data.Document{}, err
		}
	}

	if err := store.UpdateDocument(existing); err != nil {
		return data.Document{}, err
	}
	return store.GetDocumentMetadata(id)
}
