// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithOrdinals(t *testing.T) {
	t.Parallel()
	t.Run("prefixes string options", func(t *testing.T) {
		opts := withOrdinals([]huh.Option[string]{
			huh.NewOption("alpha", "a"),
			huh.NewOption("beta", "b"),
			huh.NewOption("gamma", "c"),
		})
		want := []string{"1. alpha", "2. beta", "3. gamma"}
		for i, opt := range opts {
			assert.Equalf(t, want[i], opt.Key, "option %d", i)
		}
	})

	t.Run("prefixes uint options", func(t *testing.T) {
		opts := withOrdinals([]huh.Option[uint]{
			huh.NewOption("First", uint(1)),
			huh.NewOption("Second", uint(2)),
		})
		assert.Equal(t, "1. First", opts[0].Key)
		assert.Equal(t, "2. Second", opts[1].Key)
	})

	t.Run("double-digit ordinals", func(t *testing.T) {
		opts := make([]huh.Option[string], 12)
		for i := range opts {
			opts[i] = huh.NewOption("item", "v")
		}
		opts = withOrdinals(opts)
		assert.Equal(t, "10. item", opts[9].Key)
		assert.Equal(t, "12. item", opts[11].Key)
	})
}

func TestSelectOrdinal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		msg    tea.KeyMsg
		wantN  int
		wantOk bool
	}{
		{
			name:   "key 1",
			msg:    tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}},
			wantN:  1,
			wantOk: true,
		},
		{
			name:   "key 5",
			msg:    tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}},
			wantN:  5,
			wantOk: true,
		},
		{
			name:   "key 9",
			msg:    tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}},
			wantN:  9,
			wantOk: true,
		},
		{
			name:   "key 0 is not an ordinal",
			msg:    tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}},
			wantN:  0,
			wantOk: false,
		},
		{
			name:   "letter key",
			msg:    tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}},
			wantN:  0,
			wantOk: false,
		},
		{
			name:   "enter key",
			msg:    tea.KeyMsg{Type: tea.KeyEnter},
			wantN:  0,
			wantOk: false,
		},
		{
			name:   "empty runes",
			msg:    tea.KeyMsg{Type: tea.KeyRunes, Runes: nil},
			wantN:  0,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, ok := selectOrdinal(tt.msg)
			assert.Equal(t, tt.wantN, n)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

func TestIsSelectField(t *testing.T) {
	t.Parallel()
	t.Run("select field returns true", func(t *testing.T) {
		val := "a"
		sel := huh.NewSelect[string]().
			Options(
				huh.NewOption("Alpha", "a"),
				huh.NewOption("Beta", "b"),
				huh.NewOption("Gamma", "c"),
			).
			Value(&val)
		form := huh.NewForm(huh.NewGroup(sel))
		form.Init()

		assert.True(t, isSelectField(form))
	})

	t.Run("input field returns false", func(t *testing.T) {
		val := ""
		inp := huh.NewInput().Title("Name").Value(&val)
		form := huh.NewForm(huh.NewGroup(inp))
		form.Init()

		assert.False(t, isSelectField(form))
	})
}

func TestSelectOptionCount(t *testing.T) {
	t.Parallel()
	val := "a"
	sel := huh.NewSelect[string]().
		Options(
			huh.NewOption("Alpha", "a"),
			huh.NewOption("Beta", "b"),
			huh.NewOption("Gamma", "c"),
		).
		Value(&val)
	form := huh.NewForm(huh.NewGroup(sel))
	form.Init()

	field := form.GetFocusedField()
	assert.Equal(t, 3, selectOptionCount(field))
}

func TestSelectOptionCountForInput(t *testing.T) {
	t.Parallel()
	val := ""
	inp := huh.NewInput().Title("Name").Value(&val)
	form := huh.NewForm(huh.NewGroup(inp))
	form.Init()

	field := form.GetFocusedField()
	assert.Equal(t, -1, selectOptionCount(field))
}

func TestJumpSelectToOrdinal(t *testing.T) {
	t.Parallel()
	t.Run("jumps to correct option", func(t *testing.T) {
		val := "a"
		sel := huh.NewSelect[string]().
			Options(
				huh.NewOption("Alpha", "a"),
				huh.NewOption("Beta", "b"),
				huh.NewOption("Gamma", "c"),
			).
			Value(&val)

		form := huh.NewForm(huh.NewGroup(sel))
		form.Init()

		m := &Model{fs: formState{form: form}}
		m.jumpSelectToOrdinal(2) // should jump to "Beta"

		assert.Equal(t, "b", val)
	})

	t.Run("ordinal 1 selects first option", func(t *testing.T) {
		val := "c"
		sel := huh.NewSelect[string]().
			Options(
				huh.NewOption("Alpha", "a"),
				huh.NewOption("Beta", "b"),
				huh.NewOption("Gamma", "c"),
			).
			Value(&val)

		form := huh.NewForm(huh.NewGroup(sel))
		form.Init()

		m := &Model{fs: formState{form: form}}
		m.jumpSelectToOrdinal(1)

		assert.Equal(t, "a", val)
	})

	t.Run("ordinal exceeding option count is ignored", func(t *testing.T) {
		val := "a"
		sel := huh.NewSelect[string]().
			Options(
				huh.NewOption("Alpha", "a"),
				huh.NewOption("Beta", "b"),
			).
			Value(&val)

		form := huh.NewForm(huh.NewGroup(sel))
		form.Init()

		m := &Model{fs: formState{form: form}}
		m.jumpSelectToOrdinal(5) // exceeds 2 options

		assert.Equal(t, "a", val, "value should be unchanged when ordinal exceeds count")
	})

	t.Run("works with uint select", func(t *testing.T) {
		val := uint(10)
		sel := huh.NewSelect[uint]().
			Options(
				huh.NewOption("First", uint(10)),
				huh.NewOption("Second", uint(20)),
				huh.NewOption("Third", uint(30)),
			).
			Value(&val)

		form := huh.NewForm(huh.NewGroup(sel))
		form.Init()

		m := &Model{fs: formState{form: form}}
		m.jumpSelectToOrdinal(3)

		require.Equal(t, uint(30), val)
	})
}
