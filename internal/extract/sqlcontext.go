// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"strings"

	"github.com/cpcloud/micasa/internal/data"
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

// ColType is a JSON Schema type for a column.
type ColType string

const (
	ColTypeString  ColType = "string"
	ColTypeInteger ColType = "integer"
)

// ColumnDef describes a single column the LLM may write.
type ColumnDef struct {
	Name string
	Type ColType
	Enum []any // optional enum constraint (e.g. entity_kind values)
}

// ActionDef describes what an action can do on a table's columns.
type ActionDef struct {
	Action   Action
	Required []string    // columns required for this action
	Extra    []ColumnDef // columns only present for this action (e.g. id for update)
	Omit     []string    // columns from the table to exclude for this action
}

// TableDef defines a table's columns and which actions are allowed.
// Columns are defined once; each ActionDef specifies required fields
// and any action-specific extras.
type TableDef struct {
	Table   string
	Columns []ColumnDef // shared columns across all actions
	Actions []ActionDef
}

// ExtractionTableDefs is the single source of truth for extraction table
// metadata. Column sets match what each table's commit function in
// shadow.go consumes.
var ExtractionTableDefs = []TableDef{
	{
		Table: data.TableVendors,
		Columns: []ColumnDef{
			{Name: "name", Type: ColTypeString},
			{Name: "contact_name", Type: ColTypeString},
			{Name: "email", Type: ColTypeString},
			{Name: "phone", Type: ColTypeString},
			{Name: "website", Type: ColTypeString},
			{Name: "notes", Type: ColTypeString},
		},
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"name"}},
		},
	},
	{
		Table: data.TableAppliances,
		Columns: []ColumnDef{
			{Name: "name", Type: ColTypeString},
			{Name: "brand", Type: ColTypeString},
			{Name: "model_number", Type: ColTypeString},
			{Name: "serial_number", Type: ColTypeString},
			{Name: "location", Type: ColTypeString},
			{Name: "cost_cents", Type: ColTypeInteger},
			{Name: "notes", Type: ColTypeString},
		},
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"name"}},
		},
	},
	{
		Table: data.TableQuotes,
		Columns: []ColumnDef{
			{Name: "project_id", Type: ColTypeInteger},
			{Name: "vendor_id", Type: ColTypeInteger},
			{Name: "vendor_name", Type: ColTypeString},
			{Name: "total_cents", Type: ColTypeInteger},
			{Name: "labor_cents", Type: ColTypeInteger},
			{Name: "materials_cents", Type: ColTypeInteger},
			{Name: "notes", Type: ColTypeString},
		},
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"total_cents"}},
		},
	},
	{
		Table: data.TableMaintenanceItems,
		Columns: []ColumnDef{
			{Name: "name", Type: ColTypeString},
			{Name: "category_id", Type: ColTypeInteger},
			{Name: "appliance_id", Type: ColTypeInteger},
			{Name: "interval_months", Type: ColTypeInteger},
			{Name: "cost_cents", Type: ColTypeInteger},
			{Name: "notes", Type: ColTypeString},
		},
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"name"}},
			{Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
				{Name: "id", Type: ColTypeInteger},
			}},
		},
	},
	{
		Table: data.TableDocuments,
		Columns: []ColumnDef{
			{Name: "title", Type: ColTypeString},
			{Name: "notes", Type: ColTypeString},
			{Name: "entity_kind", Type: ColTypeString, Enum: []any{
				data.DocumentEntityProject,
				data.DocumentEntityQuote,
				data.DocumentEntityMaintenance,
				data.DocumentEntityAppliance,
				data.DocumentEntityServiceLog,
				data.DocumentEntityVendor,
				data.DocumentEntityIncident,
			}},
			{Name: "entity_id", Type: ColTypeInteger},
			{Name: "file_name", Type: ColTypeString},
		},
		Actions: []ActionDef{
			{Action: ActionCreate},
			{Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
				{Name: "id", Type: ColTypeInteger},
			}, Omit: []string{"file_name"}},
		},
	},
}

// TableOp is a flattened {action, table, columns} triple expanded from
// TableDef. Used by the schema builder and derived maps.
type TableOp struct {
	Action  Action
	Table   string
	Columns []flatColumn
}

type flatColumn struct {
	Name     string
	Type     ColType
	Required bool
	Enum     []any
}

// ExtractionOps expands ExtractionTableDefs into flat TableOp entries.
var ExtractionOps = func() []TableOp {
	var ops []TableOp
	for _, td := range ExtractionTableDefs {
		for _, ad := range td.Actions {
			ops = append(ops, expandTableOp(td, ad))
		}
	}
	return ops
}()

func expandTableOp(td TableDef, ad ActionDef) TableOp {
	omit := make(map[string]bool, len(ad.Omit))
	for _, name := range ad.Omit {
		omit[name] = true
	}
	reqSet := make(map[string]bool, len(ad.Required))
	for _, name := range ad.Required {
		reqSet[name] = true
	}

	var cols []flatColumn
	for _, extra := range ad.Extra {
		cols = append(cols, flatColumn{
			Name:     extra.Name,
			Type:     extra.Type,
			Required: reqSet[extra.Name],
			Enum:     extra.Enum,
		})
	}
	for _, col := range td.Columns {
		if omit[col.Name] {
			continue
		}
		cols = append(cols, flatColumn{
			Name:     col.Name,
			Type:     col.Type,
			Required: reqSet[col.Name],
			Enum:     col.Enum,
		})
	}
	return TableOp{Action: ad.Action, Table: td.Table, Columns: cols}
}

// ExtractionAllowedOps is derived from ExtractionOps for use by
// ValidateOperations.
var ExtractionAllowedOps = func() map[string]AllowedOps {
	m := make(map[string]AllowedOps)
	for _, op := range ExtractionOps {
		a := m[op.Table]
		switch op.Action {
		case ActionCreate:
			a.Insert = true
		case ActionUpdate:
			a.Update = true
		}
		m[op.Table] = a
	}
	return m
}()

// ExtractionTables is the set of tables the LLM receives DDL for and may
// reference in its output. Includes both writable and read-only reference
// tables.
var ExtractionTables = []string{
	data.TableDocuments,
	data.TableVendors,
	data.TableQuotes,
	data.TableMaintenanceItems,
	data.TableAppliances,
	data.TableProjects,
	data.TableProjectTypes,
	data.TableMaintenanceCategories,
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
