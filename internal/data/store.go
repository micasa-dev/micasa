// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/micasa-dev/micasa/internal/data/sqlite"
	"github.com/micasa-dev/micasa/internal/fake"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/micasa-dev/micasa/internal/safeconv"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db              *gorm.DB
	maxDocumentSize uint64
	currency        locale.Currency
	deviceCell      *deviceIDCell
}

func unscopedPreload(q *gorm.DB) *gorm.DB { return q.Unscoped() }

func identity(db *gorm.DB) *gorm.DB { return db }

func listQuery[T any](
	s *Store,
	includeDeleted bool,
	prepare func(*gorm.DB) *gorm.DB,
) ([]T, error) {
	var items []T
	db := prepare(s.db)
	if includeDeleted {
		db = db.Unscoped()
	}
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func getByID[T any](s *Store, id string, prepare func(*gorm.DB) *gorm.DB) (T, error) {
	var item T
	err := prepare(s.db).First(&item, "id = ?", id).Error
	return item, err
}

// findOrCreate looks up a record (including soft-deleted) using the given
// predicate. If not found, creates it. If found and soft-deleted, restores
// it and marks the DeletionRecord as restored.
func findOrCreate[T any](
	tx *gorm.DB,
	item T,
	name string,
	nameLabel string,
	lookup func(*gorm.DB) *gorm.DB,
	entity string,
	id func(T) string,
	deleted func(T) bool,
) (T, error) {
	if strings.TrimSpace(name) == "" {
		var zero T
		return zero, fmt.Errorf("%s is required", nameLabel)
	}
	var existing T
	err := lookup(tx.Unscoped()).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Create(&item).Error; err != nil {
			var zero T
			return zero, err
		}
		return item, nil
	}
	if err != nil {
		var zero T
		return zero, err
	}
	if deleted(existing) {
		if err := restoreSoftDeleted(tx, new(T), entity, id(existing)); err != nil {
			var zero T
			return zero, err
		}
	}
	return existing, nil
}

type dependencyCheck struct {
	model  any
	fkCol  string
	errFmt string
}

func (s *Store) checkDependencies(id string, checks []dependencyCheck) error {
	for _, c := range checks {
		n, err := s.countDependents(c.model, c.fkCol, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf(c.errFmt, n)
		}
	}
	return nil
}

