// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testStreamMsg struct {
	Value string
}

type testStreamDoneMsg struct{}

func TestWaitForStreamOpenChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan string, 1)
	ch <- "hello"

	cmd := waitForStream(ch, func(s string) tea.Msg {
		return testStreamMsg{Value: s}
	}, testStreamDoneMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(testStreamMsg)
	require.True(t, ok)
	assert.Equal(t, "hello", result.Value)
}

func TestWaitForStreamClosedChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan string)
	close(ch)

	cmd := waitForStream(ch, func(s string) tea.Msg {
		return testStreamMsg{Value: s}
	}, testStreamDoneMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(testStreamDoneMsg)
	assert.True(t, ok, "closed channel should return the closed sentinel")
}
