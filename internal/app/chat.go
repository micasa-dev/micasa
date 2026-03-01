// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/llm"
	ollamaPull "github.com/cpcloud/micasa/internal/ollama"
)

const (
	roleAssistant = "assistant"
	roleUser      = "user"
	roleError     = "error"
	roleNotice    = "notice"
)

// chatMessage is one turn in the conversation.
type chatMessage struct {
	Role    string // roleUser, roleAssistant, roleError, or roleNotice
	Content string
	SQL     string // For assistant messages: the SQL query used (if any)
}

// chatState holds the state of the LLM chat overlay.
type chatState struct {
	Messages     []chatMessage
	Input        textinput.Model
	Viewport     viewport.Model
	Spinner      spinner.Model
	Streaming    bool                   // true while an LLM response is being streamed
	StreamingSQL bool                   // true during SQL generation (stage 1)
	StreamCh     <-chan llm.StreamChunk // for stage 2 (answer streaming)
	SQLStreamCh  <-chan llm.StreamChunk // for stage 1 (SQL generation)
	CancelFn     context.CancelFunc
	CurrentQuery string          // the user's current question being processed
	Completer    *modelCompleter // non-nil when the model picker is showing
	ShowSQL      bool            // when true, show generated SQL as a notice
	History      []string        // past user inputs, newest last
	HistoryCur   int             // index into History for up/down browsing (-1 = live input)
	HistoryBuf   string          // stashed live input while browsing history
	Visible      bool            // false when the overlay is hidden but session persists

	markdownRenderer
}

// modelCompleter is the inline autocomplete list for /model.
type modelCompleter struct {
	All     []modelCompleterEntry // local + well-known models
	Matches []modelCompleterMatch
	Cursor  int
	Loading bool
}

type modelCompleterEntry struct {
	Name  string
	Local bool // true if already downloaded on the server
}

type modelCompleterMatch struct {
	Name      string
	Score     int
	Index     int // original position for tiebreaking
	Positions []int
	Active    bool // true if this is the currently selected model
	Local     bool // true if already available on the server
}

func (m modelCompleterMatch) fuzzyScore() int { return m.Score }
func (m modelCompleterMatch) fuzzyIndex() int { return m.Index }

const modelCommandPrefix = "/model "

// completerMaxLines is the fixed number of lines the completer occupies.
// The viewport shrinks by this amount when the completer is active so the
// overall overlay height stays constant.
const completerMaxLines = 8

// Popular models that appear in the completer even when the server doesn't
// have them downloaded yet. Users can select one and it will be pulled.
var wellKnownModels = []string{
	"deepseek-r1:32b",
	"gemma3:12b",
	"gemma3:27b",
	"llama3.1:70b",
	"llama3.2",
	"llama3.3",
	"mistral-small:24b",
	"phi-4:14b",
	"qwen3:8b",
	"qwen3:32b",
	"qwen3:72b",
}

// chatChunkMsg delivers a single streamed token to the Bubbletea update loop.
type chatChunkMsg struct {
	Content string
	Done    bool
	Err     error
}

// sqlChunkMsg delivers partial SQL during streaming generation (stage 1).
type sqlChunkMsg struct {
	Content string // partial SQL
	Done    bool   // true when SQL generation is complete
	Err     error  // non-nil if SQL generation failed
}

// sqlResultMsg delivers the result of stage 1 (NL → SQL) back to the
// update loop so stage 2 (summary) can proceed.
type sqlResultMsg struct {
	Question string // original user question
	SQL      string // generated SELECT statement
	Columns  []string
	Rows     [][]string
	Err      error // set if SQL generation, validation, or execution failed
}

// modelsListMsg delivers the result of an async ListModels call.
type modelsListMsg struct {
	Models []string
	Err    error
}

// pullProgressMsg delivers progress from the Ollama pull API.
type pullProgressMsg struct {
	Status    string
	Percent   float64 // 0.0 - 1.0; -1 when unknown
	Done      bool
	Err       error
	Model     string
	PullState *ollamaPullState
}

// ollamaPullState tracks the streaming pull HTTP response.
type ollamaPullState struct {
	Model   string
	Cancel  context.CancelFunc
	Scanner *ollamaPull.PullScanner
}

// openChat shows the chat overlay. If a session already exists it is
// un-hidden; otherwise a fresh session is created.
func (m *Model) openChat() {
	if m.chat != nil {
		// Session exists but was hidden -- just show it again.
		m.chat.Visible = true
		m.chat.Input.Focus()
		m.refreshChatViewport()
		return
	}

	ti := textinput.New()
	ti.Placeholder = "Ask about your home data... (/help for commands)"
	ti.CharLimit = 500
	ti.Width = m.chatInputWidth()
	ti.Focus()

	vp := viewport.New(m.chatViewportWidth(), m.chatViewportHeight())
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = m.styles.AccentText()

	// Load persisted prompt history from the database.
	var history []string
	if m.store != nil {
		if h, err := m.store.LoadChatHistory(); err == nil {
			history = h
		}
	}

	m.chat = &chatState{
		Input:      ti,
		Viewport:   vp,
		Spinner:    sp,
		Visible:    true,
		History:    history,
		HistoryCur: -1,
	}

	// If no LLM client, show a hint instead of failing silently.
	if m.llmClient == nil {
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleNotice,
			Content: fmt.Sprintf(
				"No LLM configured. Create %s with:\n\n[llm]\nbase_url = \"http://localhost:11434/v1\"\nmodel = \"qwen3\"",
				shortenHome(m.configPath),
			),
		})
		m.refreshChatViewport()
	}
}

// hideChat hides the chat overlay but preserves the session so the user
// can return to it with @. In-flight streams and pulls are cancelled since
// the user won't be watching them.
func (m *Model) hideChat() {
	if m.chat == nil {
		return
	}
	m.cancelChatOperations()
	m.chat.Input.Blur()
	m.chat.Visible = false
}