func Open(path string) (*Store, error) {
	if err := ValidateDBPath(path); err != nil {
		return nil, err
	}
	db, err := gorm.Open(
		sqlite.Open(path,
			"PRAGMA foreign_keys = ON",
			"PRAGMA journal_mode = WAL",
			"PRAGMA synchronous = NORMAL",
			"PRAGMA busy_timeout = 5000",
		),
		&gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	cell := &deviceIDCell{}
	db = db.WithContext(withDeviceIDCell(db.Statement.Context, cell))
	return &Store{db: db, deviceCell: cell}, nil
}

// GormDB returns the underlying *gorm.DB for use by sync.ApplyOps,
// which needs direct GORM access to apply remote operations generically.
func (s *Store) GormDB() *gorm.DB {
	return s.db
}

// MaxDocumentSize returns the configured maximum file size for document imports.
func (s *Store) MaxDocumentSize() uint64 {
	return s.maxDocumentSize
}

// SetMaxDocumentSize overrides the maximum allowed file size for document
// imports. The value must be positive; zero is rejected.
func (s *Store) SetMaxDocumentSize(n uint64) error {
	if n == 0 {
		return fmt.Errorf("max document size must be positive, got 0")
	}
	s.maxDocumentSize = n
	return nil
}

// Currency returns the resolved currency for this store.
func (s *Store) Currency() locale.Currency {
	return s.currency
}

// SetCurrency directly sets the cached currency without touching the database.
// Intended for tests that need a currency but don't require full resolution.
func (s *Store) SetCurrency(cur locale.Currency) {
	s.currency = cur
}

// ResolveCurrency determines the currency to use. The database value is
// authoritative for the currency CODE; if unset, resolves from
// configured/env/locale and persists the code for portability. The
// formatting locale is always detected from the environment (never
// persisted) -- like displaying a UTC timestamp in the local timezone.
func (s *Store) ResolveCurrency(configured string) error {
	tag := locale.DetectLocale()
	code, err := s.GetCurrency()
	if err != nil {
		return fmt.Errorf("read currency from database: %w", err)
	}
	if code != "" {
		cur, err := locale.Resolve(code, tag)
		if err != nil {
			return err
		}
		s.currency = cur
		return nil
	}
	cur, err := locale.ResolveDefault(configured)
	if err != nil {
		return err
	}
	if err := s.PutCurrency(cur.Code()); err != nil {
		return fmt.Errorf("persist currency to database: %w", err)
	}
	s.currency = cur
	return nil
}

// ExpandHome replaces a leading "~" or "~/" with the current user's home
// directory. Other forms like "~user/" are left as-is because os/user.Lookup
// requires cgo on macOS.
func ExpandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// ValidateDBPath rejects paths that could be interpreted as URIs by the
// SQLite driver. The underlying go-sqlite driver passes query strings
// through net/url.ParseQuery (subject to CVE GO-2026-4341) and enables
// SQLITE_OPEN_URI, so both file:// URIs and scheme://... URLs must be
// blocked. Only plain filesystem paths and the special ":memory:" value
// are accepted.
func ValidateDBPath(path string) error {
	if path == "" {
		return fmt.Errorf("database path must not be empty")
	}
	if path == ":memory:" {
		return nil
	}
	// Reject anything with a URI scheme (letters followed by "://").
	if i := strings.Index(path, "://"); i > 0 {
		scheme := path[:i]
		if isLetterOnly(scheme) {
			return fmt.Errorf(
				"database path %q looks like a URI (%s://); only filesystem paths are accepted",
				path, scheme,
			)
		}
	}
	// Reject "file:" prefix -- even without "//", SQLite interprets
	// "file:path?query" as a URI when SQLITE_OPEN_URI is set.
	if strings.HasPrefix(path, "file:") {
		return fmt.Errorf(
			"database path %q uses the file: scheme; pass a plain filesystem path instead",
			path,
		)
	}
	// Reject paths containing '?' -- the go-sqlite driver splits on '?'
	// and feeds the remainder to url.ParseQuery.
	if strings.ContainsRune(path, '?') {
		return fmt.Errorf(
			"database path %q contains '?' which would be interpreted as query parameters",
			path,
		)
	}
	return nil
}

func isLetterOnly(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// WalCheckpoint runs a WAL checkpoint that flushes the WAL into the main
// database file and truncates the WAL. This ensures the .db file is
// self-contained with no -wal or -shm sidecars.
func (s *Store) WalCheckpoint() error {
	return s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get underlying db: %w", err)
	}
	return sqlDB.Close()
}

// Transaction executes fn inside a database transaction. The callback
// receives a transactional Store that shares all methods but operates on the
// transaction. If fn returns an error the transaction is rolled back;
// otherwise it is committed.
func (s *Store) Transaction(fn func(tx *Store) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		txStore := &Store{
			db:              tx,
			maxDocumentSize: s.maxDocumentSize,
			currency:        s.currency,
			deviceCell:      s.deviceCell,
		}
		return fn(txStore)
	})
}

