// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireOllama skips the test when a live Ollama server is not reachable.
func requireOllama(t *testing.T) {
	t.Helper()

	const url = "http://localhost:11434/v1/models"
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		},
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Skipf("ollama not available: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("ollama not available: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test helper
	if resp.StatusCode != http.StatusOK {
		t.Skipf("ollama not available: status %d", resp.StatusCode)
	}
}

// testLLMClient creates a llamacpp client for unit tests that don't hit a
// real server. llamacpp is OpenAI-compatible and needs no API key.
func testLLMClient(t *testing.T, model string) *llm.Client {
	t.Helper()
	c, err := llm.NewClient("llamacpp", "http://localhost:11434/v1", model, "", 5*time.Second)
	require.NoError(t, err)
	return c
}

// testOllamaClient creates an Ollama client for live integration tests.
func testOllamaClient(t *testing.T, model string) *llm.Client {
	t.Helper()
	c, err := llm.NewClient("ollama", "http://localhost:11434", model, "", 10*time.Second)
	require.NoError(t, err)
	return c
}

// --- hideChat ---

func TestHideChatSetsVisibleFalse(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	require.NotNil(t, m.chat)
	require.True(t, m.chat.Visible)

	m.hideChat()
	assert.False(t, m.chat.Visible)
	assert.NotNil(t, m.chat, "session should be preserved")
}

func TestHideChatBlursInput(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	require.True(t, m.chat.Input.Focused())

	m.hideChat()
	assert.False(t, m.chat.Input.Focused())
}

func TestHideChatCancelsStreaming(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	cancelled := false
	m.chat.Streaming = true
	m.chat.CancelFn = func() { cancelled = true }

	m.hideChat()
	assert.True(t, cancelled, "cancel function should have been called")
	assert.False(t, m.chat.Streaming)
}

func TestHideChatCancelsPull(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.pull.active = true
	m.pull.fromChat = true
	m.pull.cancel = func() {}

	m.hideChat()
	assert.False(t, m.pull.active)
}

// --- handleChatKey: esc hides chat ---

func TestHandleChatKeyEscHidesChat(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	require.True(t, m.chat.Visible)

	sendKey(m, "esc")
	assert.False(t, m.chat.Visible, "esc should hide the chat overlay")
}

// --- handleChatKey: enter when streaming is no-op ---

func TestHandleChatKeyEnterWhileStreamingNoop(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	m.chat.Input.SetValue("hello")

	sendKey(m, "enter")
	assert.Equal(t, "hello", m.chat.Input.Value(), "input should not be consumed while streaming")
}

// --- handleChatKey: ctrl+s toggles SQL ---

func TestHandleChatKeyCtrlSTogglesSQL(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	require.False(t, m.chat.ShowSQL)

	sendKey(m, "ctrl+s")
	assert.True(t, m.chat.ShowSQL)

	sendKey(m, "ctrl+s")
	assert.False(t, m.chat.ShowSQL)
}

// --- handleChatKey: up/down for history ---

func TestHandleChatKeyUpDownHistory(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.History = []string{"first", "second", "third"}
	m.chat.HistoryCur = -1

	// Up arrow navigates to most recent history.
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, "third", m.chat.Input.Value())

	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, "second", m.chat.Input.Value())

	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, "first", m.chat.Input.Value())

	// Down navigates forward.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, "second", m.chat.Input.Value())
}

// --- handleChatKey: completer navigation ---

func TestHandleChatKeyCompleterEscDismisses(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{
		Matches: []modelCompleterMatch{{Name: "test-model"}},
	}

	sendKey(m, "esc")
	assert.Nil(t, m.chat.Completer, "esc should dismiss the completer")
	assert.True(t, m.chat.Visible, "chat should remain visible")
}

func TestHandleChatKeyCompleterUpDown(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{
		Matches: []modelCompleterMatch{
			{Name: "model-a"},
			{Name: "model-b"},
			{Name: "model-c"},
		},
		Cursor: 0,
	}

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, m.chat.Completer.Cursor)

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, m.chat.Completer.Cursor)

	// Should clamp at the end.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, m.chat.Completer.Cursor)

	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, m.chat.Completer.Cursor)
}

// --- buildChatOverlay ---

func TestBuildChatOverlayContainsTitle(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	overlay := m.buildChatOverlay()
	assert.NotEmpty(t, overlay)
	assert.Contains(t, overlay, "Ask", "overlay should contain the Ask title when no LLM client")
}

func TestBuildChatOverlayWithClientShowsModelName(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "test-model")
	m.openChat()

	overlay := m.buildChatOverlay()
	assert.Contains(t, overlay, "test-model")
}

