// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import "testing"

// newTestModelWithDemoData creates a Model backed by a real SQLite store,
// seeded with randomized demo data from the given HomeFaker. This provides
// richer test scenarios than newTestModelWithStore (which has only defaults).
func newTestModelWithDemoData(t *testing.T, seed uint64) *Model {
	t.Helper()
	return newTestModelWith(t, testModelOpts{withDemo: true, seed: seed})
}