// IsMicasaDB returns true if the database contains the core micasa tables.
func (s *Store) IsMicasaDB() (bool, error) {
	var count int64
	err := s.db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN (?, ?, ?)`,
		TableVendors, TableProjects, TableAppliances,
	).Scan(&count).Error
	if err != nil {
		return false, err
	}
	return count == 3, nil
}

func (s *Store) SetQueryOnly() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB for query_only: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := s.db.Exec("PRAGMA query_only = ON").Error; err != nil {
		return fmt.Errorf("set query_only pragma: %w", err)
	}
	return nil
}

func (s *Store) AutoMigrate() error {
	if err := migrateIntToStringIDs(s.db); err != nil {
		return fmt.Errorf("pre-migrate int-to-string IDs: %w", err)
	}
	if err := s.db.AutoMigrate(Models()...); err != nil {
		return err
	}
	return s.setupFTS()
}

func (s *Store) SeedDefaults() error {
	if err := s.seedProjectTypes(); err != nil {
		return err
	}
	return s.seedMaintenanceCategories()
}

// SeedDemoData populates the database with realistic demo data using a fixed
// seed so the demo always looks the same. Skips if data already exists.
func (s *Store) SeedDemoData() error {
	return s.SeedDemoDataFrom(fake.New(42))
}

// MaxIDs returns the current maximum ID (lexicographic for ULIDs) for each of
// the named tables. Tables with no rows are omitted from the result.
func (s *Store) MaxIDs(tables ...string) (map[string]string, error) {
	result := make(map[string]string, len(tables))
	for _, table := range tables {
		var maxID *string
		if err := s.db.Table(table).Select("MAX(id)").Scan(&maxID).Error; err != nil {
			return nil, fmt.Errorf("max id for %s: %w", table, err)
		}
		if maxID != nil {
			result[table] = *maxID
		}
	}
	return result, nil
}

// RowCounts returns the number of non-deleted rows for each of the named
// tables. Tables with no rows return 0.
func (s *Store) RowCounts(tables ...string) (map[string]int, error) {
	result := make(map[string]int, len(tables))
	for _, table := range tables {
		var count int64
		if err := s.db.Table(table).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("row count for %s: %w", table, err)
		}
		n, err := safeconv.Int(count)
		if err != nil {
			return nil, fmt.Errorf("row count for %s: %w", table, err)
		}
		result[table] = n
	}
	return result, nil
}

var (
	// ErrParentDeleted indicates the parent record exists but is soft-deleted.
	ErrParentDeleted = errors.New("parent record is deleted")
	// ErrParentNotFound indicates the parent record doesn't exist at all.
	ErrParentNotFound = errors.New("parent record not found")
)

// requireParentAlive returns ErrParentDeleted if the parent record is
// soft-deleted, or ErrParentNotFound if it doesn't exist at all. Returns nil
// when the parent is alive.
func (s *Store) requireParentAlive(model any, id string) error {
	err := s.db.First(model, "id = ?", id).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// Distinguish soft-deleted from truly missing.
	if err := s.db.Unscoped().First(model, "id = ?", id).Error; err != nil {
		return ErrParentNotFound
	}
	return ErrParentDeleted
}

// parentRestoreError returns a user-facing error message for a failed parent
// alive check, distinguishing soft-deleted parents (restorable) from missing
// parents (permanently gone).
// parentCheck describes a parent FK that must be alive before a child can be
// restored. A nil id means the FK is optional and unset; the check is skipped.
type parentCheck struct {
	model any // GORM model pointer -- typed as any because gorm.DB.First accepts any
	id    *string
	label string
}

func (s *Store) checkParentsAlive(checks []parentCheck) error {
	for _, c := range checks {
		if c.id == nil {
			continue
		}
		if err := s.requireParentAlive(c.model, *c.id); err != nil {
			return parentRestoreError(c.label, err)
		}
	}
	return nil
}

func parentRestoreError(entity string, err error) error {
	if errors.Is(err, ErrParentNotFound) {
		return fmt.Errorf("%s no longer exists", entity)
	}
	return fmt.Errorf("%s is deleted -- restore it first", entity)
}

// countDependents counts non-deleted rows in model where fkColumn equals id.
// GORM's soft-delete scope automatically excludes deleted rows.
func (s *Store) countDependents(model any, fkColumn string, id string) (int64, error) {
	var count int64
	err := s.db.Model(model).Where(fkColumn+" = ?", id).Count(&count).Error
	return count, err
}

func softDeleteWith(tx *gorm.DB, model any, entity string, id string) error {
	result := tx.Where("id = ?", id).Delete(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	// Write oplog "delete" entry for the soft-deleted entity.
	if table := deletionEntityToTable[entity]; table != "" && !isSyncApplying(tx) {
		if err := writeOplogEntryRaw(tx, table, id, OpDelete); err != nil {
			return err
		}
	}

	record := DeletionRecord{
		Entity:    entity,
		TargetID:  id,
		DeletedAt: time.Now(),
	}
	return tx.Create(&record).Error
}

func (s *Store) softDelete(model any, entity string, id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return softDeleteWith(tx, model, entity, id)
	})
}

// restoreSoftDeleted clears a record's soft-delete timestamp, marks the
// corresponding DeletionRecord as restored, and writes a "restore" oplog
// entry. Callers that need transactional guarantees should pass a *gorm.DB
// obtained from db.Transaction.
func restoreSoftDeleted(tx *gorm.DB, model any, entity string, id string) error {
	if err := tx.Unscoped().Model(model).
		Where(ColID+" = ?", id).
		Update(ColDeletedAt, nil).Error; err != nil {
		return err
	}

	// Write oplog "restore" entry. GORM's Unscoped().Update() does not
	// trigger model-level AfterUpdate hooks, so we must do this explicitly.
	if table := deletionEntityToTable[entity]; table != "" && !isSyncApplying(tx) {
		if err := writeOplogEntryRaw(tx, table, id, OpRestore); err != nil {
			return err
		}
	}

	restoredAt := time.Now()
	return tx.Model(&DeletionRecord{}).
		Where(
			ColEntity+" = ? AND "+ColTargetID+" = ? AND "+ColRestoredAt+" IS NULL",
			entity, id,
		).
		Update(ColRestoredAt, restoredAt).Error
}

func (s *Store) restoreEntity(model any, entity string, id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return restoreSoftDeleted(tx, model, entity, id)
	})
}

// restoreWithParentChecks validates that all parent FKs are alive, then
// restores the entity. It deduplicates the check-parents → restore pattern
// used by RestoreQuote, RestoreMaintenance, and similar simple restore methods.
func (s *Store) restoreWithParentChecks(
	model any,
	entity string,
	id string,
	checks []parentCheck,
) error {
	if err := s.checkParentsAlive(checks); err != nil {
		return err
	}
	return s.restoreEntity(model, entity, id)
}

func (s *Store) LastDeletion(entity string) (DeletionRecord, error) {
	var record DeletionRecord
	err := s.db.
		Where(ColEntity+" = ? AND "+ColRestoredAt+" IS NULL", entity).
		Order(ColID + " desc").
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DeletionRecord{}, gorm.ErrRecordNotFound
	}
	return record, err
}

// countByFK groups rows in model by fkColumn and returns a count per FK value.
// Only non-deleted rows are counted (soft-delete scope applies automatically).
func (s *Store) countByFK(model any, fkColumn string, ids []string) (map[string]int, error) {
	if len(ids) == 0 {
		return map[string]int{}, nil
	}
	type row struct {
		FK    string `gorm:"column:fk"`
		Count int    `gorm:"column:cnt"`
	}
	var results []row
	err := s.db.Model(model).
		Select(fkColumn+" as fk, count(*) as cnt").
		Where(fkColumn+" IN ?", ids).
		Group(fkColumn).
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

// updateByIDWith updates a record by ID, preserving id, created_at, and
// deleted_at. Works with both Store.db and transaction handles.
// Writes an "update" oplog entry with the new values as payload.
func updateByIDWith(db *gorm.DB, table string, model any, id string, values any) error {
	if err := db.Model(model).Where(ColID+" = ?", id). //nolint:unqueryvet // GORM Select("*") updates all non-omitted columns
								Select("*").
								Omit(ColID, ColCreatedAt, ColDeletedAt).
								Updates(values).Error; err != nil {
		return err
	}
	if !isSyncApplying(db) {
		return writeOplogEntry(db, table, id, OpUpdate, values)
	}
	return nil
}

func (s *Store) updateByID(table string, model any, id string, values any) error {
	return updateByIDWith(s.db, table, model, id, values)
}
