// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"testing"

	"github.com/cpcloud/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractionColumns_MatchGenerated verifies that every non-synthetic
// column in ExtractionTableDefs exists in the generated TableExtractColumns
// with a matching JSON Schema type. Catches drift between models.go and
// extraction config.
func TestExtractionColumns_MatchGenerated(t *testing.T) {
	t.Parallel()

	// Synthetic columns exist only in the extraction layer, not in models.
	synthetic := map[string]bool{
		"vendor_name": true,
	}

	for _, td := range ExtractionTableDefs {
		meta := data.TableExtractColumns[td.Table]
		require.NotEmpty(t, meta, "no generated columns for table %q", td.Table)

		metaByName := make(map[string]string, len(meta))
		for _, m := range meta {
			metaByName[m.Name] = m.JSONType
		}

		for _, col := range td.Columns {
			if synthetic[col.Name] {
				continue
			}
			metaType, ok := metaByName[col.Name]
			assert.True(t, ok,
				"table %q column %q is in ExtractionTableDefs but not in generated metadata",
				td.Table, col.Name,
			)
			if ok {
				assert.Equal(t, metaType, string(col.Type),
					"table %q column %q type mismatch", td.Table, col.Name,
				)
			}
		}
	}
}

// TestExtractionColumns_ActionOmitColumnsExist verifies that every column
// listed in an ActionDef.Omit actually exists in the generated metadata or
// the table's Columns list. A stale Omit entry (referencing a renamed or
// removed column) is silently ignored -- this test catches that.
func TestExtractionColumns_ActionOmitColumnsExist(t *testing.T) {
	t.Parallel()

	for _, td := range ExtractionTableDefs {
		metaNames := make(map[string]bool)
		for _, m := range data.TableExtractColumns[td.Table] {
			metaNames[m.Name] = true
		}

		for _, ad := range td.Actions {
			for _, name := range ad.Omit {
				// Action-level Omit may reference generated columns or
				// synthetic columns that are in td.Columns.
				inMeta := metaNames[name]
				inColumns := false
				for _, col := range td.Columns {
					if col.Name == name {
						inColumns = true
						break
					}
				}
				assert.True(
					t,
					inMeta || inColumns,
					"table %q action %q has Omit for %q which is not in generated metadata or Columns",
					td.Table,
					ad.Action,
					name,
				)
			}
		}
	}
}

// TestExpandTableOp_ActionOmit verifies that expandTableOp excludes
// action-level Omit columns.
func TestExpandTableOp_ActionOmit(t *testing.T) {
	t.Parallel()

	td := TableDef{
		Table: "test",
		Columns: []ColumnDef{
			{Name: "a", Type: ColTypeString},
			{Name: "b", Type: ColTypeString},
			{Name: "c", Type: ColTypeString},
		},
		Actions: []ActionDef{
			{Action: ActionCreate, Omit: []string{"b"}},
		},
	}

	op := expandTableOp(td, td.Actions[0])

	colNames := make(map[string]bool)
	for _, col := range op.Columns {
		colNames[col.Name] = true
	}

	assert.True(t, colNames["a"], "'a' should be present")
	assert.False(t, colNames["b"], "action-level Omit should exclude 'b'")
	assert.True(t, colNames["c"], "'c' should be present")
}
