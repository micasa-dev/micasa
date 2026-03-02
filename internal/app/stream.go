// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import tea "github.com/charmbracelet/bubbletea"

// waitForStream returns a Cmd that blocks until the next value arrives on ch.
// If the channel is open, wrap converts the value to a tea.Msg.
// If the channel is closed, closed is returned directly (nil stops the loop).
func waitForStream[T any](ch <-chan T, wrap func(T) tea.Msg, closed tea.Msg) tea.Cmd {
	return func() tea.Msg {
		v, ok := <-ch
		if !ok {
			return closed
		}
		return wrap(v)
	}
}
