// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"strings"
	"testing"

	"github.com/cpcloud/micasa/internal/llm"
	"github.com/stretchr/testify/assert"
)

const testQuestion = "test question"

// TestSQLChunkCompletionDoesNotPanic verifies that when SQL streaming
// completes (Done chunk arrives), the model doesn't panic on message
// indexing. This is a regression test for a panic that occurred when the
// code tried to access messages[-3]. The chunk is delivered through
// Model.Update, the same path a real Bubble Tea message takes.
func TestSQLChunkCompletionDoesNotPanic(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	m.chat.CurrentQuery = testQuestion
	m.chat.StreamingSQL = true
	m.chat.Streaming = true
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: testQuestion},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: "SELECT * FROM projects"},
	}

	// Done chunk arrives through Update -- must not panic.
	assert.NotPanics(t, func() {
		m.Update(sqlChunkMsg{Done: true})
	})

	// The "generating query" notice should be gone and no error should appear.
	rendered := m.renderChatMessages()
	assert.NotContains(t, rendered, "generating query",
		"notice should be removed after SQL completion")
	assert.NotContains(t, rendered, "panic",
		"should not panic during SQL completion")
}

// TestSQLChunkDoneWithNoAssistantMessageShowsError verifies that a Done
// chunk arriving when no assistant message exists (edge case) shows an
// error instead of panicking. Delivered through Model.Update.
func TestSQLChunkDoneWithNoAssistantMessageShowsError(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	m.chat.CurrentQuery = testQuestion
	m.chat.StreamingSQL = true
	m.chat.Streaming = true
	m.chat.Messages = []chatMessage{} // no messages at all

	// Must not panic.
	assert.NotPanics(t, func() {
		m.Update(sqlChunkMsg{Done: true})
	})

	// Should show an error in the rendered output.
	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "LLM returned empty SQL",
		"should surface error when no assistant message exists")
}

// TestSQLStreamStartedSetsUpStreaming verifies that when the SQL stream
// starts, the model transitions into streaming state and subsequent chunks
// accumulate SQL visible in the rendered output (with ShowSQL toggled on
// via ctrl+s, as a real user would). All messages flow through Model.Update.
func TestSQLStreamStartedSetsUpStreaming(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	// User toggles SQL display on (ctrl+s).
	sendKey(m, "ctrl+s")

	question := "how much did I spend on projects?"
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: question},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: ""},
	}

	ch := make(chan llm.StreamChunk, 4)
	ch <- llm.StreamChunk{Content: "SELECT ", Done: false}
	ch <- llm.StreamChunk{Content: "1", Done: false}

	// Stream-started message arrives through Update.
	m.Update(sqlStreamStartedMsg{
		Question: question,
		Channel:  ch,
		CancelFn: func() {},
	})

	// Deliver the buffered chunks through Update.
	m.Update(sqlChunkMsg{Content: "SELECT ", Done: false})
	m.Update(sqlChunkMsg{Content: "1", Done: false})

	// The accumulated SQL should be visible in the rendered output.
	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "SELECT",
		"streamed SQL tokens should appear in rendered output when ShowSQL is on")
}

// TestCancellationDuringSQLGeneration verifies that ctrl+c during stage 1
// (SQL generation) removes the spinner and partial assistant message,
// showing only "Interrupted" in the rendered output.
func TestCancellationDuringSQLGeneration(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	_, cancel := context.WithCancel(context.Background())
	m.chat.CurrentQuery = testQuestion
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.CancelFn = cancel
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: testQuestion},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: ""},
	}

	// Before: spinner text visible.
	assert.Contains(t, m.renderChatMessages(), "generating query")

	// User presses ctrl+c.
	sendKey(m, "ctrl+c")

	// After: spinner gone, "Interrupted" shown, no stale assistant content.
	rendered := m.renderChatMessages()
	assert.NotContains(t, rendered, "generating query")
	assert.Contains(t, rendered, "Interrupted")
}

