// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cpcloud/micasa/internal/data"
)

// EntityRow is a lightweight (id, name) pair for FK context in LLM prompts.
type EntityRow struct {
	ID   string
	Name string
}

// SchemaContext provides the schema and entity data the LLM needs to generate
// correct operations against the database.
type SchemaContext struct {
	DDL                   map[string]string // table name -> CREATE TABLE SQL
	Vendors               []EntityRow
	Projects              []EntityRow
	Appliances            []EntityRow
	MaintenanceItems      []EntityRow
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
// Columns are derived from generated model metadata via columnsFromMeta;
// each ActionDef specifies required fields and any action-specific extras.
// Table-wide column exclusions are controlled by the extract:"-" struct tag
// on model fields, which causes genmeta to omit them from TableExtractColumns.
type TableDef struct {
	Table   string
	Columns []ColumnDef // shared columns across all actions
	Actions []ActionDef
}

// columnsFromMeta converts generated column metadata for a table into
// ColumnDefs. Panics if the table has no generated metadata -- this is
// intentional since extraction tables must correspond to real models.
func columnsFromMeta(table string) []ColumnDef {
	metas, ok := data.TableExtractColumns[table]
	if !ok {
		panic(fmt.Sprintf("no generated columns for table %q", table))
	}
	cols := make([]ColumnDef, len(metas))
	for i, m := range metas {
		cols[i] = ColumnDef{Name: m.Name, Type: ColType(m.JSONType)}
	}
	return cols
}

// withEnum returns a copy of cols where the named column gets the given
// enum constraint. Panics if the column is not found.
func withEnum(cols []ColumnDef, name string, values []any) []ColumnDef {
	result := slices.Clone(cols)
	for i := range result {
		if result[i].Name == name {
			result[i].Enum = values
			return result
		}
	}
	panic(fmt.Sprintf("withEnum: column %q not found", name))
}

// ExtractionTableDefs is the single source of truth for extraction table
// metadata. Column lists are derived from generated model metadata via
// columnsFromMeta; only policy annotations (Actions, Required, Enum, Omit,
// synthetic columns) are hand-maintained.
var ExtractionTableDefs = []TableDef{
	{
		Table:   data.TableVendors,
		Columns: columnsFromMeta(data.TableVendors),
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"name"}},
			{Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
				{Name: "id", Type: ColTypeString},
			}},
		},
	},
	{
		Table:   data.TableAppliances,
		Columns: columnsFromMeta(data.TableAppliances),
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"name"}},
			{Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
				{Name: "id", Type: ColTypeString},
			}},
		},
	},
	{
		Table: data.TableProjects,
		Columns: withEnum(
			columnsFromMeta(data.TableProjects),
			"status", []any{
				data.ProjectStatusIdeating,
				data.ProjectStatusPlanned,
				data.ProjectStatusQuoted,
				data.ProjectStatusInProgress,
				data.ProjectStatusDelayed,
				data.ProjectStatusCompleted,
				data.ProjectStatusAbandoned,
			},
		),
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"title"}},
		},
	},
	{
		Table: data.TableQuotes,
		Columns: append(
			columnsFromMeta(data.TableQuotes),
			ColumnDef{Name: "vendor_name", Type: ColTypeString},
		),
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"project_id", "total_cents"}},
			{Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
				{Name: "id", Type: ColTypeString},
			}},
		},
	},
	{
		Table:   data.TableMaintenanceItems,
		Columns: columnsFromMeta(data.TableMaintenanceItems),
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"name"}},
			{Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
				{Name: "id", Type: ColTypeString},
			}},
		},
	},
	{
		Table: data.TableIncidents,
		Columns: append(
			withEnum(
				withEnum(
					columnsFromMeta(data.TableIncidents),
					"status", []any{
						data.IncidentStatusOpen,
						data.IncidentStatusInProgress,
						data.IncidentStatusResolved,
					},
				),
				"severity", []any{
					data.IncidentSeverityUrgent,
					data.IncidentSeveritySoon,
					data.IncidentSeverityWhenever,
				},
			),
			ColumnDef{Name: "vendor_name", Type: ColTypeString},
		),
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"title"}},
		},
	},
	{
		Table: data.TableServiceLogEntries,
		Columns: append(
			columnsFromMeta(data.TableServiceLogEntries),
			ColumnDef{Name: "vendor_name", Type: ColTypeString},
		),
		Actions: []ActionDef{
			{Action: ActionCreate, Required: []string{"maintenance_item_id"}},
		},
	},
	{
		Table: data.TableDocuments,
		Columns: withEnum(
			columnsFromMeta(data.TableDocuments),
			"entity_kind", []any{
				data.DocumentEntityProject,
				data.DocumentEntityQuote,
				data.DocumentEntityMaintenance,
				data.DocumentEntityAppliance,
				data.DocumentEntityServiceLog,
				data.DocumentEntityVendor,
				data.DocumentEntityIncident,
			},
		),
		Actions: []ActionDef{
			{Action: ActionCreate},
			{Action: ActionUpdate, Required: []string{"id"}, Extra: []ColumnDef{
				{Name: "id", Type: ColTypeString},
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
	data.TableIncidents,
	data.TableServiceLogEntries,
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
		fmt.Fprintf(&b, "-- %s, %s\n", r.ID, r.Name)
	}
	return b.String()
}
