// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import "testing"

const testProjectTitle = "Test Project"

// newTestModelWithStore creates a Model backed by a real SQLite store with
// seeded defaults (project types, maintenance categories). The model is sized
// to 120x40 and starts in normal mode (dashboard and house form dismissed).
func newTestModelWithStore(t *testing.T) *Model {
	t.Helper()
	return newTestModelWith(t, testModelOpts{})
}
