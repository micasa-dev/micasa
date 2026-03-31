// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Based on github.com/glebarez/sqlite v1.11.0.
// Original code copyright (c) 2013-NOW Jinzhu <wosmvp@gmail.com>,
// licensed under the MIT License. See LICENSE-glebarez-sqlite for the
// full MIT text. Inlined because the upstream package is unmaintained.

package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// DriverName is the default driver name for SQLite.
const DriverName = "sqlite"

type Dialector struct {
	DriverName string
	DSN        string
	Conn       gorm.ConnPool
	Pragmas    []string // PRAGMA statements to run on each new connection
}

func Open(dsn string, pragmas ...string) gorm.Dialector {
	return &Dialector{DSN: dsn, Pragmas: pragmas}
}

// pragmaConnector implements driver.Connector and runs PRAGMA statements
// on every new connection, ensuring per-connection settings (foreign_keys,
// synchronous, busy_timeout) survive connection pool recycling.
type pragmaConnector struct {
	dsn     string
	driver  driver.Driver
	pragmas []string
}

func (c *pragmaConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	for _, p := range c.pragmas {
		if err := execPragma(ctx, conn, p); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// execPragma runs a single PRAGMA statement on a raw driver.Conn,
// using StmtExecContext to avoid the deprecated Stmt.Exec path.
func execPragma(ctx context.Context, conn driver.Conn, pragma string) error {
	stmt, err := conn.Prepare(pragma)
	if err != nil {
		return fmt.Errorf("pragma %q: %w", pragma, err)
	}
	defer func() { _ = stmt.Close() }()

	ec, ok := stmt.(driver.StmtExecContext)
	if !ok {
		return fmt.Errorf("pragma %q: driver does not support StmtExecContext", pragma)
	}
	if _, err := ec.ExecContext(ctx, nil); err != nil {
		return fmt.Errorf("pragma %q: %w", pragma, err)
	}
	return nil
}

func (c *pragmaConnector) Driver() driver.Driver {
	return c.driver
}

func (dialector Dialector) Name() string {
	return "sqlite"
}

func (dialector Dialector) Initialize(db *gorm.DB) (err error) {
	if dialector.DriverName == "" {
		dialector.DriverName = DriverName
	}

	// Force the sqlite time format so timestamps roundtrip reliably.
	// Go's default time.String() produces representations like
	// "+0530 +0530" on numeric-offset timezones that the driver
	// cannot parse back, causing Scan failures.
	sep := "?"
	if strings.ContainsRune(dialector.DSN, '?') {
		sep = "&"
	}
	dsn := dialector.DSN + sep + "_time_format=sqlite"

	if dialector.Conn != nil {
		db.ConnPool = dialector.Conn
	} else if len(dialector.Pragmas) > 0 {
		// Grab the registered driver, then use sql.OpenDB with a connector
		// that runs pragmas on every new connection.
		tmpDB, err := sql.Open(dialector.DriverName, "")
		if err != nil {
			return err
		}
		drv := tmpDB.Driver()
		if err := tmpDB.Close(); err != nil {
			return err
		}
		db.ConnPool = sql.OpenDB(&pragmaConnector{
			dsn:     dsn,
			driver:  drv,
			pragmas: dialector.Pragmas,
		})
	} else {
		conn, err := sql.Open(dialector.DriverName, dsn)
		if err != nil {
			return err
		}
		db.ConnPool = conn
	}

	var version string
	if err := db.ConnPool.QueryRowContext(
		context.Background(), "select sqlite_version()",
	).Scan(&version); err != nil {
		return err
	}
	// https://www.sqlite.org/releaselog/3_35_0.html
	if compareVersion(version, "3.35.0") >= 0 {
		callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{
			CreateClauses:        []string{"INSERT", "VALUES", "ON CONFLICT", "RETURNING"},
			UpdateClauses:        []string{"UPDATE", "SET", "FROM", "WHERE", "RETURNING"},
			DeleteClauses:        []string{"DELETE", "FROM", "WHERE", "RETURNING"},
			LastInsertIDReversed: true,
		})
	} else {
		callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{
			LastInsertIDReversed: true,
		})
	}

	maps.Copy(db.ClauseBuilders, dialector.ClauseBuilders())
	return err
}

