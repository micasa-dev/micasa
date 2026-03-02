// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"strings"
)

// EntityRow is a lightweight (id, name) pair for FK context in LLM prompts.
type EntityRow struct {
	ID   uint
	Name string
}

// SchemaContext provides the schema and entity data the LLM needs to generate
// correct operations against the database.
type SchemaContext struct {
	DDL                   map[string]string // table name -> CREATE TABLE SQL
	Vendors               []EntityRow
	Projects              []EntityRow
	Appliances            []EntityRow
	MaintenanceCategories []EntityRow
	ProjectTypes          []EntityRow
}

// AllowedOps specifies which operations are permitted on a table.
// Insert maps to "create", Update maps to "update".
type AllowedOps struct {
	Insert bool
	Update bool
}

// ExtractionTables is the set of tables the LLM receives DDL for and may
// reference in its output.
var ExtractionTables = []string{
	"documents",
	"vendors",
	"quotes",
	"maintenance_items",
	"appliances",
	"projects",
	"project_types",
	"maintenance_categories",
}

// ExtractionAllowedOps defines which operations the LLM may perform on
// each table. Used by ValidateOperations.
var ExtractionAllowedOps = map[string]AllowedOps{
	"documents":         {Insert: true, Update: true},
	"vendors":           {Insert: true},
	"quotes":            {Insert: true},
	"maintenance_items": {Insert: true},
	"appliances":        {Insert: true},
}

// FormatDDLBlock formats the DDL map as a SQL comment block for inclusion
// in the LLM system prompt.
func FormatDDLBlock(ddl map[string]string, tables []string) string {
	var b strings.Builder
	first := true
	for _, name := range tables {
		sql, ok := ddl[name]
		if !ok {
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		b.WriteString(sql)
		b.WriteString(";\n")
		first = false
	}
	return b.String()
}

// FormatEntityRows formats a named set of entity rows as SQL comments
// for inclusion in the LLM system prompt.
func FormatEntityRows(label string, rows []EntityRow) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "-- %s (id, name)\n", label)
	for _, r := range rows {
		fmt.Fprintf(&b, "-- %d, %s\n", r.ID, r.Name)
	}
	return b.String()
}
