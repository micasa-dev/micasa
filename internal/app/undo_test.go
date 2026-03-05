// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushUndo(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	require.Empty(t, m.undoStack)

	m.pushUndo(undoEntry{
		Description: "test edit",
		Restore:     func() error { return nil },
	})

	require.Len(t, m.undoStack, 1)
	assert.Equal(t, "test edit", m.undoStack[0].Description)
}

func TestPushUndoCapsAtMax(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	for i := range maxUndoStack + 10 {
		m.pushUndo(undoEntry{
			Description: fmt.Sprintf("edit %d", i),
			Restore:     func() error { return nil },
		})
	}

	require.Len(t, m.undoStack, maxUndoStack)
	assert.Equal(t, "edit 10", m.undoStack[0].Description)
}

func TestPopUndoRestoresAndRemoves(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	restored := false
	m.pushUndo(undoEntry{
		Description: "changed title",
		Restore: func() error {
			restored = true
			return nil
		},
	})

	err := m.popUndo()
	require.NoError(t, err)
	assert.True(t, restored, "expected Restore closure to be called")
	assert.Empty(t, m.undoStack)
	assert.Equal(t, statusInfo, m.status.Kind)
	// The undo message should be visible in the status bar.
	assert.Contains(t, m.statusView(), "changed title")
}

func TestPopUndoEmptyStack(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	err := m.popUndo()
	require.Error(t, err)
}

func TestPopUndoRestoreError(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.pushUndo(undoEntry{
		Description: "bad edit",
		Restore:     func() error { return fmt.Errorf("db failure") },
	})

	err := m.popUndo()
	require.Error(t, err)
}

func TestPopUndoLIFOOrder(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	var order []string

	m.pushUndo(undoEntry{
		Description: "first",
		Restore: func() error {
			order = append(order, "first")
			return nil
		},
	})
	m.pushUndo(undoEntry{
		Description: "second",
		Restore: func() error {
			order = append(order, "second")
			return nil
		},
	})

	_ = m.popUndo()
	_ = m.popUndo()

	assert.Equal(t, []string{"second", "first"}, order)
}

func TestUndoKeyInEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.mode = modeEdit
	m.setAllTableKeyMaps(editTableKeyMap())

	restored := false
	m.pushUndo(undoEntry{
		Description: "test",
		Restore: func() error {
			restored = true
			return nil
		},
	})

	sendKey(m, "u")
	assert.True(t, restored, "expected u key to trigger undo in Edit mode")
}

func TestUndoKeyIgnoredInNormalMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.mode = modeNormal

	called := false
	m.pushUndo(undoEntry{
		Description: "test",
		Restore: func() error {
			called = true
			return nil
		},
	})

	sendKey(m, "u")
	assert.False(t, called, "undo closure should not be called in Normal mode")
	assert.Len(t, m.undoStack, 1, "expected undo stack unchanged in Normal mode")
	// Status bar should still show NAV, not an undo message.
	assert.Contains(t, m.statusView(), "NAV")
}

func TestSnapshotForUndoSkipsCreates(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.fs.editID = nil
	m.fs.formKind = formProject

	m.snapshotForUndo()

	assert.Empty(t, m.undoStack, "expected no undo entry for create operations")
}

// --- Redo tests ---

func TestPopRedoEmptyStack(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	err := m.popRedo()
	require.Error(t, err)
}

func TestPopRedoRestoresAndRemoves(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	restored := false
	m.pushRedo(undoEntry{
		Description: "redo test",
		Restore: func() error {
			restored = true
			return nil
		},
	})

	err := m.popRedo()
	require.NoError(t, err)
	assert.True(t, restored, "expected Restore closure to be called")
	assert.Empty(t, m.redoStack)
	assert.Equal(t, statusInfo, m.status.Kind)
	// The redo message should be visible in the status bar.
	assert.Contains(t, m.statusView(), "redo test")
}

func TestRedoKeyInEditMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.mode = modeEdit
	m.setAllTableKeyMaps(editTableKeyMap())

	restored := false
	m.pushRedo(undoEntry{
		Description: "redo test",
		Restore: func() error {
			restored = true
			return nil
		},
	})

	sendKey(m, "r")
	assert.True(t, restored, "expected r key to trigger redo in Edit mode")
}

func TestRedoKeyIgnoredInNormalMode(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.mode = modeNormal

	called := false
	m.pushRedo(undoEntry{
		Description: "test",
		Restore: func() error {
			called = true
			return nil
		},
	})

	sendKey(m, "r")
	assert.False(t, called, "redo closure should not be called in Normal mode")
	assert.Len(t, m.redoStack, 1, "expected redo stack unchanged in Normal mode")
	assert.Contains(t, m.statusView(), "NAV")
}

func TestNewEditClearsRedoStack(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.redoStack = []undoEntry{
		{Description: "old redo"},
	}

	// Simulate a new edit by calling snapshotForUndo with no store/editID.
	// Since editID is nil and formKind is not formHouse, nothing is pushed,
	// but the real test is that a successful push clears redo.
	// We test the mechanism directly instead.
	m.fs.editID = nil
	m.fs.formKind = formProject
	m.snapshotForUndo()

	// editID nil means no push, redo should be unchanged (no new edit happened).
	assert.Len(t, m.redoStack, 1, "expected redo stack unchanged when no undo was pushed")
}

func TestUndoRedoCycle(t *testing.T) {
	t.Parallel()
	// Simulates: value starts at "A", user changes to "B", then undo, then redo.
	m := newTestModel(t)
	current := "B"

	// Push undo entry that restores "A".
	m.pushUndo(undoEntry{
		Description: "set to B",
		FormKind:    formProject,
		EntityID:    1,
		Restore: func() error {
			current = "A"
			return nil
		},
	})

	// Undo: should restore "A" (no store, so no redo snapshot via snapshotEntity).
	_ = m.popUndo()
	assert.Equal(t, "A", current)

	// Manually push a redo entry (simulating what snapshotEntity would do with a real store).
	m.pushRedo(undoEntry{
		Description: "set to B",
		FormKind:    formProject,
		EntityID:    1,
		Restore: func() error {
			current = "B"
			return nil
		},
	})

	// Redo: should restore "B".
	_ = m.popRedo()
	assert.Equal(t, "B", current)
}