func TestBuildChatOverlayShowsCompleterHints(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{
		Matches: []modelCompleterMatch{{Name: "m"}},
	}

	overlay := m.buildChatOverlay()
	assert.Contains(t, overlay, "navigate")
	assert.Contains(t, overlay, "select")
	assert.Contains(t, overlay, "dismiss")
}

func TestBuildChatOverlayShowsNormalHints(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	overlay := m.buildChatOverlay()
	assert.Contains(t, overlay, "send")
	assert.Contains(t, overlay, "sql")
	assert.Contains(t, overlay, "history")
	assert.Contains(t, overlay, "hide")
}

// --- renderModelCompleter ---

func TestRenderModelCompleterLoading(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{Loading: true}

	result := m.renderModelCompleter(80)
	assert.Contains(t, result, "loading models")
}

func TestRenderModelCompleterNoMatches(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{}
	m.chat.Input.SetValue("/model xyz")

	result := m.renderModelCompleter(80)
	assert.Contains(t, result, "no matching models")
}

func TestRenderModelCompleterNoModels(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{}
	m.chat.Input.SetValue("/model ")

	result := m.renderModelCompleter(80)
	assert.Contains(t, result, "no models available")
}

func TestRenderModelCompleterShowsMatches(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{
		Matches: []modelCompleterMatch{
			{Name: "llama3.2", Local: true},
			{Name: "gemma3:12b", Local: false},
		},
		Cursor: 0,
	}

	result := m.renderModelCompleter(80)
	assert.Contains(t, result, "llama3.2")
	assert.Contains(t, result, "gemma3:12b")
}

func TestRenderModelCompleterFixedHeight(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{
		Matches: []modelCompleterMatch{
			{Name: "model-a"},
		},
		Cursor: 0,
	}

	result := m.renderModelCompleter(80)
	lines := strings.Split(result, "\n")
	assert.Len(t, lines, completerMaxLines,
		"completer should always have %d lines", completerMaxLines)
}

// --- chatOverlayWidth ---

func TestChatOverlayWidthClampsMax(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.width = 200
	w := m.chatOverlayWidth()
	assert.LessOrEqual(t, w, 90)
}

func TestChatOverlayWidthClampsMin(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.width = 30
	w := m.chatOverlayWidth()
	assert.GreaterOrEqual(t, w, 40)
}

func TestChatOverlayWidthNormal(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.width = 80
	w := m.chatOverlayWidth()
	assert.Equal(t, 72, w, "80 - 8 = 72")
}

// --- chatViewportHeight ---

func TestChatViewportHeightClampsMin(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.height = 10
	h := m.chatViewportHeight()
	assert.GreaterOrEqual(t, h, 4)
}

func TestChatViewportHeightWithCompleter(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.height = 60
	m.openChat()

	hWithout := m.chatViewportHeight()

	m.chat.Completer = &modelCompleter{}
	hWith := m.chatViewportHeight()

	assert.Less(t, hWith, hWithout,
		"viewport should be shorter when completer is active")
}

// --- historyBack / historyForward ---

func TestHistoryBackEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.History = nil
	m.chat.HistoryCur = -1

	assert.NotPanics(t, func() { m.historyBack() })
	assert.Equal(t, -1, m.chat.HistoryCur)
}

func TestHistoryBackStashesCurrentInput(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.History = []string{"old"}
	m.chat.HistoryCur = -1
	m.chat.Input.SetValue("current typing")

	m.historyBack()
	assert.Equal(t, "current typing", m.chat.HistoryBuf)
	assert.Equal(t, "old", m.chat.Input.Value())
	assert.Equal(t, 0, m.chat.HistoryCur)
}

func TestHistoryBackClampsAtOldest(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.History = []string{"only"}
	m.chat.HistoryCur = 0

	m.historyBack()
	assert.Equal(t, 0, m.chat.HistoryCur, "should stay at oldest")
}

func TestHistoryForwardNotBrowsing(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.HistoryCur = -1

	assert.NotPanics(t, func() { m.historyForward() })
	assert.Equal(t, -1, m.chat.HistoryCur)
}

func TestHistoryForwardRestoresLiveInput(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.History = []string{"old"}
	m.chat.HistoryCur = -1
	m.chat.Input.SetValue("live input")

	m.historyBack()
	assert.Equal(t, "old", m.chat.Input.Value())

	m.historyForward()
	assert.Equal(t, -1, m.chat.HistoryCur)
	assert.Equal(t, "live input", m.chat.Input.Value())
}

