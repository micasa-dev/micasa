// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLWWLocalWinsLaterTimestamp(t *testing.T) {
	t.Parallel()
	local := time.Now()
	remote := local.Add(-time.Minute) // remote is older
	assert.True(t, lwwLocalWins(local, "dev-a", remote, "dev-b"))
}

func TestLWWRemoteWinsLaterTimestamp(t *testing.T) {
	t.Parallel()
	local := time.Now()
	remote := local.Add(time.Minute) // remote is newer
	assert.False(t, lwwLocalWins(local, "dev-a", remote, "dev-b"))
}

func TestLWWTiebreakByDeviceID(t *testing.T) {
	t.Parallel()
	ts := time.Now()

	// Same timestamp, higher device_id wins.
	assert.True(t, lwwLocalWins(ts, "dev-z", ts, "dev-a"))
	assert.False(t, lwwLocalWins(ts, "dev-a", ts, "dev-z"))
}

func TestLWWTiebreakSameDevice(t *testing.T) {
	t.Parallel()
	ts := time.Now()
	// Same timestamp, same device -- local wins (>=).
	assert.True(t, lwwLocalWins(ts, "dev-a", ts, "dev-a"))
}
