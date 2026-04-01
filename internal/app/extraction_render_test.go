// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewColumnsVendorFormatsPhone(t *testing.T) {
	// Not parallel: t.Setenv modifies process-global state.
	// LC_ALL has highest precedence in DetectCountry().
	t.Setenv("LC_ALL", "en_US.UTF-8")
	cur, err := locale.ResolveDefault("")
	require.NoError(t, err)
	cols := previewColumns(data.TableVendors, cur)

	// Find the phone column by key.
	var phoneFmt func(any) string
	for _, c := range cols {
		if c.dataKey == data.ColPhone {
			phoneFmt = c.format
			break
		}
	}
	require.NotNil(t, phoneFmt, "phone column not found in vendor preview")

	// Should format parseable numbers, passthrough garbage.
	assert.Equal(t, "(555) 123-4567", phoneFmt("5551234567"))
	assert.Equal(t, "not a phone", phoneFmt("not a phone"))
}

func TestGroupOperationsByTableUsesRowLocale(t *testing.T) {
	// Not parallel: t.Setenv modifies process-global state.
	t.Setenv("LC_ALL", "en_US.UTF-8")
	cur, err := locale.ResolveDefault("")
	require.NoError(t, err)

	ops := []extract.Operation{
		{
			Action: extract.ActionCreate,
			Table:  data.TableVendors,
			Data: map[string]any{
				data.ColName:   "UK Plumber",
				data.ColPhone:  "02079460958",
				data.ColLocale: "GB",
			},
		},
		{
			Action: extract.ActionCreate,
			Table:  data.TableVendors,
			Data: map[string]any{
				data.ColName:  "US Plumber",
				data.ColPhone: "5551234567",
			},
		},
	}

	groups := groupOperationsByTable(ops, cur)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].cells, 2)

	// Find phone column index.
	phoneIdx := -1
	for i, spec := range groups[0].specs {
		if spec.Title == "Phone" {
			phoneIdx = i
			break
		}
	}
	require.NotEqual(t, -1, phoneIdx, "phone column not found")

	// UK vendor should use GB locale formatting.
	assert.Equal(t, "020 7946 0958", groups[0].cells[0][phoneIdx].Value)
	// US vendor (no locale) should fall back to system default (US).
	assert.Equal(t, "(555) 123-4567", groups[0].cells[1][phoneIdx].Value)
}