// cancelChatOperations cancels any in-flight LLM streams or model pulls.
// When the chat is visible, this also cleans up messages and shows an
// "Interrupted" notice.
func (m *Model) cancelChatOperations() {
	if m.chat == nil {
		return
	}
	if m.chat.Streaming {
		if m.chat.CancelFn != nil {
			m.chat.CancelFn()
		}
		m.chat.Streaming = false
		m.chat.StreamingSQL = false
		m.chat.SQLStreamCh = nil
		m.chat.StreamCh = nil
		m.chat.CancelFn = nil

		if m.chat.Visible {
			m.removeLastNotice()
			if msgs := m.chat.Messages; len(msgs) > 0 && msgs[len(msgs)-1].Role == roleAssistant {
				m.chat.Messages = msgs[:len(msgs)-1]
			}
			m.chat.Messages = append(m.chat.Messages, chatMessage{
				Role: roleNotice, Content: "Interrupted",
			})
			m.refreshChatViewport()
		}
	}
	if m.pull.active {
		wasChatPull := m.pull.fromChat
		m.cancelPull()
		if wasChatPull && m.chat.Visible {
			m.chat.Messages = append(m.chat.Messages, chatMessage{
				Role: roleNotice, Content: "Pull cancelled",
			})
			m.refreshChatViewport()
		}
	}
}

// submitChat processes the user's input. Slash commands are intercepted;
// everything else enters the two-stage pipeline (NL → SQL → results → English).
func (m *Model) submitChat() tea.Cmd {
	if m.chat == nil {
		return nil
	}
	query := strings.TrimSpace(m.chat.Input.Value())
	if query == "" {
		return nil
	}
	m.chat.Input.SetValue("")

	// Record in history for up/down browsing, deduplicating consecutive repeats.
	if len(m.chat.History) == 0 || m.chat.History[len(m.chat.History)-1] != query {
		m.chat.History = append(m.chat.History, query)
		// Best-effort: persist for cross-session history. Primary chat
		// flow succeeds regardless of persistence failure.
		if m.store != nil {
			_ = m.store.AppendChatInput(query)
		}
	}
	m.chat.HistoryCur = -1
	m.chat.HistoryBuf = ""

	// Slash commands.
	if strings.HasPrefix(query, "/") {
		return m.handleSlashCommand(query)
	}

	// Regular LLM query -- two-stage pipeline.
	if m.llmClient == nil {
		return nil
	}

	// Remove trailing "Interrupted" notice from a previous cancellation.
	if n := len(m.chat.Messages); n > 0 &&
		m.chat.Messages[n-1].Role == roleNotice &&
		m.chat.Messages[n-1].Content == "Interrupted" {
		m.chat.Messages = m.chat.Messages[:n-1]
	}

	m.chat.Messages = append(m.chat.Messages, chatMessage{
		Role: roleUser, Content: query,
	})
	m.chat.Streaming = true
	m.chat.StreamingSQL = true
	m.chat.Messages = append(m.chat.Messages,
		chatMessage{Role: roleNotice, Content: "generating query"},
		// Empty assistant message that we'll populate with SQL and later the answer.
		chatMessage{Role: roleAssistant, Content: "", SQL: ""},
	)
	m.refreshChatViewport()

	// Stage 1: Stream SQL generation.
	return tea.Batch(m.startSQLStream(query), m.chat.Spinner.Tick)
}

// startSQLStream initiates streaming SQL generation (stage 1).
func (m *Model) startSQLStream(query string) tea.Cmd {
	client := m.llmClient
	store := m.store
	extraContext := m.llmExtraContext
	// Capture conversation history on the main goroutine before the closure
	// runs in a background goroutine -- m.chat.Messages is mutated by the
	// Bubble Tea event loop and is not safe to read concurrently.
	history := m.buildConversationHistory()

	return func() tea.Msg {
		// Build schema info and column hints inside the goroutine to avoid
		// blocking the UI thread with DB queries.
		tables := buildTableInfoFrom(store)
		columnHints := ""
		if store != nil {
			columnHints = store.ColumnHints()
		}
		sqlPrompt := llm.BuildSQLPrompt(tables, time.Now(), columnHints, extraContext)

		// Build conversation history: system + all previous user/assistant exchanges + current query.
		messages := []llm.Message{
			{Role: "system", Content: sqlPrompt},
		}
		messages = append(messages, history...)
		messages = append(messages, llm.Message{Role: roleUser, Content: query})

		ctx, cancel := context.WithCancel(context.Background())

		streamCh, err := client.ChatStream(ctx, messages)
		if err != nil {
			return sqlStreamStartedMsg{
				Question: query,
				CancelFn: cancel,
				Err:      fmt.Errorf("SQL generation failed: %w", err),
			}
		}

		return sqlStreamStartedMsg{
			Question: query,
			Channel:  streamCh,
			CancelFn: cancel,
		}
	}
}

// sqlStreamStartedMsg delivers the SQL stream channel back to the update loop.
type sqlStreamStartedMsg struct {
	Question string
	Channel  <-chan llm.StreamChunk
	CancelFn context.CancelFunc
	Err      error
}

// handleSlashCommand dispatches chat slash commands.
func (m *Model) handleSlashCommand(input string) tea.Cmd {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/models":
		return m.cmdListModels()
	case "/model":
		if len(parts) < 2 {
			m.chat.Messages = append(m.chat.Messages, chatMessage{
				Role: roleNotice,
				Content: "Active model: " + m.llmModelLabel() +
					"\nUsage: /model <name>",
			})
			m.refreshChatViewport()
			return nil
		}
		return m.cmdSwitchModel(parts[1])
	case "/sql":
		m.toggleSQL()
		return nil
	case "/help":
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleNotice,
			Content: "/models          list available models\n" +
				"/model <name>    switch model (pulls if needed)\n" +
				"/sql             toggle SQL query display\n" +
				"/help            show this help",
		})
		m.refreshChatViewport()
		return nil
	default:
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleError, Content: fmt.Sprintf("unknown command: %s (try /help)", cmd),
		})
		m.refreshChatViewport()
		return nil
	}
}

