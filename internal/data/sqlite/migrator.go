// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Based on github.com/glebarez/sqlite v1.11.0.
// Original code copyright (c) 2013-NOW Jinzhu <wosmvp@gmail.com>,
// licensed under the MIT License. See LICENSE-glebarez-sqlite for the
// full MIT text. Inlined because the upstream package is unmaintained.

package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

type Migrator struct {
	migrator.Migrator
}

func (m *Migrator) RunWithoutForeignKey(fc func() error) error {
	var enabled int
	m.DB.Raw("PRAGMA foreign_keys").Scan(&enabled)
	if enabled == 1 {
		m.DB.Exec("PRAGMA foreign_keys = OFF")
		defer m.DB.Exec("PRAGMA foreign_keys = ON")
	}

	return fc()
}

// HasTable implements gorm.Migrator. The signature returns bool (no error),
// matching GORM's interface contract.
func (m Migrator) HasTable(value interface{}) bool {
	var count int
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		return m.DB.Raw(
			"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?",
			stmt.Table,
		).Row().Scan(&count)
	})
	return count > 0
}

func (m Migrator) DropTable(values ...interface{}) error {
	return m.RunWithoutForeignKey(func() error {
		values = m.ReorderModels(values, false)
		tx := m.DB.Session(&gorm.Session{})

		for i := len(values) - 1; i >= 0; i-- {
			if err := m.RunWithValue(values[i], func(stmt *gorm.Statement) error {
				return tx.Exec(
					"DROP TABLE IF EXISTS ?",
					clause.Table{Name: stmt.Table},
				).Error
			}); err != nil {
				return err
			}
		}

		return nil
	})
}

func (m Migrator) GetTables() (tableList []string, err error) {
	return tableList, m.DB.Raw(
		"SELECT name FROM sqlite_master where type=?", "table",
	).Scan(&tableList).Error
}

// HasColumn implements gorm.Migrator. The signature returns bool (no error),
// matching GORM's interface contract.
func (m Migrator) HasColumn(value interface{}, name string) bool {
	var count int
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if field := stmt.Schema.LookUpField(name); field != nil {
				name = field.DBName
			}
		}

		if name != "" {
			return m.DB.Raw(
				"SELECT count(*) FROM sqlite_master WHERE type = ? AND tbl_name = ?"+
					" AND (sql LIKE ? OR sql LIKE ? OR sql LIKE ? OR sql LIKE ?"+
					" OR sql LIKE ?)",
				"table", stmt.Table,
				`%"`+name+`" %`, `%`+name+` %`,
				"%`"+name+"`%", "%["+name+"]%",
				"%\t"+name+"\t%",
			).Row().Scan(&count)
		}
		return nil
	})
	return count > 0
}

func (m Migrator) AlterColumn(value interface{}, name string) error {
	return m.RunWithoutForeignKey(func() error {
		return m.recreateTable(value, nil,
			func(d *ddl, stmt *gorm.Statement) (*ddl, []interface{}, error) {
				if field := stmt.Schema.LookUpField(name); field != nil {
					var sqlArgs []interface{}
					for i, f := range d.fields {
						matches := columnRegexp.FindStringSubmatch(f)
						if len(matches) > 1 && matches[1] == field.DBName {
							d.fields[i] = fmt.Sprintf("`%v` ?", field.DBName)
							sqlArgs = []interface{}{m.FullDataTypeOf(field)}
							if strings.Contains(strings.ToUpper(matches[3]), " UNIQUE") {
								uniName := m.DB.NamingStrategy.UniqueName(
									stmt.Table, field.DBName,
								)
								uni, _ := m.GuessConstraintInterfaceAndTable(stmt, uniName)
								if uni != nil {
									uniSQL, uniArgs := uni.Build()
									d.addConstraint(uniName, uniSQL)
									sqlArgs = append(sqlArgs, uniArgs...)
								}
							}
							break
						}
					}
					return d, sqlArgs, nil
				}
				return nil, nil, fmt.Errorf("failed to alter field with name %v", name)
			})
	})
}