func TestHistoryRoundTrip(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.History = []string{"a", "b", "c"}
	m.chat.HistoryCur = -1
	m.chat.Input.SetValue("live")

	m.historyBack() // -> c
	assert.Equal(t, "c", m.chat.Input.Value())
	m.historyBack() // -> b
	assert.Equal(t, "b", m.chat.Input.Value())
	m.historyBack() // -> a
	assert.Equal(t, "a", m.chat.Input.Value())
	m.historyBack() // still a (oldest)
	assert.Equal(t, "a", m.chat.Input.Value())

	m.historyForward() // -> b
	assert.Equal(t, "b", m.chat.Input.Value())
	m.historyForward() // -> c
	assert.Equal(t, "c", m.chat.Input.Value())
	m.historyForward() // -> live
	assert.Equal(t, "live", m.chat.Input.Value())
	assert.Equal(t, -1, m.chat.HistoryCur)
}

// --- buildConversationHistory ---

func TestBuildConversationHistoryEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = nil

	history := m.buildConversationHistory()
	assert.Nil(t, history)
}

func TestBuildConversationHistoryFiltersRoles(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "question 1"},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "answer 1", SQL: "SELECT 1"},
		{Role: roleError, Content: "some error"},
		{Role: roleUser, Content: "question 2"},
		{Role: roleAssistant, Content: "answer 2"},
	}

	history := m.buildConversationHistory()
	require.Len(t, history, 4)
	assert.Equal(t, "user", history[0].Role)
	assert.Equal(t, "question 1", history[0].Content)
	assert.Equal(t, "assistant", history[1].Role)
	assert.Equal(t, "answer 1", history[1].Content)
	assert.Equal(t, "user", history[2].Role)
	assert.Equal(t, "question 2", history[2].Content)
	assert.Equal(t, "assistant", history[3].Role)
	assert.Equal(t, "answer 2", history[3].Content)
}

func TestBuildConversationHistoryExcludesStreamingAssistant(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "q"},
		{Role: roleAssistant, Content: ""}, // streaming, empty
	}

	history := m.buildConversationHistory()
	require.Len(t, history, 1)
	assert.Equal(t, "user", history[0].Role)
}

func TestBuildConversationHistoryExcludesEmptyAssistant(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "q"},
		{Role: roleAssistant, Content: ""},
	}

	history := m.buildConversationHistory()
	require.Len(t, history, 1)
	assert.Equal(t, "user", history[0].Role)
}

// --- buildFallbackMessages ---

func TestBuildFallbackMessagesStructure(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "test-model")
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "prior question"},
		{Role: roleAssistant, Content: "prior answer"},
	}

	msgs := m.buildFallbackMessages("new question")
	require.NotEmpty(t, msgs)

	assert.Equal(t, "system", msgs[0].Role)
	// Should include conversation history.
	assert.Equal(t, "user", msgs[1].Role)
	assert.Equal(t, "prior question", msgs[1].Content)
	assert.Equal(t, "assistant", msgs[2].Role)
	assert.Equal(t, "prior answer", msgs[2].Content)
	// Current question last.
	assert.Equal(t, "user", msgs[len(msgs)-1].Role)
	assert.Equal(t, "new question", msgs[len(msgs)-1].Content)
}

func TestBuildFallbackMessagesNoHistory(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "test-model")
	m.openChat()

	msgs := m.buildFallbackMessages("question")
	require.Len(t, msgs, 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, "user", msgs[1].Role)
	assert.Equal(t, "question", msgs[1].Content)
}

// --- buildTableInfo / buildTableInfoFrom ---

func TestBuildTableInfoFromRealStore(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	tables := buildTableInfoFrom(m.store)
	require.NotEmpty(t, tables, "should return at least one table from seeded store")

	var hasProjects bool
	for _, tbl := range tables {
		if tbl.Name == data.TableProjects {
			hasProjects = true
			require.NotEmpty(t, tbl.Columns)
		}
	}
	assert.True(t, hasProjects, "should include the projects table")
}

func TestBuildTableInfoDelegatesToBuildTableInfoFrom(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	tables := m.buildTableInfo()
	require.NotEmpty(t, tables)
}

// --- mergeModelLists ---

func TestMergeModelListsNilServer(t *testing.T) {
	t.Parallel()
	all := mergeModelLists(nil)
	assert.NotEmpty(t, all, "should include well-known models")
	for _, e := range all {
		assert.False(t, e.Local, "should all be remote when no server models")
	}
}

func TestMergeModelListsServerOnly(t *testing.T) {
	t.Parallel()
	all := mergeModelLists([]string{"custom-model"})
	require.NotEmpty(t, all)
	assert.Equal(t, "custom-model", all[0].Name)
	assert.True(t, all[0].Local)
}

func TestMergeModelListsDeduplicates(t *testing.T) {
	t.Parallel()
	all := mergeModelLists([]string{"llama3.2"})
	count := 0
	for _, e := range all {
		if e.Name == "llama3.2" {
			count++
		}
	}
	assert.Equal(t, 1, count, "llama3.2 should appear exactly once")
}

