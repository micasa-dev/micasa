// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatKSqft(t *testing.T) {
	t.Parallel()
	tests := []struct {
		sqft int
		want string
	}{
		{0, ""},
		{850, "850"},
		{1200, "1.2k"},
		{1950, "2k"},
		{2400, "2.4k"},
		{5000, "5k"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatKSqft(tt.sqft), "sqft=%d", tt.sqft)
	}
}

func TestHouseEmptyFieldCount(t *testing.T) {
	t.Parallel()
	defs := houseFieldDefs()
	m := newTestModelWithDemoData(t, 42)
	p := emptyHouseProfile("Test")
	count := houseEmptyFieldCount(p, m.cur, m.unitSystem)
	// Toggle fields are excluded (always have a value). Nickname is set.
	toggleCount := 0
	for _, d := range defs {
		if d.toggle != nil {
			toggleCount++
		}
	}
	assert.Equal(t, len(defs)-1-toggleCount, count)
}

func TestHouseEmptyFieldCountFullProfile(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	count := houseEmptyFieldCount(m.house, m.cur, m.unitSystem)
	// Demo data fills many fields; empty count should be less than total.
	assert.Less(t, count, len(houseFieldDefs()))
}

func TestHouseCollapsedNicknamePill(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	view := m.houseCollapsed()
	assert.Contains(t, view, m.house.Nickname, "should show nickname")
	assert.NotContains(t, view, "House", "should not show House label")
}

func TestHouseCollapsedNoHouseStillShowsHouse(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.hasHouse = false
	view := m.houseView()
	assert.Contains(t, view, "House", "no-house state should show House pill")
	assert.Contains(t, view, "setup", "no-house state should show setup badge")
}
