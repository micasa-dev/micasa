// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Package rlsdb encapsulates database access behind row-level security
// aware transaction wrappers.
//
// The raw *gorm.DB is unexported, making it structurally inaccessible
// from outside this package at compile time. All database queries must
// go through Tx (household-scoped) or WithoutHousehold (explicit bypass).
package rlsdb

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// DB wraps a *gorm.DB and enforces that all queries go through an
// RLS-scoped transaction.
type DB struct {
	raw *gorm.DB
}

// RLSTable specifies a table name and which column holds the household ID
// for row-level security policy creation.
type RLSTable struct {
	Name   string
	Column string
}

// New wraps a *gorm.DB in an RLS-aware wrapper.
func New(db *gorm.DB) *DB {
	return &DB{raw: db}
}

// Tx opens a transaction scoped to the given household.
// All store methods use this as the standard database access path.
func (d *DB) Tx(ctx context.Context, householdID string, fn func(tx *gorm.DB) error) error {
	if householdID == "" {
		return fmt.Errorf("rlsdb.Tx: householdID must not be empty")
	}
	return d.raw.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.household_id', ?, true)", householdID).Error; err != nil {
			return fmt.Errorf("set app.household_id: %w", err)
		}
		return fn(tx)
	})
}

// WithoutHousehold opens a transaction for non-RLS tables only.
//
// This method clears app.household_id to empty string, which the RLS
// policy on ops/blobs treats as NULL (no rows visible). Do NOT use this
// as a way to "skip" household scoping — it is only for methods that
// exclusively query non-RLS tables (devices, households, invites,
// key_exchanges) and have no household ID available.
//
// If you have a household ID but don't trust it, validate it first and
// use Tx — do not bypass scoping. If unsure, stop and ask the user.
//
// SAFETY: Every call site MUST have a // SAFETY: comment explaining why
// no household ID is available. New call sites require user approval.
func (d *DB) WithoutHousehold(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return d.raw.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Clear any stale session-level household_id from connection pooling.
		if err := tx.Exec("SELECT set_config('app.household_id', '', true)").Error; err != nil {
			return fmt.Errorf("clear app.household_id: %w", err)
		}
		return fn(tx)
	})
}

// Migrate runs GORM AutoMigrate inside a transaction with a dummy
// household_id set. This is necessary because GORM introspects existing
// tables, and with FORCE ROW LEVEL SECURITY, those queries trigger the
// current_setting policy. Construction-time only.
func (d *DB) Migrate(models ...any) error {
	return d.raw.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"SELECT set_config('app.household_id', '__migration__', true)",
		).Error; err != nil {
			return fmt.Errorf("set migration household_id: %w", err)
		}
		return tx.AutoMigrate(models...)
	})
}

// InitRLS enables row-level security and creates isolation policies for
// the given tables. Idempotent. Construction-time only.
//
// For each table:
//  1. ENABLE ROW LEVEL SECURITY
//  2. FORCE ROW LEVEL SECURITY (policies apply even to table owner)
//  3. DROP + CREATE policy enforcing column = current_setting('app.household_id')
func (d *DB) InitRLS(tables []RLSTable) error {
	return d.raw.Transaction(func(tx *gorm.DB) error {
		for _, t := range tables {
			stmts := []string{
				fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY", t.Name),
				fmt.Sprintf("ALTER TABLE %s FORCE ROW LEVEL SECURITY", t.Name),
				fmt.Sprintf(
					"DROP POLICY IF EXISTS %s_household_isolation ON %s",
					t.Name, t.Name,
				),
				fmt.Sprintf(
					"CREATE POLICY %s_household_isolation ON %s "+
						"USING (%s = NULLIF(current_setting('app.household_id', true), '')) "+
						"WITH CHECK (%s = NULLIF(current_setting('app.household_id', true), ''))",
					t.Name, t.Name, t.Column, t.Column,
				),
			}
			for _, sql := range stmts {
				if err := tx.Exec(sql).Error; err != nil {
					return fmt.Errorf("init RLS on %s: %w", t.Name, err)
				}
			}
		}
		return nil
	})
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	sqlDB, err := d.raw.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