func TestMergeModelListsServerFirst(t *testing.T) {
	t.Parallel()
	all := mergeModelLists([]string{"my-local-model"})
	require.NotEmpty(t, all)
	assert.Equal(t, "my-local-model", all[0].Name, "server models should come first")
	assert.True(t, all[0].Local)
}

func TestMergeModelListsLocalFlag(t *testing.T) {
	t.Parallel()
	all := mergeModelLists([]string{"llama3.2"})
	var localCount, remoteCount int
	for _, e := range all {
		if e.Local {
			localCount++
		} else {
			remoteCount++
		}
	}
	assert.Equal(t, 1, localCount)
	assert.Positive(t, remoteCount, "should have some well-known remote models")
}

// --- completerQuery ---

func TestCompleterQueryMatchesPrefix(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model qwen")

	q, ok := m.completerQuery()
	assert.True(t, ok)
	assert.Equal(t, "qwen", q)
}

func TestCompleterQueryEmptyAfterPrefix(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model ")

	q, ok := m.completerQuery()
	assert.True(t, ok)
	assert.Empty(t, q)
}

func TestCompleterQueryNoMatch(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("hello")

	_, ok := m.completerQuery()
	assert.False(t, ok)
}

func TestCompleterQueryCaseInsensitive(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/Model test")

	q, ok := m.completerQuery()
	assert.True(t, ok)
	assert.Equal(t, "test", q)
}

func TestCompleterQueryShortInput(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/mod")

	_, ok := m.completerQuery()
	assert.False(t, ok)
}

// --- activateCompleter ---

func TestActivateCompleterNoClientSetsNoLoading(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	require.Nil(t, m.llmClient)

	cmd := m.activateCompleter()
	assert.Nil(t, cmd, "should not fetch when no LLM client")
	require.NotNil(t, m.chat.Completer)
	assert.False(t, m.chat.Completer.Loading)
}

func TestActivateCompleterAlreadyActive(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{Loading: false}

	cmd := m.activateCompleter()
	assert.Nil(t, cmd, "should not re-activate")
}

func TestActivateCompleterWithClient(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "test")
	m.openChat()

	cmd := m.activateCompleter()
	require.NotNil(t, m.chat.Completer)
	assert.True(t, m.chat.Completer.Loading)
	assert.NotNil(t, cmd, "should return a fetch command")
}

// --- refilterCompleter ---

func TestRefilterCompleterEmptyQuery(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model ")
	m.chat.Completer = &modelCompleter{
		All: []modelCompleterEntry{
			{Name: "alpha", Local: true},
			{Name: "beta", Local: false},
		},
	}

	m.refilterCompleter()
	require.Len(t, m.chat.Completer.Matches, 2)
	assert.Equal(t, "alpha", m.chat.Completer.Matches[0].Name)
	assert.Equal(t, "beta", m.chat.Completer.Matches[1].Name)
}

func TestRefilterCompleterWithQuery(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model alp")
	m.chat.Completer = &modelCompleter{
		All: []modelCompleterEntry{
			{Name: "alpha", Local: true},
			{Name: "beta", Local: false},
		},
	}

	m.refilterCompleter()
	require.NotEmpty(t, m.chat.Completer.Matches)
	// "alpha" should match "alp"; "beta" should not.
	found := false
	for _, match := range m.chat.Completer.Matches {
		if match.Name == "alpha" {
			found = true
		}
		assert.NotEqual(t, "beta", match.Name)
	}
	assert.True(t, found, "alpha should be in matches")
}

func TestRefilterCompleterMarksActive(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "alpha")
	m.openChat()
	m.chat.Input.SetValue("/model ")
	m.chat.Completer = &modelCompleter{
		All: []modelCompleterEntry{
			{Name: "alpha", Local: true},
			{Name: "beta", Local: false},
		},
	}

	m.refilterCompleter()
	require.Len(t, m.chat.Completer.Matches, 2)
	assert.True(t, m.chat.Completer.Matches[0].Active, "alpha should be marked active")
	assert.False(t, m.chat.Completer.Matches[1].Active, "beta should not be active")
}

func TestRefilterCompleterClampsCursor(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model xyz")
	m.chat.Completer = &modelCompleter{
		All:    []modelCompleterEntry{{Name: "alpha"}},
		Cursor: 5,
	}

	m.refilterCompleter()
	assert.GreaterOrEqual(t, m.chat.Completer.Cursor, 0)
}

// --- clampCursor ---

func TestClampCursorHighValue(t *testing.T) {
	t.Parallel()
	mc := &modelCompleter{
		Matches: []modelCompleterMatch{{Name: "a"}, {Name: "b"}},
		Cursor:  10,
	}
	mc.clampCursor()
	assert.Equal(t, 1, mc.Cursor, "should clamp to last index")
}

