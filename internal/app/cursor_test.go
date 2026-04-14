// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPointerShape_SkipsRedundantWrite(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	result := setPointerShape(&buf, pointerShapePointer, pointerShapePointer)
	assert.Equal(t, pointerShapePointer, result)
	assert.Empty(t, buf.String(), "should not write when shape unchanged")
}

func TestSetPointerShape_WritesOnChange(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	result := setPointerShape(&buf, pointerShapePointer, pointerShapeDefault)
	assert.Equal(t, pointerShapePointer, result)
	assert.Equal(t, "\x1b]22;pointer\x1b\\", buf.String())
}

func TestResetPointerShape(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	resetPointerShape(&buf)
	assert.Equal(t, "\x1b]22;\x1b\\", buf.String())
}

func TestHoverOverTab_SetsPointerShape(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneTab+"0")
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
	assert.Contains(t, buf.String(), "pointer")
}

func TestHoverOffZone_ResetsPointerShape(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneTab+"0")

	sendMouseMotion(m, z.StartX, z.StartY)
	require.Equal(t, pointerShapePointer, m.lastPointerShape)

	// Move far off any zone (beyond terminal dimensions).
	buf.Reset()
	sendMouseMotion(m, m.width+100, m.height+100)
	assert.Equal(t, pointerShapeDefault, m.lastPointerShape)
	assert.Contains(t, buf.String(), "\x1b]22;\x1b\\")
}

func TestHoverOverRow_SetsPointerShape(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	tab := m.activeTab()
	require.NotEmpty(t, tab.CellRows)

	z := requireZone(t, m, zoneRow+"0")
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestHoverOverHint_SetsPointerShape(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneHint+"help")
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestHoverOverHouseHeader_SetsPointerShape(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneHouse)
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestHoverOverColumnHeader_SetsPointerShape(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneCol+"0")
	sendMouseMotion(m, z.StartX, z.StartY)

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestHoverRedundantMotion_NoExtraWrite(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneTab+"0")

	sendMouseMotion(m, z.StartX, z.StartY)
	firstLen := buf.Len()
	require.Positive(t, firstLen)

	// Second motion on same zone -- no additional write.
	sendMouseMotion(m, z.StartX+1, z.StartY)
	assert.Equal(t, firstLen, buf.Len(), "redundant motion should not write")
}

func TestQuitResetsPointerShape(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneTab+"0")
	sendMouseMotion(m, z.StartX, z.StartY)
	require.Equal(t, pointerShapePointer, m.lastPointerShape)

	buf.Reset()

	sendKey(m, keyCtrlQ)

	assert.Contains(t, buf.String(), "\x1b]22;\x1b\\", "quit should reset pointer")
}

func TestConfirmQuitResetsPointerShape(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	openAddForm(m)
	require.Equal(t, modeForm, m.mode)
	m.fs.formDirty = true

	m.lastPointerShape = pointerShapePointer

	buf.Reset()

	sendKey(m, keyCtrlQ)
	require.Equal(t, confirmFormQuitDiscard, m.confirm)

	sendKey(m, keyY)

	assert.Contains(t, buf.String(), "\x1b]22;\x1b\\", "confirm quit should reset pointer")
}

func TestMouseMotionMsg_DispatchedInUpdate(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	z := requireZone(t, m, zoneTab+"0")
	m.Update(tea.MouseMotionMsg{X: z.StartX, Y: z.StartY})

	assert.Equal(t, pointerShapePointer, m.lastPointerShape)
}

func TestViewUsesAllMotion(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	v := m.View()
	assert.Equal(t, tea.MouseModeAllMotion, v.MouseMode)
}

func TestOverlayBlocksBaseZones(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	var buf bytes.Buffer
	m.pointerWriter = &buf

	m.showDashboard = true
	_ = m.loadDashboardAt(time.Now())

	if !m.dashboardVisible() {
		t.Skip("dashboard has no data")
	}

	tabZone := requireZone(t, m, zoneTab+"0")

	sendMouseMotion(m, tabZone.StartX, tabZone.StartY)
	assert.Equal(t, pointerShapeDefault, m.lastPointerShape,
		"base zones should not be clickable when overlay is active")
}