// TestCancellationDuringAnswerStreaming verifies that ctrl+c during stage 2
// (answer streaming) removes the partial response and shows "Interrupted".
func TestCancellationDuringAnswerStreaming(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	_, cancel := context.WithCancel(context.Background())
	m.chat.CurrentQuery = testQuestion
	m.chat.Streaming = true
	m.chat.StreamingSQL = false // stage 2
	m.chat.CancelFn = cancel
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: testQuestion},
		{Role: roleAssistant, Content: "Based on the data", SQL: "SELECT * FROM projects"},
	}

	// Before: partial response visible.
	assert.Contains(t, m.renderChatMessages(), "Based on the data")

	// User presses ctrl+c.
	sendKey(m, "ctrl+c")

	// After: partial gone, "Interrupted" shown.
	rendered := m.renderChatMessages()
	assert.NotContains(t, rendered, "Based on the data")
	assert.Contains(t, rendered, "Interrupted")
}

// TestCancellationBeforeStreamEstablished verifies that ctrl+c works
// even in the window between submitChat and handleSQLStreamStarted where
// the LLM stream hasn't been set up yet (CancelFn is nil).
func TestCancellationBeforeStreamEstablished(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.CancelFn = nil // stream not yet established
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: testQuestion},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: ""},
	}

	// Before: spinner visible.
	assert.Contains(t, m.renderChatMessages(), "generating query")

	// User presses ctrl+c -- must not panic despite nil CancelFn.
	assert.NotPanics(t, func() { sendKey(m, "ctrl+c") })

	// After: clean output with Interrupted.
	rendered := m.renderChatMessages()
	assert.NotContains(t, rendered, "generating query")
	assert.Contains(t, rendered, "Interrupted")
}

// TestSpinnerOnlyShowsForLastMessage verifies that the spinner is only
// rendered for the last assistant message, not all assistant messages.
// This is a regression test for the bug where all assistant messages with
// empty content/SQL would show spinners during streaming.
func TestSpinnerOnlyShowsForLastMessage(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	// Setup: one completed assistant message + one streaming
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "question 1"},
		{Role: roleAssistant, Content: "", SQL: ""}, // completed but empty
		{Role: roleUser, Content: "question 2"},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: ""}, // currently streaming
	}

	rendered := m.renderChatMessages()

	// "generating query" should appear exactly once (for the last message only).
	// Count occurrences: the notice is skipped in rendering, so only the
	// inline spinner text contributes. With isLastMessage check, only the
	// last assistant message renders the spinner.
	count := strings.Count(rendered, "generating query")
	assert.Equal(t, 1, count,
		"generating query should appear once (last message only), got %d", count)
}

// TestNoSpinnerAfterCancellation verifies that after ctrl+c the rendered
// chat output contains no spinner text.
func TestNoSpinnerAfterCancellation(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	_, cancel := context.WithCancel(context.Background())
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.CancelFn = cancel
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: testQuestion},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: "SELECT"},
	}

	// User presses ctrl+c.
	sendKey(m, "ctrl+c")

	rendered := m.renderChatMessages()
	assert.NotContains(t, rendered, "generating query",
		"should not show spinner text after cancellation")
	assert.NotContains(t, rendered, "thinking",
		"should not show thinking text after cancellation")
	assert.Contains(t, rendered, "Interrupted",
		"should show Interrupted notice")
}

// TestLateSQLChunkAfterCancellationIsDropped verifies that a Done chunk
// arriving after ctrl+c doesn't overwrite the "Interrupted" notice with
// "LLM returned empty SQL". This reproduces the race where the channel
// has a buffered Done chunk that gets delivered after cancelChatOperations.
//
// The test drives everything through Model.Update to exercise the real
// dispatch path a user would trigger.
func TestLateSQLChunkAfterCancellationIsDropped(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	_, cancel := context.WithCancel(context.Background())
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.CancelFn = cancel
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: testQuestion},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: "SELECT"},
	}

	// User presses ctrl+c -- goes through Update -> cancelChatOperations.
	sendKey(m, "ctrl+c")

	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "Interrupted")

	// Late Done chunk arrives through Update (buffered in the channel
	// before cancellation drained it).
	m.Update(sqlChunkMsg{Done: true})

	// The rendered output must still show "Interrupted", not the error.
	rendered = m.renderChatMessages()
	assert.Contains(t, rendered, "Interrupted",
		"Interrupted notice must survive a late Done chunk")
	assert.NotContains(t, rendered, "LLM returned empty SQL",
		"late Done chunk must not produce an error after cancellation")
}

