// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
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