func TestClampCursorNegative(t *testing.T) {
	t.Parallel()
	mc := &modelCompleter{
		Matches: []modelCompleterMatch{{Name: "a"}},
		Cursor:  -5,
	}
	mc.clampCursor()
	assert.Equal(t, 0, mc.Cursor)
}

func TestClampCursorEmptyMatches(t *testing.T) {
	t.Parallel()
	mc := &modelCompleter{
		Matches: nil,
		Cursor:  3,
	}
	mc.clampCursor()
	assert.Equal(t, 0, mc.Cursor)
}

func TestClampCursorAlreadyValid(t *testing.T) {
	t.Parallel()
	mc := &modelCompleter{
		Matches: []modelCompleterMatch{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		Cursor:  1,
	}
	mc.clampCursor()
	assert.Equal(t, 1, mc.Cursor)
}

// --- cleanPullStatus ---

func TestCleanPullStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		model  string
		want   string
	}{
		{"pulling manifest", "qwen3", "pulling qwen3"},
		{"Pulling manifest", "qwen3", "pulling qwen3"},
		{"pulling sha256:abcdef", "qwen3", "downloading qwen3"},
		{"verifying sha256:abc", "qwen3", "verifying qwen3"},
		{"writing manifest", "qwen3", "finalizing qwen3"},
		{"success", "qwen3", "ready"},
		{"something else", "qwen3", "something else"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, cleanPullStatus(tt.status, tt.model))
		})
	}
}

// --- openChat ---

func TestOpenChatCreatesNewSession(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	assert.Nil(t, m.chat)

	m.openChat()
	require.NotNil(t, m.chat)
	assert.True(t, m.chat.Visible)
	assert.True(t, m.chat.Input.Focused())
	assert.Equal(t, -1, m.chat.HistoryCur)
}

func TestOpenChatReshowsHiddenSession(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	// openChat with no llmClient adds a notice; track the initial count.
	initialCount := len(m.chat.Messages)
	m.chat.Messages = append(m.chat.Messages, chatMessage{
		Role: roleUser, Content: "preserved",
	})
	m.hideChat()
	require.False(t, m.chat.Visible)

	m.openChat()
	assert.True(t, m.chat.Visible)
	assert.True(t, m.chat.Input.Focused())
	require.Len(t, m.chat.Messages, initialCount+1)

	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, "preserved", last.Content)
}

func TestOpenChatNoLLMClientShowsNotice(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = nil
	m.openChat()

	require.NotEmpty(t, m.chat.Messages)
	assert.Equal(t, roleNotice, m.chat.Messages[0].Role)
	assert.Contains(t, m.chat.Messages[0].Content, "No LLM configured")
}

func TestOpenChatWithStoreLoadsHistory(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	require.NoError(t, m.store.AppendChatInput("stored-query"))

	m.openChat()
	require.NotNil(t, m.chat)
	assert.Contains(t, m.chat.History, "stored-query")
}

// --- cancelChatOperations ---

func TestCancelChatOperationsStreamingNotVisible(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Visible = false
	m.chat.Streaming = true
	cancelled := false
	m.chat.CancelFn = func() { cancelled = true }

	m.cancelChatOperations()
	assert.True(t, cancelled)
	assert.False(t, m.chat.Streaming)
}

// --- submitChat ---

func TestSubmitChatEmptyInput(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("")
	cmd := m.submitChat()
	assert.Nil(t, cmd)
}

func TestSubmitChatWhitespaceInput(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("   ")
	cmd := m.submitChat()
	assert.Nil(t, cmd)
}

func TestSubmitChatNoLLMClient(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("hello")
	cmd := m.submitChat()
	assert.Nil(t, cmd, "should return nil when no LLM client")
	assert.Empty(t, m.chat.Input.Value(), "input should be cleared")
}

func TestSubmitChatRecordsHistory(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("my question")
	m.submitChat()

	assert.Contains(t, m.chat.History, "my question")
	assert.Equal(t, -1, m.chat.HistoryCur)
	assert.Empty(t, m.chat.HistoryBuf)
}

func TestSubmitChatDeduplicatesHistory(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	m.chat.Input.SetValue("same question")
	m.submitChat()
	m.chat.Input.SetValue("same question")
	m.submitChat()

	count := 0
	for _, h := range m.chat.History {
		if h == "same question" {
			count++
		}
	}
	assert.Equal(t, 1, count, "consecutive duplicates should be deduplicated")
}

func TestSubmitChatRemovesInterruptedNotice(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "test")
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleNotice, Content: "Interrupted"},
	}
	m.chat.Input.SetValue("new question")
	m.submitChat()

	for _, msg := range m.chat.Messages {
		if msg.Role == roleNotice {
			assert.NotEqual(t, "Interrupted", msg.Content,
				"Interrupted notice should have been removed")
		}
	}
}