// cmdListModels fetches models from the server and displays them.
func (m *Model) cmdListModels() tea.Cmd {
	if m.llmClient == nil {
		return nil
	}
	client := m.llmClient
	timeout := client.Timeout()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		models, err := client.ListModels(ctx)
		return modelsListMsg{Models: models, Err: err}
	}
}

// handleModelsListMsg populates the completer if active, or renders the
// model list into the chat (for /models command).
func (m *Model) handleModelsListMsg(msg modelsListMsg) {
	if m.chat == nil {
		return
	}

	// Feed the completer if it's waiting for results.
	if mc := m.chat.Completer; mc != nil && mc.Loading {
		mc.Loading = false
		if msg.Err == nil {
			mc.All = mergeModelLists(msg.Models)
		} else {
			// Server unreachable -- show well-known models only.
			mc.All = mergeModelLists(nil)
		}
		m.refilterCompleter()
		return
	}

	// Otherwise this was a /models command -- render into chat.
	if msg.Err != nil {
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleError, Content: msg.Err.Error(),
		})
		m.refreshChatViewport()
		return
	}

	current := m.llmModelLabel()
	var b strings.Builder
	for _, name := range msg.Models {
		if name == current {
			// Use accent-colored bullet to indicate active model.
			marker := m.styles.AccentText().Render("• ")
			b.WriteString(marker + name + "\n")
		} else {
			b.WriteString("  " + name + "\n")
		}
	}
	if len(msg.Models) == 0 {
		b.WriteString("  (no models available)")
	}
	m.chat.Messages = append(m.chat.Messages, chatMessage{
		Role: roleNotice, Content: strings.TrimRight(b.String(), "\n"),
	})
	m.refreshChatViewport()
}

// completerQuery returns the filter text after "/model " if the input
// currently matches the command prefix. Returns ("", false) otherwise.
func (m *Model) completerQuery() (string, bool) {
	val := m.chat.Input.Value()
	if len(val) >= len(modelCommandPrefix) &&
		strings.EqualFold(val[:len(modelCommandPrefix)], modelCommandPrefix) {
		return val[len(modelCommandPrefix):], true
	}
	return "", false
}

// activateCompleter opens the model completer, fetching models if needed.
func (m *Model) activateCompleter() tea.Cmd {
	if m.chat.Completer != nil {
		return nil // already active
	}
	m.chat.Completer = &modelCompleter{Loading: true}

	if m.llmClient == nil {
		m.chat.Completer.Loading = false
		return nil
	}
	client := m.llmClient
	timeout := client.Timeout()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		models, err := client.ListModels(ctx)
		return modelsListMsg{Models: models, Err: err}
	}
}

// deactivateCompleter closes the model completer.
func (m *Model) deactivateCompleter() {
	m.chat.Completer = nil
}

// mergeModelLists combines server-local models with well-known models,
// putting local models first and deduplicating.
func mergeModelLists(serverModels []string) []modelCompleterEntry {
	seen := make(map[string]bool, len(serverModels)+len(wellKnownModels))
	var all []modelCompleterEntry

	// Server models first (these are local).
	for _, name := range serverModels {
		seen[name] = true
		all = append(all, modelCompleterEntry{Name: name, Local: true})
	}

	// Well-known models that aren't already local.
	for _, name := range wellKnownModels {
		if !seen[name] {
			all = append(all, modelCompleterEntry{Name: name, Local: false})
		}
	}

	return all
}

// refilterCompleter updates the chat completer match list from the current input.
func (m *Model) refilterCompleter() {
	mc := m.chat.Completer
	if mc == nil {
		return
	}
	query, _ := m.completerQuery()
	refilterModelCompleter(mc, query, m.llmModelLabel())
}

// refilterModelCompleter updates a model completer's match list for the given
// filter query, marking current as the active model.
func refilterModelCompleter(mc *modelCompleter, query, current string) {
	if query == "" {
		mc.Matches = make([]modelCompleterMatch, len(mc.All))
		for i, entry := range mc.All {
			mc.Matches[i] = modelCompleterMatch{
				Name: entry.Name, Index: i, Local: entry.Local,
				Active: entry.Name == current,
			}
		}
		mc.clampCursor()
		return
	}

	mc.Matches = mc.Matches[:0]
	for i, entry := range mc.All {
		if score, positions := fuzzyMatch(query, entry.Name); score > 0 {
			mc.Matches = append(mc.Matches, modelCompleterMatch{
				Name: entry.Name, Score: score, Index: i, Positions: positions,
				Active: entry.Name == current, Local: entry.Local,
			})
		}
	}
	sortFuzzyScored(mc.Matches)
	mc.clampCursor()
}

func (mc *modelCompleter) clampCursor() {
	if mc.Cursor >= len(mc.Matches) {
		mc.Cursor = len(mc.Matches) - 1
	}
	if mc.Cursor < 0 {
		mc.Cursor = 0
	}
}

