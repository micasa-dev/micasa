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