func (m Migrator) ColumnTypes(value interface{}) ([]gorm.ColumnType, error) {
	columnTypes := make([]gorm.ColumnType, 0)
	execErr := m.RunWithValue(value, func(stmt *gorm.Statement) (err error) {
		var (
			sqls   []string
			sqlDDL *ddl
		)

		if err := m.DB.Raw(
			"SELECT sql FROM sqlite_master WHERE type IN ? AND tbl_name = ?"+
				" AND sql IS NOT NULL order by type = ? desc",
			[]string{"table", "index"}, stmt.Table, "table",
		).Scan(&sqls).Error; err != nil {
			return err
		}

		if sqlDDL, err = parseDDL(sqls...); err != nil {
			return err
		}

		rows, err := m.DB.Session(&gorm.Session{}).Table(stmt.Table).Limit(1).Rows()
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()

		var rawColumnTypes []*sql.ColumnType
		rawColumnTypes, err = rows.ColumnTypes()
		if err != nil {
			return err
		}

		for _, c := range rawColumnTypes {
			columnType := migrator.ColumnType{SQLColumnType: c}
			for _, column := range sqlDDL.columns {
				if column.NameValue.String == c.Name() {
					column.SQLColumnType = c
					columnType = column
					break
				}
			}
			columnTypes = append(columnTypes, columnType)
		}

		return err
	})

	return columnTypes, execErr
}

func (m Migrator) DropColumn(value interface{}, name string) error {
	return m.recreateTable(value, nil,
		func(d *ddl, stmt *gorm.Statement) (*ddl, []interface{}, error) {
			if field := stmt.Schema.LookUpField(name); field != nil {
				name = field.DBName
			}

			d.removeColumn(name)
			return d, nil, nil
		})
}

func (m Migrator) CreateConstraint(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		constraint, table := m.GuessConstraintInterfaceAndTable(stmt, name)

		return m.recreateTable(value, &table,
			func(d *ddl, _ *gorm.Statement) (*ddl, []interface{}, error) {
				var (
					constraintName   string
					constraintSQL    string
					constraintValues []interface{}
				)

				if constraint != nil {
					constraintName = constraint.GetName()
					constraintSQL, constraintValues = constraint.Build()
				} else {
					return nil, nil, nil
				}

				d.addConstraint(constraintName, constraintSQL)
				return d, constraintValues, nil
			})
	})
}

func (m Migrator) DropConstraint(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		constraint, table := m.GuessConstraintInterfaceAndTable(stmt, name)
		if constraint != nil {
			name = constraint.GetName()
		}

		return m.recreateTable(value, &table,
			func(d *ddl, _ *gorm.Statement) (*ddl, []interface{}, error) {
				d.removeConstraint(name)
				return d, nil, nil
			})
	})
}

// HasConstraint implements gorm.Migrator. The signature returns bool (no
// error), matching GORM's interface contract.
func (m Migrator) HasConstraint(value interface{}, name string) bool {
	var count int64
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		constraint, table := m.GuessConstraintInterfaceAndTable(stmt, name)
		if constraint != nil {
			name = constraint.GetName()
		}

		return m.DB.Raw(
			"SELECT count(*) FROM sqlite_master WHERE type = ? AND tbl_name = ?"+
				" AND (sql LIKE ? OR sql LIKE ? OR sql LIKE ? OR sql LIKE ?"+
				" OR sql LIKE ?)",
			"table", table,
			`%CONSTRAINT "`+name+`" %`, `%CONSTRAINT `+name+` %`,
			"%CONSTRAINT `"+name+"`%", "%CONSTRAINT ["+name+"]%",
			"%CONSTRAINT \t"+name+"\t%",
		).Row().Scan(&count)
	})

	return count > 0
}

func (m Migrator) CurrentDatabase() (name string) {
	var null interface{}
	_ = m.DB.Raw("PRAGMA database_list").Row().Scan(&null, &name, &null)
	return
}

func (m Migrator) BuildIndexOptions(
	opts []schema.IndexOption, stmt *gorm.Statement,
) (results []interface{}) {
	for _, opt := range opts {
		str := stmt.Quote(opt.DBName)
		if opt.Expression != "" {
			str = opt.Expression
		}

		if opt.Collate != "" {
			str += " COLLATE " + opt.Collate
		}

		if opt.Sort != "" {
			str += " " + opt.Sort
		}
		results = append(results, clause.Expr{SQL: str})
	}
	return
}

func (m Migrator) CreateIndex(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if idx := stmt.Schema.LookIndex(name); idx != nil {
				opts := m.BuildIndexOptions(idx.Fields, stmt)
				values := []interface{}{
					clause.Column{Name: idx.Name},
					clause.Table{Name: stmt.Table},
					opts,
				}

				var b strings.Builder
				b.WriteString("CREATE ")
				if idx.Class != "" {
					b.WriteString(idx.Class)
					b.WriteByte(' ')
				}
				b.WriteString("INDEX ?")
				if idx.Type != "" {
					b.WriteString(" USING ")
					b.WriteString(idx.Type)
				}
				b.WriteString(" ON ??")
				if idx.Where != "" {
					b.WriteString(" WHERE ")
					b.WriteString(idx.Where)
				}

				return m.DB.Exec(b.String(), values...).Error
			}
		}
		return fmt.Errorf("failed to create index with name %v", name)
	})
}

