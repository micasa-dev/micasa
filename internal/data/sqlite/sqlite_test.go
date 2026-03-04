// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Based on github.com/glebarez/sqlite v1.11.0.
// Original code copyright (c) 2013-NOW Jinzhu <wosmvp@gmail.com>,
// licensed under the MIT License. See LICENSE-glebarez-sqlite for the
// full MIT text. Inlined because the upstream package is unmaintained.

package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	modernsqlite "modernc.org/sqlite"
)

var testSeq atomic.Uint64

// testDSN returns a unique shared in-memory DSN per test invocation to avoid
// cross-test lock contention on the same shared cache database.
func testDSN(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), testSeq.Add(1))
}

var registerCustomDriver sync.Once

func TestDialector(t *testing.T) {
	const customDriverName = "test_custom_driver"

	registerCustomDriver.Do(func() {
		sql.Register(customDriverName, &modernsqlite.Driver{})
	})

	dsn := testDSN(t)
	tests := []struct {
		description  string
		dialector    *Dialector
		openSuccess  bool
		query        string
		querySuccess bool
	}{
		{
			description:  "default_driver",
			dialector:    &Dialector{DSN: dsn},
			openSuccess:  true,
			query:        "SELECT 1",
			querySuccess: true,
		},
		{
			description: "explicit_default_driver",
			dialector: &Dialector{
				DriverName: DriverName,
				DSN:        dsn,
			},
			openSuccess:  true,
			query:        "SELECT 1",
			querySuccess: true,
		},
		{
			description: "bad_driver",
			dialector: &Dialector{
				DriverName: "not-a-real-driver",
				DSN:        dsn,
			},
			openSuccess: false,
		},
		{
			description: "custom_driver",
			dialector: &Dialector{
				DriverName: customDriverName,
				DSN:        dsn,
			},
			openSuccess:  true,
			query:        "SELECT 1",
			querySuccess: true,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d/%s", i, tt.description), func(t *testing.T) {
			db, err := gorm.Open(tt.dialector, &gorm.Config{})
			if !tt.openSuccess {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, db)

			if tt.query != "" {
				err = db.Exec(tt.query).Error
				if !tt.querySuccess {
					assert.Error(t, err)
					return
				}
				assert.NoError(t, err)
			}
		})
	}
}

func TestErrorTranslator(t *testing.T) {
	t.Parallel()
	type Article struct {
		ArticleNumber string `gorm:"unique"`
	}

	db, err := gorm.Open(&Dialector{DSN: testDSN(t)}, &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	require.NoError(t, err)
	require.NotNil(t, db)

	require.NoError(t, db.AutoMigrate(&Article{}))

	err = db.Create(&Article{ArticleNumber: "A00000XX"}).Error
	require.NoError(t, err)

	err = db.Create(&Article{ArticleNumber: "A00000XX"}).Error
	require.Error(t, err)
	assert.ErrorIs(t, err, gorm.ErrDuplicatedKey)
}

func TestSQLiteVersion(t *testing.T) {
	t.Parallel()
	db, err := sql.Open(DriverName, ":memory:")
	require.NoError(t, err)

	var version string
	require.NoError(t, db.QueryRowContext(t.Context(), "select sqlite_version()").Scan(&version))
	assert.NotEmpty(t, version)
	t.Logf("SQLite version: %s", version)
}

func TestOpen(t *testing.T) {
	t.Parallel()
	dsn := testDSN(t)
	d := Open(dsn)
	assert.Equal(t, "sqlite", d.Name())

	dialector, ok := d.(*Dialector)
	require.True(t, ok)
	assert.Equal(t, dsn, dialector.DSN)
	assert.Empty(t, dialector.DriverName)
}

func TestDefaultValueOf(t *testing.T) {
	t.Parallel()
	d := Dialector{}

	auto := d.DefaultValueOf(&schema.Field{AutoIncrement: true})
	assert.Equal(t, clause.Expr{SQL: "NULL"}, auto)

	normal := d.DefaultValueOf(&schema.Field{AutoIncrement: false})
	assert.Equal(t, clause.Expr{SQL: "DEFAULT"}, normal)
}

func TestDataTypeOf(t *testing.T) {
	t.Parallel()
	d := Dialector{}

	tests := []struct {
		field *schema.Field
		want  string
	}{
		{&schema.Field{DataType: schema.Bool}, "numeric"},
		{&schema.Field{DataType: schema.Int}, "integer"},
		{
			&schema.Field{DataType: schema.Int, AutoIncrement: true},
			"integer PRIMARY KEY AUTOINCREMENT",
		},
		{&schema.Field{DataType: schema.Uint}, "integer"},
		{&schema.Field{DataType: schema.Float}, "real"},
		{&schema.Field{DataType: schema.String}, "text"},
		{&schema.Field{DataType: schema.Time}, "datetime"},
		{
			&schema.Field{DataType: schema.Time, TagSettings: map[string]string{"TYPE": "date"}},
			"date",
		},
		{&schema.Field{DataType: schema.Bytes}, "blob"},
		{&schema.Field{DataType: "custom_type"}, "custom_type"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, d.DataTypeOf(tt.field))
		})
	}
}

