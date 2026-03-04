// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Based on github.com/glebarez/sqlite v1.11.0.
// Original code copyright (c) 2013-NOW Jinzhu <wosmvp@gmail.com>,
// licensed under the MIT License. See LICENSE-glebarez-sqlite for the
// full MIT text. Inlined because the upstream package is unmaintained.

package sqlite

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/migrator"
)

func TestParseDDL(t *testing.T) {
	t.Parallel()
	params := []struct {
		name    string
		sql     []string
		nFields int
		columns []migrator.ColumnType
	}{
		{
			"with_fk",
			[]string{
				"CREATE TABLE `notes` (" +
					"`id` integer NOT NULL,`text` varchar(500) DEFAULT \"hello\"," +
					"`age` integer DEFAULT 18,`user_id` integer," +
					"PRIMARY KEY (`id`)," +
					"CONSTRAINT `fk_users_notes` FOREIGN KEY (`user_id`) " +
					"REFERENCES `users`(`id`))",
				"CREATE UNIQUE INDEX `idx_profiles_refer` ON `profiles`(`text`)",
			},
			6,
			[]migrator.ColumnType{
				{
					NameValue:         sql.NullString{String: "id", Valid: true},
					DataTypeValue:     sql.NullString{String: "integer", Valid: true},
					ColumnTypeValue:   sql.NullString{String: "integer", Valid: true},
					PrimaryKeyValue:   sql.NullBool{Bool: true, Valid: true},
					NullableValue:     sql.NullBool{Valid: true},
					UniqueValue:       sql.NullBool{Valid: true},
					DefaultValueValue: sql.NullString{Valid: false},
				},
				{
					NameValue:         sql.NullString{String: "text", Valid: true},
					DataTypeValue:     sql.NullString{String: "varchar", Valid: true},
					LengthValue:       sql.NullInt64{Int64: 500, Valid: true},
					ColumnTypeValue:   sql.NullString{String: "varchar(500)", Valid: true},
					DefaultValueValue: sql.NullString{String: "hello", Valid: true},
					NullableValue:     sql.NullBool{Bool: true, Valid: true},
					UniqueValue:       sql.NullBool{Bool: false, Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
				{
					NameValue:         sql.NullString{String: "age", Valid: true},
					DataTypeValue:     sql.NullString{String: "integer", Valid: true},
					ColumnTypeValue:   sql.NullString{String: "integer", Valid: true},
					DefaultValueValue: sql.NullString{String: "18", Valid: true},
					NullableValue:     sql.NullBool{Bool: true, Valid: true},
					UniqueValue:       sql.NullBool{Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
				{
					NameValue:         sql.NullString{String: "user_id", Valid: true},
					DataTypeValue:     sql.NullString{String: "integer", Valid: true},
					ColumnTypeValue:   sql.NullString{String: "integer", Valid: true},
					DefaultValueValue: sql.NullString{Valid: false},
					NullableValue:     sql.NullBool{Bool: true, Valid: true},
					UniqueValue:       sql.NullBool{Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
			},
		},
		{
			"with_check",
			[]string{
				"CREATE TABLE Persons (" +
					"ID int NOT NULL,LastName varchar(255) NOT NULL," +
					"FirstName varchar(255),Age int," +
					"CHECK (Age>=18),CHECK (FirstName<>'John'))",
			},
			6,
			[]migrator.ColumnType{
				{
					NameValue:         sql.NullString{String: "ID", Valid: true},
					DataTypeValue:     sql.NullString{String: "int", Valid: true},
					ColumnTypeValue:   sql.NullString{String: "int", Valid: true},
					NullableValue:     sql.NullBool{Valid: true},
					DefaultValueValue: sql.NullString{Valid: false},
					UniqueValue:       sql.NullBool{Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
				{
					NameValue:         sql.NullString{String: "LastName", Valid: true},
					DataTypeValue:     sql.NullString{String: "varchar", Valid: true},
					LengthValue:       sql.NullInt64{Int64: 255, Valid: true},
					ColumnTypeValue:   sql.NullString{String: "varchar(255)", Valid: true},
					NullableValue:     sql.NullBool{Bool: false, Valid: true},
					DefaultValueValue: sql.NullString{Valid: false},
					UniqueValue:       sql.NullBool{Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
				{
					NameValue:         sql.NullString{String: "FirstName", Valid: true},
					DataTypeValue:     sql.NullString{String: "varchar", Valid: true},
					LengthValue:       sql.NullInt64{Int64: 255, Valid: true},
					ColumnTypeValue:   sql.NullString{String: "varchar(255)", Valid: true},
					DefaultValueValue: sql.NullString{Valid: false},
					NullableValue:     sql.NullBool{Bool: true, Valid: true},
					UniqueValue:       sql.NullBool{Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
				{
					NameValue:         sql.NullString{String: "Age", Valid: true},
					DataTypeValue:     sql.NullString{String: "int", Valid: true},
					ColumnTypeValue:   sql.NullString{String: "int", Valid: true},
					DefaultValueValue: sql.NullString{Valid: false},
					NullableValue:     sql.NullBool{Bool: true, Valid: true},
					UniqueValue:       sql.NullBool{Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
			},
		},
		{
			"lowercase",
			[]string{"create table test (ID int NOT NULL)"},
			1,
			[]migrator.ColumnType{
				{
					NameValue:         sql.NullString{String: "ID", Valid: true},
					DataTypeValue:     sql.NullString{String: "int", Valid: true},
					ColumnTypeValue:   sql.NullString{String: "int", Valid: true},
					NullableValue:     sql.NullBool{Bool: false, Valid: true},
					DefaultValueValue: sql.NullString{Valid: false},
					UniqueValue:       sql.NullBool{Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
			},
		},
		{
			"no_brackets",
			[]string{"create table test"},
			0,
			nil,
		},
		{
			"with_special_characters",
			[]string{
				"CREATE TABLE `test` (`text` varchar(10) DEFAULT \"测试, \")",
			},
			1,
			[]migrator.ColumnType{
				{
					NameValue:         sql.NullString{String: "text", Valid: true},
					DataTypeValue:     sql.NullString{String: "varchar", Valid: true},
					LengthValue:       sql.NullInt64{Int64: 10, Valid: true},
					ColumnTypeValue:   sql.NullString{String: "varchar(10)", Valid: true},
					DefaultValueValue: sql.NullString{String: "测试, ", Valid: true},
					NullableValue:     sql.NullBool{Bool: true, Valid: true},
					UniqueValue:       sql.NullBool{Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
			},
		},
		{
			"table_name_with_dash",
			[]string{
				"CREATE TABLE `test-a` (`id` int NOT NULL)",
				"CREATE UNIQUE INDEX `idx_test-a_id` ON `test-a`(`id`)",
			},
			1,
			[]migrator.ColumnType{
				{
					NameValue:         sql.NullString{String: "id", Valid: true},
					DataTypeValue:     sql.NullString{String: "int", Valid: true},
					ColumnTypeValue:   sql.NullString{String: "int", Valid: true},
					NullableValue:     sql.NullBool{Bool: false, Valid: true},
					DefaultValueValue: sql.NullString{Valid: false},
					UniqueValue:       sql.NullBool{Bool: false, Valid: true},
					PrimaryKeyValue:   sql.NullBool{Valid: true},
				},
			},
		},
		{
			"unique_index",
			[]string{
				"CREATE TABLE `test-b` (`field` integer NOT NULL)",
				"CREATE UNIQUE INDEX `idx_uq` ON `test-b`(`field`) WHERE field = 0",
			},
			1,
			[]migrator.ColumnType{
				{
					NameValue:       sql.NullString{String: "field", Valid: true},
					DataTypeValue:   sql.NullString{String: "integer", Valid: true},
					ColumnTypeValue: sql.NullString{String: "integer", Valid: true},
					PrimaryKeyValue: sql.NullBool{Bool: false, Valid: true},
					UniqueValue:     sql.NullBool{Bool: false, Valid: true},
					NullableValue:   sql.NullBool{Bool: false, Valid: true},
				},
			},
		},
		{
			"normal_index",
			[]string{
				"CREATE TABLE `test-c` (`field` integer NOT NULL)",
				"CREATE INDEX `idx_uq` ON `test-c`(`field`)",
			},
			1,
			[]migrator.ColumnType{
				{
					NameValue:       sql.NullString{String: "field", Valid: true},
					DataTypeValue:   sql.NullString{String: "integer", Valid: true},
					ColumnTypeValue: sql.NullString{String: "integer", Valid: true},
					PrimaryKeyValue: sql.NullBool{Bool: false, Valid: true},
					UniqueValue:     sql.NullBool{Bool: false, Valid: true},
					NullableValue:   sql.NullBool{Bool: false, Valid: true},
				},
			},
		},
		{
			"unique_constraint",
			[]string{
				"CREATE TABLE `unique_struct` " +
					"(`name` text," +
					"CONSTRAINT `uni_unique_struct_name` UNIQUE (`name`))",
			},
			2,
			[]migrator.ColumnType{
				{
					NameValue:       sql.NullString{String: "name", Valid: true},
					DataTypeValue:   sql.NullString{String: "text", Valid: true},
					ColumnTypeValue: sql.NullString{String: "text", Valid: true},
					PrimaryKeyValue: sql.NullBool{Bool: false, Valid: true},
					UniqueValue:     sql.NullBool{Bool: true, Valid: true},
					NullableValue:   sql.NullBool{Bool: true, Valid: true},
				},
			},
		},
		{
			"non_unique_index",
			[]string{
				"CREATE TABLE `test-c` (`field` integer NOT NULL)",
				"CREATE INDEX `idx_uq` ON `test-b`(`field`) WHERE field = 0",
			},
			1,
			[]migrator.ColumnType{
				{
					NameValue:       sql.NullString{String: "field", Valid: true},
					DataTypeValue:   sql.NullString{String: "integer", Valid: true},
					ColumnTypeValue: sql.NullString{String: "integer", Valid: true},
					PrimaryKeyValue: sql.NullBool{Bool: false, Valid: true},
					UniqueValue:     sql.NullBool{Bool: false, Valid: true},
					NullableValue:   sql.NullBool{Bool: false, Valid: true},
				},
			},
		},
		{
			"index_with_newline_from_schema",
			[]string{
				"CREATE TABLE `test-d` (`field` integer NOT NULL)",
				"CREATE INDEX `idx_uq`\n    ON `test-b`(`field`) WHERE field = 0",
			},
			1,
			[]migrator.ColumnType{
				{
					NameValue:       sql.NullString{String: "field", Valid: true},
					DataTypeValue:   sql.NullString{String: "integer", Valid: true},
					ColumnTypeValue: sql.NullString{String: "integer", Valid: true},
					PrimaryKeyValue: sql.NullBool{Bool: false, Valid: true},
					UniqueValue:     sql.NullBool{Bool: false, Valid: true},
					NullableValue:   sql.NullBool{Bool: false, Valid: true},
				},
			},
		},
	}

	for _, p := range params {
		t.Run(p.name, func(t *testing.T) {
			d, err := parseDDL(p.sql...)
			require.NoError(t, err)
			assert.Equal(t, p.sql[0], d.compile())
			assert.Len(t, d.fields, p.nFields)
			assert.Equal(t, p.columns, d.columns)
		})
	}
}

func TestParseDDL_Whitespaces(t *testing.T) {
	t.Parallel()
	testColumns := []migrator.ColumnType{
		{
			NameValue:         sql.NullString{String: "id", Valid: true},
			DataTypeValue:     sql.NullString{String: "integer", Valid: true},
			ColumnTypeValue:   sql.NullString{String: "integer", Valid: true},
			NullableValue:     sql.NullBool{Bool: true, Valid: true},
			DefaultValueValue: sql.NullString{Valid: false},
			UniqueValue:       sql.NullBool{Bool: true, Valid: true},
			PrimaryKeyValue:   sql.NullBool{Bool: true, Valid: true},
		},
		{
			NameValue:         sql.NullString{String: "dark_mode", Valid: true},
			DataTypeValue:     sql.NullString{String: "numeric", Valid: true},
			ColumnTypeValue:   sql.NullString{String: "numeric", Valid: true},
			NullableValue:     sql.NullBool{Bool: true, Valid: true},
			DefaultValueValue: sql.NullString{String: "true", Valid: true},
			UniqueValue:       sql.NullBool{Bool: false, Valid: true},
			PrimaryKeyValue:   sql.NullBool{Bool: false, Valid: true},
		},
	}

	params := []struct {
		name    string
		sql     []string
		nFields int
		columns []migrator.ColumnType
	}{
		{
			"with_newline",
			[]string{
				"CREATE TABLE `users`\n(\nid integer primary key unique," +
					"\ndark_mode numeric DEFAULT true)",
			},
			2,
			testColumns,
		},
		{
			"with_newline_2",
			[]string{
				"CREATE TABLE `users` (\n\nid integer primary key unique," +
					"\ndark_mode numeric DEFAULT true)",
			},
			2,
			testColumns,
		},
		{
			"with_missing_space",
			[]string{
				"CREATE TABLE `users`" +
					"(id integer primary key unique, dark_mode numeric DEFAULT true)",
			},
			2,
			testColumns,
		},
		{
			"with_many_spaces",
			[]string{
				"CREATE TABLE `users`       " +
					"(id    integer   primary key unique," +
					"     dark_mode    numeric DEFAULT true)",
			},
			2,
			testColumns,
		},
	}

	for _, p := range params {
		t.Run(p.name, func(t *testing.T) {
			d, err := parseDDL(p.sql...)
			require.NoError(t, err)
			assert.Len(t, d.fields, p.nFields)
			assert.Equal(t, p.columns, d.columns)
		})
	}
}

func TestParseDDL_Error(t *testing.T) {
	t.Parallel()
	params := []struct {
		name string
		sql  string
	}{
		{"invalid_cmd", "CREATE TABLE"},
		{"unbalanced_brackets", "CREATE TABLE test (ID int NOT NULL,Name varchar(255)"},
		{"unbalanced_brackets2", "CREATE TABLE test (ID int NOT NULL,Name varchar(255)))"},
	}

	for _, p := range params {
		t.Run(p.name, func(t *testing.T) {
			_, err := parseDDL(p.sql)
			assert.Error(t, err)
		})
	}
}

func TestAddConstraint(t *testing.T) {
	t.Parallel()
	params := []struct {
		name   string
		fields []string
		cName  string
		sql    string
		expect []string
	}{
		{
			name:   "add_new",
			fields: []string{"`id` integer NOT NULL"},
			cName:  "fk_users_notes",
			sql: "CONSTRAINT `fk_users_notes` FOREIGN KEY (`user_id`) " +
				"REFERENCES `users`(`id`))",
			expect: []string{
				"`id` integer NOT NULL",
				"CONSTRAINT `fk_users_notes` FOREIGN KEY (`user_id`) " +
					"REFERENCES `users`(`id`))",
			},
		},
		{
			name: "update",
			fields: []string{
				"`id` integer NOT NULL",
				"CONSTRAINT `fk_users_notes` FOREIGN KEY (`user_id`) " +
					"REFERENCES `users`(`id`))",
			},
			cName: "fk_users_notes",
			sql: "CONSTRAINT `fk_users_notes` FOREIGN KEY (`user_id`) " +
				"REFERENCES `users`(`id`)) ON UPDATE CASCADE ON DELETE CASCADE",
			expect: []string{
				"`id` integer NOT NULL",
				"CONSTRAINT `fk_users_notes` FOREIGN KEY (`user_id`) " +
					"REFERENCES `users`(`id`)) ON UPDATE CASCADE ON DELETE CASCADE",
			},
		},
		{
			name:   "add_check",
			fields: []string{"`id` integer NOT NULL"},
			cName:  "name_checker",
			sql:    "CONSTRAINT `name_checker` CHECK (`name` <> 'jinzhu')",
			expect: []string{
				"`id` integer NOT NULL",
				"CONSTRAINT `name_checker` CHECK (`name` <> 'jinzhu')",
			},
		},
		{
			name: "update_check",
			fields: []string{
				"`id` integer NOT NULL",
				"CONSTRAINT `name_checker` CHECK (`name` <> 'thetadev')",
			},
			cName: "name_checker",
			sql:   "CONSTRAINT `name_checker` CHECK (`name` <> 'jinzhu')",
			expect: []string{
				"`id` integer NOT NULL",
				"CONSTRAINT `name_checker` CHECK (`name` <> 'jinzhu')",
			},
		},
	}

	for _, p := range params {
		t.Run(p.name, func(t *testing.T) {
			testDDL := ddl{fields: p.fields}
			testDDL.addConstraint(p.cName, p.sql)
			assert.Equal(t, p.expect, testDDL.fields)
		})
	}
}

func TestRemoveConstraint(t *testing.T) {
	t.Parallel()
	params := []struct {
		name    string
		fields  []string
		cName   string
		success bool
		expect  []string
	}{
		{
			name: "fk",
			fields: []string{
				"`id` integer NOT NULL",
				"CONSTRAINT `fk_users_notes` FOREIGN KEY (`user_id`) " +
					"REFERENCES `users`(`id`))",
			},
			cName:   "fk_users_notes",
			success: true,
			expect:  []string{"`id` integer NOT NULL"},
		},
		{
			name: "check",
			fields: []string{
				"CONSTRAINT `name_checker` CHECK (`name` <> 'thetadev')",
				"`id` integer NOT NULL",
			},
			cName:   "name_checker",
			success: true,
			expect:  []string{"`id` integer NOT NULL"},
		},
		{
			name: "none",
			fields: []string{
				"CONSTRAINT `name_checker` CHECK (`name` <> 'thetadev')",
				"`id` integer NOT NULL",
			},
			cName:   "nothing",
			success: false,
			expect: []string{
				"CONSTRAINT `name_checker` CHECK (`name` <> 'thetadev')",
				"`id` integer NOT NULL",
			},
		},
	}

	for _, p := range params {
		t.Run(p.name, func(t *testing.T) {
			testDDL := ddl{fields: p.fields}
			success := testDDL.removeConstraint(p.cName)
			assert.Equal(t, p.success, success)
			assert.Equal(t, p.expect, testDDL.fields)
		})
	}
}

func TestRemoveColumn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fields  []string
		column  string
		success bool
		expect  []string
	}{
		{
			name:    "backtick_quoted",
			fields:  []string{"`id` integer NOT NULL", "`name` text"},
			column:  "name",
			success: true,
			expect:  []string{"`id` integer NOT NULL"},
		},
		{
			name:    "double_quoted",
			fields:  []string{`"id" integer NOT NULL`, `"name" text`},
			column:  "name",
			success: true,
			expect:  []string{`"id" integer NOT NULL`},
		},
		{
			name:    "single_quoted",
			fields:  []string{"'id' integer NOT NULL", "'name' text"},
			column:  "name",
			success: true,
			expect:  []string{"'id' integer NOT NULL"},
		},
		{
			name:    "unquoted",
			fields:  []string{"id integer NOT NULL", "name text"},
			column:  "name",
			success: true,
			expect:  []string{"id integer NOT NULL"},
		},
		{
			name:    "not_found",
			fields:  []string{"`id` integer NOT NULL", "`name` text"},
			column:  "missing",
			success: false,
			expect:  []string{"`id` integer NOT NULL", "`name` text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ddl{fields: tt.fields}
			ok := d.removeColumn(tt.column)
			assert.Equal(t, tt.success, ok)
			assert.Equal(t, tt.expect, d.fields)
		})
	}
}

func TestClone(t *testing.T) {
	t.Parallel()
	original := &ddl{
		head:   "CREATE TABLE `test`",
		fields: []string{"`id` integer NOT NULL", "`name` text"},
		columns: []migrator.ColumnType{
			{NameValue: sql.NullString{String: "id", Valid: true}},
		},
	}

	cloned := original.clone()

	// Values should be equal
	assert.Equal(t, original.head, cloned.head)
	assert.Equal(t, original.fields, cloned.fields)
	assert.Equal(t, original.columns, cloned.columns)

	// Mutating the clone should not affect the original
	cloned.fields[0] = "changed"
	assert.NotEqual(t, original.fields[0], cloned.fields[0])
}

func TestRenameTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		head    string
		src     string
		dst     string
		wantErr bool
	}{
		{
			name: "backtick_quoted",
			head: "CREATE TABLE `users`",
			src:  "users",
			dst:  "users__temp",
		},
		{
			name: "double_quoted",
			head: `CREATE TABLE "users"`,
			src:  "users",
			dst:  "users__temp",
		},
		{
			name: "unquoted",
			head: "CREATE TABLE users",
			src:  "users",
			dst:  "users__temp",
		},
		{
			name:    "not_found",
			head:    "CREATE TABLE `something`",
			src:     "other",
			dst:     "other__temp",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &ddl{head: tt.head}
			err := d.renameTable(tt.dst, tt.src)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, d.head, tt.dst)
			}
		})
	}
}

func TestGetColumns(t *testing.T) {
	t.Parallel()
	params := []struct {
		name    string
		ddl     string
		columns []string
	}{
		{
			name: "with_fk",
			ddl: "CREATE TABLE `notes` (" +
				"`id` integer NOT NULL,`text` varchar(500),`user_id` integer," +
				"PRIMARY KEY (`id`)," +
				"CONSTRAINT `fk_users_notes` FOREIGN KEY (`user_id`) " +
				"REFERENCES `users`(`id`))",
			columns: []string{"`id`", "`text`", "`user_id`"},
		},
		{
			name: "with_check",
			ddl: "CREATE TABLE Persons (" +
				"ID int NOT NULL,LastName varchar(255) NOT NULL," +
				"FirstName varchar(255),Age int," +
				"CHECK (Age>=18),CHECK (FirstName!='John'))",
			columns: []string{"`ID`", "`LastName`", "`FirstName`", "`Age`"},
		},
		{
			name: "with_escaped_quote",
			ddl: "CREATE TABLE Persons (" +
				"ID int NOT NULL," +
				"LastName varchar(255) NOT NULL DEFAULT \"\"," +
				"FirstName varchar(255))",
			columns: []string{"`ID`", "`LastName`", "`FirstName`"},
		},
		{
			name: "with_generated_column",
			ddl: "CREATE TABLE Persons (" +
				"ID int NOT NULL,LastName varchar(255) NOT NULL," +
				"FirstName varchar(255)," +
				"FullName varchar(255) GENERATED ALWAYS AS " +
				"(FirstName || ' ' || LastName))",
			columns: []string{"`ID`", "`LastName`", "`FirstName`"},
		},
		{
			name: "with_new_line",
			ddl: "CREATE TABLE \"tb_sys_role_menu__temp\" (\n" +
				"  \"id\" integer  PRIMARY KEY AUTOINCREMENT,\n" +
				"  \"created_at\" datetime NOT NULL,\n" +
				"  \"updated_at\" datetime NOT NULL,\n" +
				"  \"created_by\" integer NOT NULL DEFAULT 0,\n" +
				"  \"updated_by\" integer NOT NULL DEFAULT 0,\n" +
				"  \"role_id\" integer NOT NULL,\n" +
				"  \"menu_id\" bigint NOT NULL\n" +
				")",
			columns: []string{
				"`id`", "`created_at`", "`updated_at`",
				"`created_by`", "`updated_by`", "`role_id`", "`menu_id`",
			},
		},
	}

	for _, p := range params {
		t.Run(p.name, func(t *testing.T) {
			testDDL, err := parseDDL(p.ddl)
			require.NoError(t, err)
			assert.Equal(t, p.columns, testDDL.getColumns())
		})
	}
}
