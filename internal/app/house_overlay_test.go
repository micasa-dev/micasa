// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHouseOverlayToggle(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	assert.Nil(t, m.houseOverlay, "overlay should start nil")

	sendKey(m, keyTab)
	assert.NotNil(t, m.houseOverlay, "tab should open overlay")

	sendKey(m, keyTab)
	assert.Nil(t, m.houseOverlay, "tab should close overlay")
}

func TestHouseOverlayMouseToggle(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	assert.Nil(t, m.houseOverlay)

	z := requireZone(t, m, zoneHouse)

	sendClick(m, z.StartX, z.StartY)
	assert.NotNil(t, m.houseOverlay, "click should open overlay")

	// Wait for overlay zone to flush so click dispatch uses known bounds.
	m.View()
	require.Eventually(t, func() bool {
		oz := m.zones.Get(zoneOverlay)
		return oz != nil && !oz.IsZero()
	}, 2*time.Second, time.Millisecond, "overlay zone never populated")

	// Click at (0,0) — outside centered overlay — should dismiss.
	sendClick(m, 0, 0)
	assert.Nil(t, m.houseOverlay, "click outside overlay should close it")
}

func TestHouseOverlayEscCloses(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	require.NotNil(t, m.houseOverlay)

	sendKey(m, keyEsc)
	assert.Nil(t, m.houseOverlay, "esc should close overlay")
}

func TestHouseOverlayRenders(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	view := m.buildView()
	assert.Contains(t, view, "Structure")
	assert.Contains(t, view, "Utilities")
	assert.Contains(t, view, "Financial")
	assert.Contains(t, view, m.house.Nickname)
}

func TestHouseOverlayNoHouseNoOpen(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.hasHouse = false
	sendKey(m, keyTab)
	assert.Nil(t, m.houseOverlay, "tab should not open overlay without house")
}

func TestHouseOverlayNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab) // open overlay

	// Starts at first structure field (section=1, row=0).
	require.NotNil(t, m.houseOverlay)
	assert.Equal(t, 1, m.houseOverlay.section)
	assert.Equal(t, 0, m.houseOverlay.row)

	// Down moves within column.
	sendKey(m, keyDown)
	assert.Equal(t, 1, m.houseOverlay.section)
	assert.Equal(t, 1, m.houseOverlay.row)

	// Right jumps to utilities column.
	sendKey(m, keyRight)
	assert.Equal(t, 2, m.houseOverlay.section)

	// Right again to financial.
	sendKey(m, keyRight)
	assert.Equal(t, 3, m.houseOverlay.section)

	// Right at rightmost column clamps.
	sendKey(m, keyRight)
	assert.Equal(t, 3, m.houseOverlay.section)

	// Up from row 0 in grid moves to identity section.
	m.houseOverlay.section = 1
	m.houseOverlay.row = 0
	sendKey(m, keyUp)
	assert.Equal(t, 0, m.houseOverlay.section, "should enter identity section")
}

func TestHouseOverlayRowClamping(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)

	// Move deep into structure column (longest).
	defs := houseFieldDefs()
	structLen := 0
	for _, d := range defs {
		if d.section == houseSectionStructure {
			structLen++
		}
	}
	for range structLen - 1 {
		sendKey(m, keyDown)
	}
	assert.Equal(t, structLen-1, m.houseOverlay.row)

	// Jump to utilities (shorter) -- row should clamp.
	sendKey(m, keyRight)
	utilLen := 0
	for _, d := range defs {
		if d.section == houseSectionUtilities {
			utilLen++
		}
	}
	assert.LessOrEqual(t, m.houseOverlay.row, utilLen-1)
}

func TestHouseOverlayIdentityNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)

	// Move to identity section.
	m.houseOverlay.section = 0
	m.houseOverlay.row = 0

	// Count identity fields.
	defs := houseFieldDefs()
	identLen := 0
	for _, d := range defs {
		if d.section == houseSectionIdentity {
			identLen++
		}
	}

	// Right in identity cycles through identity fields.
	sendKey(m, keyRight)
	assert.Equal(t, 0, m.houseOverlay.section, "should stay in identity")
	assert.Equal(t, 1, m.houseOverlay.row)

	// Right at last identity field clamps.
	m.houseOverlay.row = identLen - 1
	sendKey(m, keyRight)
	assert.Equal(t, 0, m.houseOverlay.section)
	assert.Equal(t, identLen-1, m.houseOverlay.row)

	// Left in identity cycles backward.
	m.houseOverlay.row = 1
	sendKey(m, keyLeft)
	assert.Equal(t, 0, m.houseOverlay.section)
	assert.Equal(t, 0, m.houseOverlay.row)

	// Left at row 0 in identity clamps.
	sendKey(m, keyLeft)
	assert.Equal(t, 0, m.houseOverlay.section)
	assert.Equal(t, 0, m.houseOverlay.row)

	// Down from identity moves to structure section.
	sendKey(m, keyDown)
	assert.Equal(t, 1, m.houseOverlay.section)
	assert.Equal(t, 0, m.houseOverlay.row)
}

func TestHouseOverlayLeftFromGrid(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)

	// Start at utilities.
	m.houseOverlay.section = 2
	m.houseOverlay.row = 0

	// Left goes to structure.
	sendKey(m, keyLeft)
	assert.Equal(t, 1, m.houseOverlay.section)

	// Left from structure goes to identity.
	sendKey(m, keyLeft)
	assert.Equal(t, 0, m.houseOverlay.section)
}