// --- handleSlashCommand ---

func TestHandleSlashCommandHelp(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/help")

	cmd := m.handleSlashCommand("/help")
	assert.Nil(t, cmd)
	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleNotice, last.Role)
	assert.Contains(t, last.Content, "/models")
	assert.Contains(t, last.Content, "/model")
	assert.Contains(t, last.Content, "/sql")
	assert.Contains(t, last.Content, "/help")
}

func TestHandleSlashCommandSqlToggles(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	require.False(t, m.chat.ShowSQL)

	cmd := m.handleSlashCommand("/sql")
	assert.Nil(t, cmd)
	assert.True(t, m.chat.ShowSQL)
}

func TestHandleSlashCommandUnknown(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	cmd := m.handleSlashCommand("/bogus")
	assert.Nil(t, cmd)
	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleError, last.Role)
	assert.Contains(t, last.Content, "unknown command")
	assert.Contains(t, last.Content, "/help")
}

func TestHandleSlashCommandModelNoArg(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "qwen3")
	m.openChat()

	cmd := m.handleSlashCommand("/model")
	assert.Nil(t, cmd)
	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleNotice, last.Role)
	assert.Contains(t, last.Content, "qwen3")
	assert.Contains(t, last.Content, "Usage")
}

// --- handleModelsListMsg ---

func TestHandleModelsListMsgRendersInChat(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "m1")
	m.openChat()

	m.handleModelsListMsg(modelsListMsg{Models: []string{"m1", "m2"}})
	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleNotice, last.Role)
	assert.Contains(t, last.Content, "m1")
	assert.Contains(t, last.Content, "m2")
}

func TestHandleModelsListMsgError(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	m.handleModelsListMsg(modelsListMsg{Err: fmt.Errorf("connection refused")})
	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleError, last.Role)
	assert.Contains(t, last.Content, "connection refused")
}

func TestHandleModelsListMsgEmptyModels(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()

	m.handleModelsListMsg(modelsListMsg{Models: []string{}})
	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Contains(t, last.Content, "no models available")
}

func TestHandleModelsListMsgFeedsCompleter(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model ")
	m.chat.Completer = &modelCompleter{Loading: true}

	m.handleModelsListMsg(modelsListMsg{Models: []string{"m1", "m2"}})
	assert.False(t, m.chat.Completer.Loading)
	require.NotEmpty(t, m.chat.Completer.All)
}

func TestHandleModelsListMsgCompleterError(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model ")
	m.chat.Completer = &modelCompleter{Loading: true}

	m.handleModelsListMsg(modelsListMsg{Err: fmt.Errorf("oops")})
	assert.False(t, m.chat.Completer.Loading)
	// Should fall back to well-known models only.
	require.NotEmpty(t, m.chat.Completer.All)
}

// --- removeLastNotice ---

func TestRemoveLastNotice(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "q"},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "a"},
	}

	m.removeLastNotice()
	require.Len(t, m.chat.Messages, 2)
	assert.Equal(t, roleUser, m.chat.Messages[0].Role)
	assert.Equal(t, roleAssistant, m.chat.Messages[1].Role)
}

// --- syncCompleter ---

func TestSyncCompleterActivatesOnModelPrefix(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model ")
	require.Nil(t, m.chat.Completer)

	m.syncCompleter(nil)
	require.NotNil(t, m.chat.Completer, "should activate completer for /model prefix")
}

func TestSyncCompleterDeactivatesWhenPrefixRemoved(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Completer = &modelCompleter{}
	m.chat.Input.SetValue("hello")

	m.syncCompleter(nil)
	assert.Nil(t, m.chat.Completer, "should deactivate when input doesn't match /model prefix")
}

func TestSyncCompleterRefiltersExisting(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Input.SetValue("/model q")
	m.chat.Completer = &modelCompleter{
		All: []modelCompleterEntry{
			{Name: "qwen3:8b"},
			{Name: "llama3.2"},
		},
	}

	m.syncCompleter(nil)
	// Should have refiltered: qwen3 matches "q", llama probably doesn't.
	found := false
	for _, match := range m.chat.Completer.Matches {
		if match.Name == "qwen3:8b" {
			found = true
		}
	}
	assert.True(t, found, "qwen3:8b should match 'q'")
}

// --- handleSQLStreamStarted ---

func TestHandleSQLStreamStartedError(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "q"},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: ""},
	}

	cmd := m.handleSQLStreamStarted(sqlStreamStartedMsg{
		Err:      fmt.Errorf("connection refused"),
		CancelFn: func() {},
	})
	assert.Nil(t, cmd)
	assert.False(t, m.chat.Streaming)
	assert.False(t, m.chat.StreamingSQL)

	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleError, last.Role)
	assert.Contains(t, last.Content, "connection refused")
}