// TestLateChatChunkAfterCancellationIsDropped is the stage-2 equivalent:
// a streamed answer chunk arriving after cancellation should be ignored.
// Driven through Model.Update like the real dispatch path.
func TestLateChatChunkAfterCancellationIsDropped(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	_, cancel := context.WithCancel(context.Background())
	m.chat.Streaming = true
	m.chat.CancelFn = cancel
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: testQuestion},
		{Role: roleAssistant, Content: "partial", SQL: "SELECT 1"},
	}

	// User presses ctrl+c.
	sendKey(m, "ctrl+c")

	// Late chunk from the answer stream arrives through Update.
	m.Update(chatChunkMsg{Content: " more", Done: false})

	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "Interrupted")
	assert.NotContains(t, rendered, "partial",
		"partial content should have been removed by cancellation")
}

// TestChatMagModeToggle verifies that ctrl+o toggles the global mag mode
// even when the chat overlay is active.
func TestChatMagModeToggle(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	// Simulate a completed assistant response with a dollar amount.
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "how much?"},
		{Role: roleAssistant, Content: "You spent $1,000.00 total."},
	}

	// Initially off: dollar amount visible in rendered chat.
	assert.False(t, m.magMode)
	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "$1,000.00",
		"dollar amount should appear with mag mode off")

	// Toggle on from within chat.
	sendKey(m, "ctrl+o")
	assert.True(t, m.magMode)
	rendered = m.renderChatMessages()
	assert.NotContains(t, rendered, "$1,000.00",
		"dollar amount should not appear with mag mode on")
	assert.Contains(t, rendered, magArrow,
		"magnitude notation should appear with mag mode on")

	// Toggle off.
	sendKey(m, "ctrl+o")
	assert.False(t, m.magMode)
	rendered = m.renderChatMessages()
	assert.Contains(t, rendered, "$1,000.00",
		"dollar amount should reappear after toggling off")
}

// TestChatMagModeTogglesRenderedOutput verifies that toggling mag mode
// updates dollar amounts in already-displayed LLM responses.
func TestChatMagModeTogglesRenderedOutput(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.width = 120
	m.height = 40
	m.openChat()

	// Simulate a completed assistant response with dollar amounts.
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "how much did I spend?"},
		{Role: roleAssistant, Content: "You spent $5,234.23 on kitchen renovations."},
	}
	m.refreshChatViewport()

	// Mag mode off: original dollar amount should appear in the viewport.
	vpContent := m.chat.Viewport.View()
	assert.Contains(t, vpContent, "$5,234.23",
		"dollar amount should appear verbatim with mag mode off")
	assert.NotContains(t, vpContent, magArrow,
		"mag arrow should not appear with mag mode off")

	// Toggle mag mode on from within chat.
	sendKey(m, "ctrl+o")
	assert.True(t, m.magMode)

	vpContent = m.chat.Viewport.View()
	assert.NotContains(t, vpContent, "$5,234.23",
		"original dollar amount should be replaced with mag mode on")
	assert.Contains(t, vpContent, magArrow,
		"mag arrow should appear with mag mode on")

	// Toggle back off.
	sendKey(m, "ctrl+o")
	assert.False(t, m.magMode)

	vpContent = m.chat.Viewport.View()
	assert.Contains(t, vpContent, "$5,234.23",
		"dollar amount should reappear after toggling mag mode off")
}