func TestHouseOverlayDownClamps(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)

	// Count structure fields.
	defs := houseFieldDefs()
	structLen := 0
	for _, d := range defs {
		if d.section == houseSectionStructure {
			structLen++
		}
	}

	// Move to last row in structure.
	m.houseOverlay.section = 1
	m.houseOverlay.row = structLen - 1

	// Down at bottom clamps.
	sendKey(m, keyDown)
	assert.Equal(t, structLen-1, m.houseOverlay.row)
}

func TestHouseOverlayVimKeys(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	require.NotNil(t, m.houseOverlay)

	// j moves down.
	sendKey(m, keyJ)
	assert.Equal(t, 1, m.houseOverlay.row)

	// k moves up.
	sendKey(m, keyK)
	assert.Equal(t, 0, m.houseOverlay.row)

	// l moves right to utilities.
	sendKey(m, keyL)
	assert.Equal(t, 2, m.houseOverlay.section)

	// h moves left to structure.
	sendKey(m, keyH)
	assert.Equal(t, 1, m.houseOverlay.section)
}

func TestHouseOverlayFieldZones(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	_ = m.buildView()
	// Grid fields (structure, utilities, financial) are zone-marked.
	// Identity fields live in the header line, not the column grid.
	requireZone(t, m, zoneHouseField+"year_built")
	requireZone(t, m, zoneHouseField+"heating_type")
	requireZone(t, m, zoneHouseField+"insurance_carrier")
}

func TestHouseOverlayInlineEdit(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab) // open overlay

	// Starts at structure section row 0 (year_built).
	require.NotNil(t, m.houseOverlay)
	assert.False(t, m.houseOverlay.editing)

	// Enter to start editing.
	sendKey(m, keyEnter)
	assert.True(t, m.houseOverlay.editing)
	assert.NotNil(t, m.houseOverlay.form)

	// Esc to cancel edit (not close overlay).
	sendKey(m, keyEsc)
	assert.False(t, m.houseOverlay.editing)
	assert.Nil(t, m.houseOverlay.form)
	assert.NotNil(t, m.houseOverlay, "overlay should still be open after edit cancel")
}

func TestHouseOverlayEditPersists(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab) // open overlay

	// Navigate to identity section (nickname).
	m.houseOverlay.section = int(houseSectionIdentity)
	m.houseOverlay.row = 0
	sendKey(m, keyEnter) // start edit

	require.True(t, m.houseOverlay.editing)
	require.NotNil(t, m.houseOverlay.form)

	// Clear existing value and type new one.
	sendKey(m, "ctrl+e") // move to end
	sendKey(m, "ctrl+u") // kill line
	for _, r := range "Bungalow" {
		sendKey(m, string(r))
	}
	sendKey(m, keyEnter) // submit

	assert.False(t, m.houseOverlay.editing, "should exit edit mode")
	assert.Equal(t, "Bungalow", m.house.Nickname, "nickname should persist")
}

func TestHouseOverlayEditHintBar(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	require.NotNil(t, m.houseOverlay)

	// Not editing: hints should include "edit".
	view := m.buildHouseOverlay()
	assert.Contains(t, view, "edit")
	assert.Contains(t, view, "navigate")

	// Start editing.
	sendKey(m, keyEnter)
	require.True(t, m.houseOverlay.editing)

	view = m.buildHouseOverlay()
	assert.Contains(t, view, "confirm")
	assert.Contains(t, view, "cancel")
}

func TestHouseOverlayEditInvalidValue(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	require.NotNil(t, m.houseOverlay)

	// Cursor starts on year_built (structure row 0).
	assert.Equal(t, int(houseSectionStructure), m.houseOverlay.section)
	assert.Equal(t, 0, m.houseOverlay.row)

	original := m.house.YearBuilt
	sendKey(m, keyEnter) // start edit
	require.True(t, m.houseOverlay.editing)

	// Type invalid value.
	sendKey(m, "ctrl+e")
	sendKey(m, "ctrl+u")
	for _, r := range "notanumber" {
		sendKey(m, string(r))
	}
	sendKey(m, keyEnter) // attempt submit

	// Should still be editing (validation error shown in status).
	assert.True(t, m.houseOverlay.editing, "should remain in edit mode on validation error")
	assert.Equal(t, original, m.house.YearBuilt, "value should not change on error")
}

func TestHouseOverlayClickSelectsField(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	_ = m.buildView()

	// Click a field in utilities section.
	z := requireZone(t, m, zoneHouseField+"heating_type")
	sendClick(m, z.StartX+1, z.StartY)

	// Cursor should have moved to utilities section.
	assert.Equal(t, int(houseSectionUtilities), m.houseOverlay.section, "should be in utilities")
	assert.Equal(t, 0, m.houseOverlay.row, "should be first row in utilities")

	// Click a non-first field in structure column.
	z2 := requireZone(t, m, zoneHouseField+"bedrooms")
	sendClick(m, z2.StartX+1, z2.StartY)
	assert.Equal(t, int(houseSectionStructure), m.houseOverlay.section, "should be in structure")
	assert.Equal(t, 3, m.houseOverlay.row, "bedrooms is 4th structure field (index 3)")
}