func (dialector Dialector) ClauseBuilders() map[string]clause.ClauseBuilder {
	return map[string]clause.ClauseBuilder{
		"INSERT": func(c clause.Clause, builder clause.Builder) {
			if insert, ok := c.Expression.(clause.Insert); ok {
				if stmt, ok := builder.(*gorm.Statement); ok {
					_, _ = stmt.WriteString("INSERT ")
					if insert.Modifier != "" {
						_, _ = stmt.WriteString(insert.Modifier)
						_ = stmt.WriteByte(' ')
					}

					_, _ = stmt.WriteString("INTO ")
					if insert.Table.Name == "" {
						stmt.WriteQuoted(stmt.Table)
					} else {
						stmt.WriteQuoted(insert.Table)
					}
					return
				}
			}

			c.Build(builder)
		},
		"LIMIT": func(c clause.Clause, builder clause.Builder) {
			if limit, ok := c.Expression.(clause.Limit); ok {
				lmt := -1
				if limit.Limit != nil && *limit.Limit >= 0 {
					lmt = *limit.Limit
				}
				if lmt >= 0 || limit.Offset > 0 {
					_, _ = builder.WriteString("LIMIT ")
					_, _ = builder.WriteString(strconv.Itoa(lmt))
				}
				if limit.Offset > 0 {
					_, _ = builder.WriteString(" OFFSET ")
					_, _ = builder.WriteString(strconv.Itoa(limit.Offset))
				}
			}
		},
		"FOR": func(c clause.Clause, builder clause.Builder) {
			if _, ok := c.Expression.(clause.Locking); ok {
				// SQLite3 does not support row-level locking.
				return
			}
			c.Build(builder)
		},
	}
}

func (dialector Dialector) DefaultValueOf(field *schema.Field) clause.Expression {
	if field.AutoIncrement {
		return clause.Expr{SQL: "NULL"}
	}

	return clause.Expr{SQL: "DEFAULT"}
}

func (dialector Dialector) Migrator(db *gorm.DB) gorm.Migrator {
	return Migrator{migrator.Migrator{Config: migrator.Config{
		DB:                          db,
		Dialector:                   dialector,
		CreateIndexAfterCreateTable: true,
	}}}
}

func (dialector Dialector) BindVarTo(
	writer clause.Writer, _ *gorm.Statement, _ any,
) {
	_ = writer.WriteByte('?')
}

func (dialector Dialector) QuoteTo(writer clause.Writer, str string) {
	var (
		underQuoted, selfQuoted bool
		continuousBacktick      int8
		shiftDelimiter          int8
	)

	for _, v := range []byte(str) {
		switch v {
		case '`':
			continuousBacktick++
			if continuousBacktick == 2 {
				_, _ = writer.WriteString("``")
				continuousBacktick = 0
			}
		case '.':
			if continuousBacktick > 0 || !selfQuoted {
				shiftDelimiter = 0
				underQuoted = false
				continuousBacktick = 0
				_, _ = writer.WriteString("`")
			}
			_ = writer.WriteByte(v)
			continue
		default:
			if shiftDelimiter-continuousBacktick <= 0 && !underQuoted {
				_, _ = writer.WriteString("`")
				underQuoted = true
				if selfQuoted = continuousBacktick > 0; selfQuoted {
					continuousBacktick--
				}
			}

			for ; continuousBacktick > 0; continuousBacktick-- {
				_, _ = writer.WriteString("``")
			}

			_ = writer.WriteByte(v)
		}
		shiftDelimiter++
	}

	if continuousBacktick > 0 && !selfQuoted {
		_, _ = writer.WriteString("``")
	}
	_, _ = writer.WriteString("`")
}

func (dialector Dialector) Explain(sql string, vars ...any) string {
	return logger.ExplainSQL(sql, nil, `"`, vars...)
}

func (dialector Dialector) DataTypeOf(field *schema.Field) string {
	switch field.DataType {
	case schema.Bool:
		return "numeric"
	case schema.Int, schema.Uint:
		if field.AutoIncrement {
			return "integer PRIMARY KEY AUTOINCREMENT"
		}
		return "integer"
	case schema.Float:
		return "real"
	case schema.String:
		return "text"
	case schema.Time:
		if val, ok := field.TagSettings["TYPE"]; ok {
			return val
		}
		return "datetime"
	case schema.Bytes:
		return "blob"
	}

	return string(field.DataType)
}

func (dialector Dialector) SavePoint(tx *gorm.DB, name string) error {
	return tx.Exec("SAVEPOINT `" + name + "`").Error
}

func (dialector Dialector) RollbackTo(tx *gorm.DB, name string) error {
	return tx.Exec("ROLLBACK TO SAVEPOINT `" + name + "`").Error
}

// Translate maps SQLite error codes to GORM sentinel errors.
// Uses modernc.org/sqlite's Error type directly instead of
// the unmaintained glebarez/go-sqlite wrapper.
func (dialector Dialector) Translate(err error) error {
	var terr *sqlite.Error
	if errors.As(err, &terr) {
		switch terr.Code() {
		case sqlite3.SQLITE_CONSTRAINT_UNIQUE,
			sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
			return gorm.ErrDuplicatedKey
		case sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
			return gorm.ErrForeignKeyViolated
		}
	}
	return err
}

func compareVersion(version1, version2 string) int {
	n, m := len(version1), len(version2)
	i, j := 0, 0
	for i < n || j < m {
		x := 0
		for ; i < n && version1[i] != '.'; i++ {
			x = x*10 + int(version1[i]-'0')
		}
		i++
		y := 0
		for ; j < m && version2[j] != '.'; j++ {
			y = y*10 + int(version2[j]-'0')
		}
		j++
		if x > y {
			return 1
		}
		if x < y {
			return -1
		}
	}
	return 0
}