func TestHandleSQLStreamStartedSuccess(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "q"},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: ""},
	}

	ch := make(chan llm.StreamChunk, 1)
	cancel := func() {}

	cmd := m.handleSQLStreamStarted(sqlStreamStartedMsg{
		Question: "q",
		Channel:  ch,
		CancelFn: cancel,
	})
	assert.NotNil(t, cmd, "should return a cmd to read SQL chunks")
	assert.Equal(t, "q", m.chat.CurrentQuery)
}

// --- handleSQLChunk ---

func TestHandleSQLChunkDroppedAfterCancel(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = false // Already cancelled.

	cmd := m.handleSQLChunk(sqlChunkMsg{Content: "SELECT 1", Done: false})
	assert.Nil(t, cmd)
}

func TestHandleSQLChunkError(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "q"},
		{Role: roleNotice, Content: "generating query"},
		{Role: roleAssistant, Content: "", SQL: ""},
	}

	cmd := m.handleSQLChunk(sqlChunkMsg{Err: fmt.Errorf("stream error")})
	assert.Nil(t, cmd)
	assert.False(t, m.chat.Streaming)

	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleError, last.Role)
	assert.Contains(t, last.Content, "stream error")
}

func TestHandleSQLChunkAccumulatesSQL(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	ch := make(chan llm.StreamChunk, 5)
	m.chat.SQLStreamCh = ch
	m.chat.Messages = []chatMessage{
		{Role: roleAssistant, Content: "", SQL: ""},
	}

	m.handleSQLChunk(sqlChunkMsg{Content: "SELECT ", Done: false})
	assert.Equal(t, "SELECT ", m.chat.Messages[0].SQL)

	m.handleSQLChunk(sqlChunkMsg{Content: "1", Done: false})
	assert.Equal(t, "SELECT 1", m.chat.Messages[0].SQL)
}

// --- handleChatChunk ---

func TestHandleChatChunkError(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	m.chat.Messages = []chatMessage{
		{Role: roleAssistant, Content: ""},
	}

	cmd := m.handleChatChunk(chatChunkMsg{Err: fmt.Errorf("oops")})
	assert.Nil(t, cmd)
	assert.False(t, m.chat.Streaming)
	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleError, last.Role)
}

func TestHandleChatChunkAccumulates(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	ch := make(chan llm.StreamChunk, 5)
	m.chat.StreamCh = ch
	m.chat.Messages = []chatMessage{
		{Role: roleAssistant, Content: ""},
	}

	m.handleChatChunk(chatChunkMsg{Content: "Hello "})
	assert.Equal(t, "Hello ", m.chat.Messages[0].Content)

	m.handleChatChunk(chatChunkMsg{Content: "world"})
	assert.Equal(t, "Hello world", m.chat.Messages[0].Content)
}

func TestHandleChatChunkDone(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Streaming = true
	m.chat.Messages = []chatMessage{
		{Role: roleAssistant, Content: "done"},
	}

	cmd := m.handleChatChunk(chatChunkMsg{Done: true})
	assert.Nil(t, cmd)
	assert.False(t, m.chat.Streaming)
}

// --- renderChatMessages ---

func TestRenderChatMessagesShowsAllRoles(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "my question"},
		{Role: roleAssistant, Content: "my answer"},
		{Role: roleError, Content: "bad thing"},
		{Role: roleNotice, Content: "info notice"},
	}

	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "my question")
	assert.Contains(t, rendered, "my answer")
	assert.Contains(t, rendered, "bad thing")
	assert.Contains(t, rendered, "info notice")
}

func TestRenderChatMessagesShowsSQLWhenToggled(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.ShowSQL = true
	m.chat.Messages = []chatMessage{
		{Role: roleAssistant, Content: "answer", SQL: "SELECT * FROM projects"},
	}

	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "SELECT")
}

func TestRenderChatMessagesHidesSQLWhenOff(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.ShowSQL = false
	m.chat.Messages = []chatMessage{
		{Role: roleAssistant, Content: "answer", SQL: "SELECT * FROM projects"},
	}

	rendered := m.renderChatMessages()
	assert.NotContains(t, rendered, "SELECT")
}

func TestRenderChatMessagesSkipsGeneratingQueryNotice(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleNotice, Content: "generating query"},
	}

	rendered := m.renderChatMessages()
	// The notice "generating query" is handled inline in the assistant message,
	// so the standalone notice is skipped.
	assert.NotContains(t, rendered, "generating query")
}

func TestRenderChatMessagesInterrupted(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleNotice, Content: "Interrupted"},
	}

	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "Interrupted")
}

func TestRenderChatMessagesPullCancelled(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleNotice, Content: "Pull cancelled"},
	}

	rendered := m.renderChatMessages()
	assert.Contains(t, rendered, "Pull cancelled")
}