// cmdSwitchModel switches to a model, pulling it via the Ollama API if needed.
func (m *Model) cmdSwitchModel(name string) tea.Cmd {
	if m.llmClient == nil {
		return nil
	}
	if m.pull.active {
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleError, Content: "a model pull is already in progress",
		})
		m.refreshChatViewport()
		return nil
	}

	m.pull.fromChat = true
	m.pull.display = "checking " + name + symEllipsis
	m.resizeTables()

	client := m.llmClient
	timeout := client.Timeout()
	baseURL := client.BaseURL()
	isLocal := client.IsLocalServer()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		// Best-effort: if listing fails, fall through to pull attempt.
		models, _ := client.ListModels(ctx)
		for _, model := range models {
			if model == name || strings.HasPrefix(model, name+":") {
				return pullProgressMsg{
					Status: "Switched to " + model,
					Done:   true,
					Model:  model,
				}
			}
		}
		if !isLocal {
			return pullProgressMsg{
				Err:  fmt.Errorf("model %q not found -- check the model name", name),
				Done: true,
			}
		}
		return startPull(baseURL, name)
	}
}

// startPull initiates a model pull via the Ollama HTTP API and returns the
// first progress message.
func startPull(baseURL, name string) tea.Msg {
	ctx, cancel := context.WithCancel(context.Background())
	scanner, err := ollamaPull.PullModel(ctx, baseURL, name)
	if err != nil {
		cancel()
		return pullProgressMsg{Err: err, Done: true}
	}

	ps := &ollamaPullState{Model: name, Cancel: cancel, Scanner: scanner}
	return readNextPullChunk(ps)
}

// readNextPullChunk reads the next JSON line from the pull stream.
func readNextPullChunk(ps *ollamaPullState) tea.Msg {
	chunk, err := ps.Scanner.Next()
	if err != nil {
		ps.Cancel()
		return pullProgressMsg{Err: err, Done: true, PullState: ps, Model: ps.Model}
	}
	if chunk == nil {
		ps.Cancel()
		return pullProgressMsg{
			Status:    ps.Model + " ready",
			Done:      true,
			Model:     ps.Model,
			PullState: ps,
		}
	}
	// Check if Ollama streamed an error in the chunk itself.
	if chunk.Error != "" {
		ps.Cancel()
		return pullProgressMsg{
			Err:       fmt.Errorf("%s", chunk.Error),
			Done:      true,
			PullState: ps,
			Model:     ps.Model,
		}
	}
	pct := -1.0
	if chunk.Total > 0 {
		pct = float64(chunk.Completed) / float64(chunk.Total)
	}
	return pullProgressMsg{
		Status:    chunk.Status,
		Percent:   pct,
		PullState: ps,
		Model:     ps.Model,
	}
}

// cleanPullStatus tidies up Ollama's raw status strings into something
// readable. Ollama sends things like "pulling sha256:abcdef123456..." which
// aren't useful for the user.
func cleanPullStatus(status, model string) string {
	s := strings.ToLower(status)
	switch {
	case strings.HasPrefix(s, "pulling manifest"):
		return "pulling " + model
	case strings.HasPrefix(s, "pulling"):
		return "downloading " + model
	case strings.HasPrefix(s, "verifying"):
		return "verifying " + model
	case strings.HasPrefix(s, "writing"):
		return "finalizing " + model
	case s == "success":
		return "ready"
	default:
		return status
	}
}

// handleSQLResult processes the stage 1 output. On success, it kicks off
// stage 2 (streaming summary). On failure, it falls back to the single-stage
// approach with the full data dump.
func (m *Model) handleSQLResult(msg sqlResultMsg) tea.Cmd {
	if m.chat == nil {
		return nil
	}

	// Remove the "generating query" notice.
	m.removeLastNotice()

	if msg.Err != nil {
		// Fall back to single-stage: dump all data and ask directly.
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleNotice, Content: "falling back to direct query" + symEllipsis,
		})
		m.refreshChatViewport()
		return m.startFallbackStream(msg.Question)
	}

	// The SQL is already stored in the assistant message's SQL field.
	// Stage 2: summarize results via streaming LLM call.
	// Always send unformatted numbers to the LLM so the stored response
	// contains regular dollar amounts. Client-side magTransformText handles
	// mag notation at render time, making it toggleable.
	resultsTable := llm.FormatResultsTable(msg.Columns, msg.Rows)
	summaryPrompt := llm.BuildSummaryPrompt(
		msg.Question,
		msg.SQL,
		resultsTable,
		time.Now(),
		m.llmExtraContext,
	)

	messages := []llm.Message{
		{Role: "system", Content: summaryPrompt},
		{Role: roleUser, Content: "Summarize these results."},
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := m.llmClient.ChatStream(ctx, messages)
	if err != nil {
		cancel()
		m.chat.Streaming = false
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleError, Content: err.Error(),
		})
		m.refreshChatViewport()
		return nil
	}

	m.chat.StreamCh = ch
	m.chat.CancelFn = cancel
	// The assistant message already exists from stage 1; we'll populate its Content field.
	m.refreshChatViewport()

	return waitForChunk(ch)
}

// startFallbackStream uses the old single-stage approach: full data dump in
// the system prompt, streamed response.
func (m *Model) startFallbackStream(question string) tea.Cmd {
	messages := m.buildFallbackMessages(question)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := m.llmClient.ChatStream(ctx, messages)
	if err != nil {
		cancel()
		m.chat.Streaming = false
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleError, Content: err.Error(),
		})
		m.refreshChatViewport()
		return nil
	}

	m.chat.StreamCh = ch
	m.chat.CancelFn = cancel
	m.chat.Messages = append(m.chat.Messages, chatMessage{
		Role: roleAssistant, Content: "",
	})
	m.refreshChatViewport()

	return waitForChunk(ch)
}

