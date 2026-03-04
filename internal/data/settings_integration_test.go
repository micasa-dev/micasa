// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelPersistenceAcrossReopen simulates switching models and reopening
// the store to verify the last-used model persists.
func TestModelPersistenceAcrossReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))

	// Session 1: set model.
	store1, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, store1.PutLastModel("qwen3:8b"))
	require.NoError(t, store1.Close())

	// Session 2: read persisted model.
	store2, err := Open(path)
	require.NoError(t, err)
	model, err := store2.GetLastModel()
	require.NoError(t, err)
	assert.Equal(t, "qwen3:8b", model)
	require.NoError(t, store2.Close())
}

// TestChatHistoryPersistenceAcrossReopen simulates adding prompts and
// reopening the store to verify history persists.
func TestChatHistoryPersistenceAcrossReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.db")
	require.NoError(t, os.WriteFile(path, templateBytes, 0o600))

	// Session 1: add history.
	store1, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, store1.AppendChatInput("how many projects?"))
	require.NoError(t, store1.AppendChatInput("oldest appliance?"))
	require.NoError(t, store1.Close())

	// Session 2: load history.
	store2, err := Open(path)
	require.NoError(t, err)
	history, err := store2.LoadChatHistory()
	require.NoError(t, err)
	assert.Equal(t, []string{"how many projects?", "oldest appliance?"}, history)
	require.NoError(t, store2.Close())
}

// TestChatHistoryTrimming verifies that old entries beyond the cap are removed.
func TestChatHistoryTrimming(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Add entries beyond the cap.
	for i := range 250 {
		require.NoError(t, store.AppendChatInput(string(rune('a'+i%26))))
	}

	history, err := store.LoadChatHistory()
	require.NoError(t, err)
	assert.LessOrEqual(t, len(history), 200, "history should be capped at 200 entries")
}