// --- cmdSwitchModel: pull already in progress ---

func TestCmdSwitchModelPullAlreadyInProgress(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.llmClient = testLLMClient(t, "test")
	m.openChat()
	m.pull.active = true

	cmd := m.cmdSwitchModel("new-model")
	assert.Nil(t, cmd)
	require.NotEmpty(t, m.chat.Messages)
	last := m.chat.Messages[len(m.chat.Messages)-1]
	assert.Equal(t, roleError, last.Role)
	assert.Contains(t, last.Content, "already in progress")
}

// ===================================================================
// Ollama-dependent tests: skipped when Ollama is not available.
// ===================================================================

func TestCmdListModelsLive(t *testing.T) {
	t.Parallel()
	requireOllama(t)
	m := newTestModel()
	m.llmClient = testOllamaClient(t, "")
	m.openChat()

	cmd := m.cmdListModels()
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(modelsListMsg)
	require.True(t, ok, "expected modelsListMsg, got %T", msg)
	assert.NoError(t, result.Err)
}

func TestCmdSwitchModelLive(t *testing.T) {
	t.Parallel()
	requireOllama(t)
	m := newTestModel()
	m.llmClient = testOllamaClient(t, "")
	m.openChat()

	cmd := m.cmdSwitchModel("this-model-definitely-does-not-exist-xyz-9999")
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(pullProgressMsg)
	require.True(t, ok, "expected pullProgressMsg, got %T", msg)
}

func TestSubmitChatLiveSlashModels(t *testing.T) {
	t.Parallel()
	requireOllama(t)
	m := newTestModel()
	m.llmClient = testOllamaClient(t, "test")
	m.openChat()
	m.chat.Input.SetValue("/models")

	cmd := m.submitChat()
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(modelsListMsg)
	require.True(t, ok, "expected modelsListMsg, got %T", msg)
	assert.NoError(t, result.Err)
}

// --- replaceAssistantWithError ---

func TestReplaceAssistantWithErrorRemovesIncomplete(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "question"},
		{Role: roleAssistant, Content: "partial..."},
	}

	m.replaceAssistantWithError("stream failed")
	require.Len(t, m.chat.Messages, 2)
	assert.Equal(t, roleUser, m.chat.Messages[0].Role)
	assert.Equal(t, roleError, m.chat.Messages[1].Role)
	assert.Equal(t, "stream failed", m.chat.Messages[1].Content)
}

func TestReplaceAssistantWithErrorNoAssistant(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = []chatMessage{
		{Role: roleUser, Content: "question"},
	}

	m.replaceAssistantWithError("something broke")
	require.Len(t, m.chat.Messages, 2)
	assert.Equal(t, roleUser, m.chat.Messages[0].Role)
	assert.Equal(t, roleError, m.chat.Messages[1].Role)
	assert.Equal(t, "something broke", m.chat.Messages[1].Content)
}

func TestReplaceAssistantWithErrorEmptyMessages(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.openChat()
	m.chat.Messages = nil

	m.replaceAssistantWithError("error")
	require.Len(t, m.chat.Messages, 1)
	assert.Equal(t, roleError, m.chat.Messages[0].Role)
}

// --- waitForSQLChunk / waitForChunk ---

func TestWaitForSQLChunkOpenChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Content: "SELECT ", Done: false}

	cmd := waitForSQLChunk(ch)
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(sqlChunkMsg)
	require.True(t, ok)
	assert.Equal(t, "SELECT ", result.Content)
	assert.False(t, result.Done)
}

func TestWaitForSQLChunkClosedChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan llm.StreamChunk)
	close(ch)

	cmd := waitForSQLChunk(ch)
	require.NotNil(t, cmd)

	msg := cmd()
	assert.Nil(t, msg, "closed channel should return nil sentinel")
}

func TestWaitForChunkOpenChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Content: "Hello", Done: false}

	cmd := waitForChunk(ch)
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(chatChunkMsg)
	require.True(t, ok)
	assert.Equal(t, "Hello", result.Content)
	assert.False(t, result.Done)
}

func TestWaitForChunkClosedChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan llm.StreamChunk)
	close(ch)

	cmd := waitForChunk(ch)
	require.NotNil(t, cmd)

	msg := cmd()
	assert.Nil(t, msg, "closed channel should return nil sentinel")
}

func TestActivateCompleterLive(t *testing.T) {
	t.Parallel()
	requireOllama(t)
	m := newTestModel()
	m.llmClient = testOllamaClient(t, "")
	m.openChat()

	cmd := m.activateCompleter()
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(modelsListMsg)
	require.True(t, ok, "expected modelsListMsg, got %T", msg)
	assert.NoError(t, result.Err)
}