// historyBack moves to the previous entry in the prompt history.
func (m *Model) historyBack() {
	if len(m.chat.History) == 0 {
		return
	}
	if m.chat.HistoryCur == -1 {
		// Entering history -- stash the current live input.
		m.chat.HistoryBuf = m.chat.Input.Value()
		m.chat.HistoryCur = len(m.chat.History) - 1
	} else if m.chat.HistoryCur > 0 {
		m.chat.HistoryCur--
	} else {
		return // already at oldest
	}
	m.chat.Input.SetValue(m.chat.History[m.chat.HistoryCur])
	m.chat.Input.CursorEnd()
}

// historyForward moves to the next entry in the prompt history, or back
// to the live input buffer.
func (m *Model) historyForward() {
	if m.chat.HistoryCur == -1 {
		return // not browsing history
	}
	if m.chat.HistoryCur < len(m.chat.History)-1 {
		m.chat.HistoryCur++
		m.chat.Input.SetValue(m.chat.History[m.chat.HistoryCur])
		m.chat.Input.CursorEnd()
	} else {
		// Return to live input.
		m.chat.HistoryCur = -1
		m.chat.Input.SetValue(m.chat.HistoryBuf)
		m.chat.Input.CursorEnd()
	}
}

// toggleSQL flips the SQL display flag. The state is reflected in the
// hint bar color -- no chat notice needed.
func (m *Model) toggleSQL() {
	if m.chat == nil {
		return
	}
	m.chat.ShowSQL = !m.chat.ShowSQL
	// Refresh viewport to immediately show/hide SQL for all messages.
	m.refreshChatViewport()
}

func (m *Model) toggleMagMode() {
	m.magMode = !m.magMode
	if m.chat != nil {
		m.refreshChatViewport()
	}
}

// sqlHintItem renders the ctrl+s hint with color indicating whether SQL
// display is active: accent when on, dim when off.
func (m *Model) sqlHintItem() string {
	keycaps := m.renderKeys(keyCtrlS)
	label := "sql"
	var style lipgloss.Style
	if m.chat != nil && m.chat.ShowSQL {
		style = m.styles.AccentBold()
	} else {
		style = m.styles.HeaderHint()
	}
	return strings.TrimSpace(keycaps + " " + style.Render(label))
}

// removeLastNotice removes the most recent notice message from the chat.
func (m *Model) removeLastNotice() {
	for i := len(m.chat.Messages) - 1; i >= 0; i-- {
		if m.chat.Messages[i].Role == roleNotice {
			m.chat.Messages = append(m.chat.Messages[:i], m.chat.Messages[i+1:]...)
			return
		}
	}
}

// replaceAssistantWithError removes the last assistant message (if present),
// appends an error message, and refreshes the viewport. Used by stream error
// handlers where the incomplete assistant message should be discarded.
func (m *Model) replaceAssistantWithError(errMsg string) {
	msgs := m.chat.Messages
	if n := len(msgs); n > 0 && msgs[n-1].Role == roleAssistant {
		m.chat.Messages = msgs[:n-1]
	}
	m.chat.Messages = append(m.chat.Messages, chatMessage{
		Role: roleError, Content: errMsg,
	})
	m.refreshChatViewport()
}

// handleSQLStreamStarted processes the initial SQL stream setup.
func (m *Model) handleSQLStreamStarted(msg sqlStreamStartedMsg) tea.Cmd {
	if msg.Err != nil {
		m.chat.Streaming = false
		m.chat.StreamingSQL = false
		m.removeLastNotice()
		m.replaceAssistantWithError(msg.Err.Error())
		return nil
	}

	// Store the cancel function, channel, and question, then start reading chunks.
	m.chat.CancelFn = msg.CancelFn
	m.chat.SQLStreamCh = msg.Channel
	m.chat.CurrentQuery = msg.Question
	return waitForSQLChunk(msg.Channel)
}

// waitForSQLChunk returns a Cmd that reads the next SQL chunk from the stream.
func waitForSQLChunk(ch <-chan llm.StreamChunk) tea.Cmd {
	return waitForStream(ch, func(c llm.StreamChunk) tea.Msg {
		return sqlChunkMsg{Content: c.Content, Done: c.Done, Err: c.Err}
	}, nil)
}

// handleSQLChunk processes a single SQL token from the stream.
func (m *Model) handleSQLChunk(msg sqlChunkMsg) tea.Cmd {
	// Drop chunks that arrive after cancellation has already cleaned up.
	if !m.chat.Streaming {
		return nil
	}

	if msg.Err != nil {
		m.chat.Streaming = false
		m.chat.StreamingSQL = false
		m.chat.CancelFn = nil
		m.removeLastNotice()
		m.replaceAssistantWithError(msg.Err.Error())
		return nil
	}

	// Append to the SQL field of the assistant message (last message should be the assistant one).
	if len(m.chat.Messages) > 0 {
		lastIdx := len(m.chat.Messages) - 1
		if m.chat.Messages[lastIdx].Role == roleAssistant {
			m.chat.Messages[lastIdx].SQL += msg.Content
			m.refreshChatViewport()
		}
	}

	if msg.Done {
		// SQL generation complete. Extract, validate, and execute.
		m.chat.StreamingSQL = false
		m.chat.SQLStreamCh = nil
		sql := ""
		if len(m.chat.Messages) > 0 &&
			m.chat.Messages[len(m.chat.Messages)-1].Role == roleAssistant {
			sql = llm.ExtractSQL(m.chat.Messages[len(m.chat.Messages)-1].SQL)
		}
		m.removeLastNotice() // Remove "generating query"

		if sql == "" {
			m.chat.Streaming = false
			m.replaceAssistantWithError("LLM returned empty SQL")
			return nil
		}

		// Store the cleaned SQL back into the message.
		if len(m.chat.Messages) > 0 &&
			m.chat.Messages[len(m.chat.Messages)-1].Role == roleAssistant {
			m.chat.Messages[len(m.chat.Messages)-1].SQL = sql
		}

		// Execute the SQL query.
		return m.executeSQLQuery(sql)
	}

	// More chunks coming.
	return waitForSQLChunk(m.chat.SQLStreamCh)
}