// HasIndex implements gorm.Migrator. The signature returns bool (no error),
// matching GORM's interface contract.
func (m Migrator) HasIndex(value interface{}, name string) bool {
	var count int
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if idx := stmt.Schema.LookIndex(name); idx != nil {
				name = idx.Name
			}
		}

		if name != "" {
			return m.DB.Raw(
				"SELECT count(*) FROM sqlite_master WHERE type = ?"+
					" AND tbl_name = ? AND name = ?",
				"index", stmt.Table, name,
			).Row().Scan(&count)
		}
		return nil
	})
	return count > 0
}

func (m Migrator) RenameIndex(value interface{}, oldName, newName string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		var indexSQL string
		if err := m.DB.Raw(
			"SELECT sql FROM sqlite_master WHERE type = ?"+
				" AND tbl_name = ? AND name = ?",
			"index", stmt.Table, oldName,
		).Row().Scan(&indexSQL); err != nil {
			return err
		}
		if indexSQL != "" {
			if err := m.DropIndex(value, oldName); err != nil {
				return err
			}
			return m.DB.Exec(strings.Replace(indexSQL, oldName, newName, 1)).Error
		}
		return fmt.Errorf("failed to find index with name %v", oldName)
	})
}

func (m Migrator) DropIndex(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if idx := stmt.Schema.LookIndex(name); idx != nil {
				name = idx.Name
			}
		}

		return m.DB.Exec("DROP INDEX ?", clause.Column{Name: name}).Error
	})
}

type Index struct {
	Seq     int
	Name    string
	Unique  bool
	Origin  string
	Partial bool
}

func (m Migrator) GetIndexes(value interface{}) ([]gorm.Index, error) {
	indexes := make([]gorm.Index, 0)
	err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		rst := make([]*Index, 0)
		if err := m.DB.Raw(
			"SELECT * FROM PRAGMA_index_list(?)", stmt.Table,
		).Scan(&rst).Error; err != nil {
			return err
		}
		for _, index := range rst {
			if index.Origin == "u" {
				continue
			}
			var columns []string
			if err := m.DB.Raw(
				"SELECT name FROM PRAGMA_index_info(?)", index.Name,
			).Scan(&columns).Error; err != nil {
				return err
			}
			indexes = append(indexes, &migrator.Index{
				TableName:  stmt.Table,
				NameValue:  index.Name,
				ColumnList: columns,
				PrimaryKeyValue: sql.NullBool{
					Bool: index.Origin == "pk", Valid: true,
				},
				UniqueValue: sql.NullBool{Bool: index.Unique, Valid: true},
			})
		}
		return nil
	})
	return indexes, err
}

func (m Migrator) getRawDDL(table string) (string, error) {
	var createSQL string
	if err := m.DB.Raw(
		"SELECT sql FROM sqlite_master WHERE type = ?"+
			" AND tbl_name = ? AND name = ?",
		"table", table, table,
	).Row().Scan(&createSQL); err != nil {
		return "", err
	}
	return createSQL, nil
}

func (m Migrator) recreateTable(
	value interface{}, tablePtr *string,
	getCreateSQL func(
		d *ddl, stmt *gorm.Statement,
	) (result *ddl, sqlArgs []interface{}, err error),
) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		table := stmt.Table
		if tablePtr != nil {
			table = *tablePtr
		}

		rawDDL, err := m.getRawDDL(table)
		if err != nil {
			return err
		}

		originDDL, err := parseDDL(rawDDL)
		if err != nil {
			return err
		}

		createDDL, sqlArgs, err := getCreateSQL(originDDL.clone(), stmt)
		if err != nil {
			return err
		}
		if createDDL == nil {
			return nil
		}

		newTableName := table + "__temp"
		if err := createDDL.renameTable(newTableName, table); err != nil {
			return err
		}

		columns := createDDL.getColumns()
		createSQL := createDDL.compile()

		return m.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(createSQL, sqlArgs...).Error; err != nil {
				return err
			}

			queries := []string{
				fmt.Sprintf(
					"INSERT INTO `%v`(%v) SELECT %v FROM `%v`",
					newTableName,
					strings.Join(columns, ","),
					strings.Join(columns, ","),
					table,
				),
				fmt.Sprintf("DROP TABLE `%v`", table),
				fmt.Sprintf(
					"ALTER TABLE `%v` RENAME TO `%v`",
					newTableName, table,
				),
			}
			for _, query := range queries {
				if err := tx.Exec(query).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
}