func TestExplain(t *testing.T) {
	t.Parallel()
	d := Dialector{}
	result := d.Explain("SELECT * FROM users WHERE id = ?", 42)
	assert.Contains(t, result, "42")
	assert.Contains(t, result, "SELECT")
}

func TestQuoteTo(t *testing.T) {
	t.Parallel()
	d := Dialector{}

	tests := []struct {
		input string
		want  string
	}{
		{"simple", "`simple`"},
		{"schema.table", "`schema`.`table`"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var buf strings.Builder
			d.QuoteTo(&buf, tt.input)
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func TestSavePointAndRollbackTo(t *testing.T) {
	t.Parallel()
	db, err := gorm.Open(&Dialector{DSN: testDSN(t)}, &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)

	type Item struct {
		ID   uint
		Name string
	}
	require.NoError(t, db.AutoMigrate(&Item{}))

	d := Dialector{}

	require.NoError(t, db.Create(&Item{Name: "before"}).Error)

	require.NoError(t, d.SavePoint(db, "sp1"))
	require.NoError(t, db.Create(&Item{Name: "after_savepoint"}).Error)
	require.NoError(t, d.RollbackTo(db, "sp1"))

	var count int64
	db.Model(&Item{}).Where("name = ?", "after_savepoint").Count(&count)
	assert.Equal(t, int64(0), count, "rollback should have undone the insert")
}

func TestTranslateForeignKeyViolation(t *testing.T) {
	t.Parallel()
	db, err := gorm.Open(&Dialector{DSN: testDSN(t)}, &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	require.NoError(t, err)

	// Enable foreign keys
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)

	type Parent struct {
		ID uint `gorm:"primarykey"`
	}
	type Child struct {
		ID       uint `gorm:"primarykey"`
		ParentID uint
		Parent   Parent `gorm:"foreignKey:ParentID"`
	}
	require.NoError(t, db.AutoMigrate(&Parent{}, &Child{}))

	// Insert a child referencing a non-existent parent
	err = db.Create(&Child{ParentID: 9999}).Error
	require.Error(t, err)
	assert.ErrorIs(t, err, gorm.ErrForeignKeyViolated)
}

// Regression test for #393: time.Time values in numeric-offset timezones
// (e.g. IST +0530) must roundtrip through SQLite without Scan errors.
func TestTimeRoundtripNumericTimezone(t *testing.T) {
	t.Parallel()
	type Record struct {
		ID        uint `gorm:"primaryKey"`
		CreatedAt time.Time
	}

	db, err := gorm.Open(&Dialector{DSN: testDSN(t)}, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Record{}))

	ist := time.FixedZone("+0530", 5*3600+30*60)
	ts := time.Date(2026, 2, 20, 9, 46, 30, 0, ist)

	require.NoError(t, db.Create(&Record{CreatedAt: ts}).Error)

	var got Record
	require.NoError(t, db.First(&got).Error)
	assert.True(t, ts.Equal(got.CreatedAt),
		"want %v, got %v", ts, got.CreatedAt)
}

func TestCompareVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"3.35.0", "3.35.0", 0},
		{"3.35.1", "3.35.0", 1},
		{"3.35.0", "3.35.1", -1},
		{"3.45.0", "3.35.0", 1},
		{"3.9.0", "3.35.0", -1},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_vs_%s", tt.v1, tt.v2), func(t *testing.T) {
			assert.Equal(t, tt.want, compareVersion(tt.v1, tt.v2))
		})
	}
}