// executeSQLQuery runs the generated SQL and starts stage 2 (summary).
func (m *Model) executeSQLQuery(sql string) tea.Cmd {
	store := m.store
	query := m.chat.CurrentQuery

	return func() tea.Msg {
		cols, rows, err := store.ReadOnlyQuery(sql)
		if err != nil {
			return sqlResultMsg{Question: query, SQL: sql, Err: fmt.Errorf("query error: %w", err)}
		}

		return sqlResultMsg{
			Question: query,
			SQL:      sql,
			Columns:  cols,
			Rows:     rows,
		}
	}
}

func (m *Model) handleChatChunk(msg chatChunkMsg) tea.Cmd {
	if m.chat == nil || !m.chat.Streaming {
		return nil
	}

	if msg.Err != nil {
		m.chat.Streaming = false
		m.chat.CancelFn = nil
		m.chat.Messages = append(m.chat.Messages, chatMessage{
			Role: roleError, Content: msg.Err.Error(),
		})
		m.refreshChatViewport()
		return nil
	}

	if len(m.chat.Messages) > 0 && msg.Content != "" {
		last := &m.chat.Messages[len(m.chat.Messages)-1]
		if last.Role == roleAssistant {
			last.Content += msg.Content
		}
	}
	m.refreshChatViewport()

	if msg.Done {
		m.chat.Streaming = false
		m.chat.CancelFn = nil
		return nil
	}

	return waitForChunk(m.chat.StreamCh)
}

// buildFallbackMessages assembles the full message list for the single-stage
// fallback: system prompt with schema + full data dump, conversation history, then the question.
func (m *Model) buildFallbackMessages(question string) []llm.Message {
	tables := m.buildTableInfo()
	dataDump := ""
	if m.store != nil {
		dataDump = m.store.DataDump()
	}
	systemPrompt := llm.BuildSystemPrompt(
		tables,
		dataDump,
		time.Now(),
		m.llmExtraContext,
	)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	messages = append(messages, m.buildConversationHistory()...)
	messages = append(messages, llm.Message{Role: roleUser, Content: question})
	return messages
}

// buildConversationHistory converts the chat message history into LLM messages.
// Only includes user and assistant messages (not notices or errors) up to the
// last complete assistant response. Excludes the pending/streaming message.
func (m *Model) buildConversationHistory() []llm.Message {
	if m.chat == nil || len(m.chat.Messages) == 0 {
		return nil
	}

	var history []llm.Message
	// Iterate through messages, but stop before any incomplete assistant message.
	for i, msg := range m.chat.Messages {
		// Skip the last message if it's an incomplete assistant message (currently streaming).
		if i == len(m.chat.Messages)-1 &&
			msg.Role == roleAssistant &&
			msg.Content == "" &&
			(m.chat.Streaming || m.chat.StreamingSQL) {
			break
		}

		switch msg.Role {
		case roleUser:
			history = append(history, llm.Message{Role: "user", Content: msg.Content})
		case roleAssistant:
			// Only include completed assistant messages.
			if msg.Content != "" {
				history = append(history, llm.Message{Role: "assistant", Content: msg.Content})
			}
			// Skip roleError and roleNotice - these are UI elements, not conversation.
		}
	}

	return history
}

// buildTableInfo queries the database schema and returns it in the format
// the prompt builder expects.
func (m *Model) buildTableInfo() []llm.TableInfo {
	return buildTableInfoFrom(m.store)
}

// buildTableInfoFrom queries schema metadata from the store. Extracted so it
// can be called from background goroutines without holding a Model reference.
func buildTableInfoFrom(store *data.Store) []llm.TableInfo {
	if store == nil {
		return nil
	}
	names, err := store.TableNames()
	if err != nil {
		return nil
	}
	var tables []llm.TableInfo
	for _, name := range names {
		cols, err := store.TableColumns(name)
		if err != nil {
			continue
		}
		t := llm.TableInfo{Name: name}
		for _, c := range cols {
			t.Columns = append(t.Columns, llm.ColumnInfo{
				Name:    c.Name,
				Type:    c.Type,
				NotNull: c.NotNull,
				PK:      c.PK > 0,
			})
		}
		tables = append(tables, t)
	}
	return tables
}

// waitForChunk returns a Cmd that blocks until the next chunk arrives on the
// channel, then delivers it as a chatChunkMsg.
func waitForChunk(ch <-chan llm.StreamChunk) tea.Cmd {
	return waitForStream(ch, func(c llm.StreamChunk) tea.Msg {
		return chatChunkMsg{Content: c.Content, Done: c.Done, Err: c.Err}
	}, nil)
}

// refreshChatViewport rebuilds the viewport content from the message history.
func (m *Model) refreshChatViewport() {
	if m.chat == nil {
		return
	}
	content := m.renderChatMessages()
	m.chat.Viewport.SetContent(content)
	m.chat.Viewport.GotoBottom()
}

// renderChatMessages formats the conversation for display in the viewport.
func (m *Model) renderChatMessages() string {
	if m.chat == nil {
		return ""
	}

	innerW := m.chatViewportWidth()
	var parts []string
	for i, msg := range m.chat.Messages {
		var rendered string
		switch msg.Role {
		case roleUser:
			label := m.styles.ChatUser().Render("›")
			// Inline compact: label then content on same line.
			textW := innerW - lipgloss.Width(label) - 1
			text := wordWrap(msg.Content, textW)
			rendered = label + " " + text
		case roleAssistant:
			label := m.styles.ChatAssistant().Render(m.llmModelLabel())
			text := msg.Content
			sql := msg.SQL
			isLastMessage := i == len(m.chat.Messages)-1

			var parts []string

			// Show SQL if toggle is on and SQL exists.
			if m.chat.ShowSQL && sql != "" {
				sqlWidth := innerW - 8
				if sqlWidth < 30 {
					sqlWidth = 30
				}
				sqlBlock := m.chat.renderMarkdown(
					"```sql\n"+llm.FormatSQL(sql, sqlWidth)+"\n```",
					innerW-2,
				)
				parts = append(parts, sqlBlock)
			}

			// Show response if available.
			if text != "" {
				display := text
				if m.magMode {
					display = magTransformText(display, m.cur.Symbol())
				}
				parts = append(parts, m.chat.renderMarkdown(display, innerW-2))
			}

			// Join content parts, trimming glamour's leading whitespace.
			body := strings.TrimLeft(strings.Join(parts, "\n"), "\n")

			// Determine what to show on the label line.
			// Only show spinner for the currently streaming message (last one).
			if isLastMessage && m.chat.StreamingSQL && sql == "" {
				// Stage 1: generating SQL query
				rendered = label + "  " + m.chat.Spinner.View() + " " + m.styles.HeaderHint().
					Render(
						"generating query",
					)
			} else if isLastMessage && text == "" && m.chat.Streaming && !m.chat.StreamingSQL {
				// Stage 2: thinking about response (may have SQL already)
				labelLine := label + "  " + m.chat.Spinner.View() + " " + m.styles.HeaderHint().Render("thinking")
				if body != "" {
					rendered = labelLine + "\n" + body
				} else {
					rendered = labelLine
				}
			} else if body != "" {
				rendered = label + "\n" + body
			} else {
				rendered = label
			}

			// Add subtle separator after assistant response (end of Q&A pair).
			// Skip if it's the last message to avoid trailing separator.
			if i < len(m.chat.Messages)-1 && text != "" {
				sep := strings.Repeat("─", innerW)
				rendered += "\n" + m.styles.TextDim().Render(sep)
			}
		case roleError:
			rendered = m.styles.Error().Render("error: " + wordWrap(msg.Content, innerW-9))
		case roleNotice:
			// Skip "generating query" notice - status is shown inline with model label.
			if msg.Content == "generating query" {
				continue
			}
			if msg.Content == "Interrupted" || msg.Content == "Pull cancelled" {
				rendered = m.styles.ChatInterrupted().Render(msg.Content)
			} else {
				rendered = m.styles.ChatNotice().Render(msg.Content)
			}
		}
		parts = append(parts, rendered)
	}
	return strings.Join(parts, "\n")
}

func (m *Model) llmModelLabel() string {
	if m.llmClient != nil {
		return m.llmClient.Model()
	}
	return "LLM"
}

// handleChatKey processes keys when the chat overlay is active.
func (m *Model) handleChatKey(key tea.KeyMsg) tea.Cmd {
	// Completer navigation takes priority over normal input handling.
	if mc := m.chat.Completer; mc != nil && !mc.Loading {
		switch key.String() {
		case keyEsc:
			// Dismiss completer but keep "/model " in the input so the user
			// can edit and re-trigger.
			m.deactivateCompleter()
			return nil
		case keyUp, keyCtrlP:
			if mc.Cursor > 0 {
				mc.Cursor--
			}
			return nil
		case keyDown, keyCtrlN:
			if mc.Cursor < len(mc.Matches)-1 {
				mc.Cursor++
			}
			return nil
		case keyEnter:
			if len(mc.Matches) > 0 {
				selected := mc.Matches[mc.Cursor].Name
				m.deactivateCompleter()
				m.chat.Input.SetValue("")
				return m.cmdSwitchModel(selected)
			}
			m.deactivateCompleter()
			return nil
		case keyCtrlQ:
			return tea.Quit
		}
	}

	switch key.String() {
	case keyEsc:
		m.hideChat()
		return nil
	case keyEnter:
		if m.chat.Streaming {
			return nil
		}
		return m.submitChat()
	case keyCtrlS:
		m.toggleSQL()
		return nil
	case keyCtrlO:
		m.toggleMagMode()
		return nil
	case keyCtrlC:
		// Handled by the global ctrl+c handler in model.Update which calls
		// cancelChatOperations. This case is unreachable but kept for clarity.
		return nil
	case keyUp, keyCtrlP:
		if m.chat.Input.Focused() && !m.chat.Streaming {
			m.historyBack()
			return nil
		}
	case keyDown, keyCtrlN:
		if m.chat.Input.Focused() && !m.chat.Streaming {
			m.historyForward()
			return nil
		}
	}

	// Let the text input handle the keystroke, then check whether we need
	// to activate or deactivate the completer based on the new input value.
	if m.chat.Input.Focused() {
		var cmd tea.Cmd
		m.chat.Input, cmd = m.chat.Input.Update(key)
		return m.syncCompleter(cmd)
	}

	var cmd tea.Cmd
	m.chat.Viewport, cmd = m.chat.Viewport.Update(key)
	return cmd
}

// syncCompleter checks the current input value and activates/deactivates
// the model completer as needed. It wraps an existing Cmd from the text
// input update so both can be batched.
func (m *Model) syncCompleter(inputCmd tea.Cmd) tea.Cmd {
	_, isModelCmd := m.completerQuery()
	if isModelCmd {
		if m.chat.Completer == nil {
			// Just crossed into "/model " territory -- activate.
			fetchCmd := m.activateCompleter()
			if fetchCmd != nil {
				return tea.Batch(inputCmd, fetchCmd)
			}
		} else {
			// Already active -- just refilter.
			m.refilterCompleter()
		}
	} else if m.chat.Completer != nil {
		// Input no longer starts with "/model " -- dismiss.
		m.deactivateCompleter()
	}
	return inputCmd
}

// --- Chat overlay rendering ---

func (m *Model) buildChatOverlay() string {
	if m.chat == nil {
		return ""
	}

	contentW := m.chatOverlayWidth()
	innerW := contentW - 4

	titleText := " Ask "
	if m.llmClient != nil {
		titleText = " " + m.llmClient.Model() + " "
	}
	title := m.styles.HeaderSection().Render(titleText)

	vpH := m.chatViewportHeight()
	m.chat.Viewport.Width = innerW
	m.chat.Viewport.Height = vpH
	vpView := m.chat.Viewport.View()

	m.chat.Input.Width = innerW - 2
	inputView := m.chat.Input.View()

	// Model completer list (between input and viewport).
	completerView := m.renderModelCompleter(innerW)

	var hintParts []string
	if m.chat.Completer != nil {
		hintParts = append(hintParts,
			m.helpItem(keyUp+"/"+keyDown, "navigate"),
			m.helpItem(symReturn, "select"),
			m.helpItem(keyEsc, "dismiss"),
		)
	} else {
		hintParts = append(hintParts,
			m.helpItem(symReturn, "send"),
			m.sqlHintItem(),
			m.helpItem(symUp+"/"+symDown, "history"),
			m.helpItem(keyEsc, "hide"),
		)
	}
	hints := joinWithSeparator(m.helpSeparator(), hintParts...)

	// Build layout: title, viewport, [completer], input, hints.
	sections := []string{title, "", vpView, ""}
	if completerView != "" {
		sections = append(sections, completerView, "")
	}
	sections = append(sections, inputView, "", hints)

	boxContent := lipgloss.JoinVertical(lipgloss.Left, sections...)

	maxH := m.effectiveHeight() * 3 / 5
	if maxH < 12 {
		maxH = 12
	}

	return m.styles.OverlayBox().
		Width(contentW).
		MaxHeight(maxH).
		Render(boxContent)
}

// renderModelCompleter renders the inline model completion list with a
// fixed height of completerMaxLines so the overlay doesn't shift.
func (m *Model) renderModelCompleter(innerW int) string {
	query, _ := m.completerQuery()
	return m.renderModelCompleterFor(m.chat.Completer, query, innerW)
}

// renderModelCompleterFor renders the model completion list for any completer.
func (m *Model) renderModelCompleterFor(mc *modelCompleter, query string, innerW int) string {
	if mc == nil {
		return ""
	}

	lines := make([]string, completerMaxLines)

	if mc.Loading {
		lines[0] = m.styles.HeaderHint().Render("  loading models" + symEllipsis)
		for i := 1; i < completerMaxLines; i++ {
			lines[i] = ""
		}
		return strings.Join(lines, "\n")
	}

	if len(mc.Matches) == 0 {
		if query != "" {
			lines[0] = m.styles.Empty().Render("  no matching models")
		} else {
			lines[0] = m.styles.Empty().Render("  no models available")
		}
		for i := 1; i < completerMaxLines; i++ {
			lines[i] = ""
		}
		return strings.Join(lines, "\n")
	}

	maxVisible := completerMaxLines
	if maxVisible > len(mc.Matches) {
		maxVisible = len(mc.Matches)
	}
	start := mc.Cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(mc.Matches) {
		end = len(mc.Matches)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	pointer := m.styles.AccentBold()
	lineIdx := 0
	for i := start; i < end; i++ {
		entry := mc.Matches[i]
		selected := i == mc.Cursor

		label := m.highlightModelMatch(entry)

		var line string
		if selected {
			line = pointer.Render("▸ ") + label
		} else {
			line = "  " + label
		}

		if lipgloss.Width(line) > innerW {
			line = m.styles.Base().MaxWidth(innerW).Render(line)
		}
		lines[lineIdx] = line
		lineIdx++
	}
	// Pad remaining lines to maintain fixed height.
	for i := lineIdx; i < completerMaxLines; i++ {
		lines[i] = ""
	}

	return strings.Join(lines, "\n")
}

// highlightModelMatch renders a model name styled by its state:
//   - Active:                accent (sky blue) + bold
//   - Local but inactive:   bright text (clearly available)
//   - Not downloaded:        dim text + italic (needs fetching)
//
// Fuzzy-matched characters get the accent highlight regardless.
func (m *Model) highlightModelMatch(match modelCompleterMatch) string {
	var baseStyle, highlightStyle lipgloss.Style
	switch {
	case match.Active:
		baseStyle = m.styles.ModelActive()
		highlightStyle = m.styles.ModelActiveHL()
	case match.Local:
		baseStyle = m.styles.ModelLocal()
		highlightStyle = m.styles.ModelLocalHL()
	default:
		baseStyle = m.styles.ModelRemote()
		highlightStyle = m.styles.ModelRemoteHL()
	}
	return highlightFuzzyPositions(match.Name, match.Positions, baseStyle, highlightStyle)
}

// --- Layout helpers ---

func (m *Model) chatOverlayWidth() int {
	w := m.effectiveWidth() - 8
	if w > 90 {
		w = 90
	}
	if w < 40 {
		w = 40
	}
	return w
}

func (m *Model) chatViewportWidth() int {
	return m.chatOverlayWidth() - 8
}

func (m *Model) chatViewportHeight() int {
	maxOverlay := m.effectiveHeight() * 3 / 5
	// Chrome: border(2) + padding(2) + title(1) + blanks(3)
	// + input(1) + blank(1) + hints(1) = 11 lines.
	chrome := 11
	if m.chat != nil && m.chat.Completer != nil {
		// Reserve space for the completer + its surrounding blank line.
		chrome += completerMaxLines + 1
	}
	h := maxOverlay - chrome
	if h < 4 {
		h = 4
	}
	return h
}

func (m *Model) chatInputWidth() int {
	return m.chatOverlayWidth() - 10
}
