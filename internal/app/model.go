// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/micasa-dev/micasa/internal/config"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
	"github.com/micasa-dev/micasa/internal/llm"
	"github.com/micasa-dev/micasa/internal/locale"
	"github.com/micasa-dev/micasa/internal/sync"
	"gorm.io/gorm"
)

const (
	// Navigation keys.
	keyUp        = "up"
	keyDown      = "down"
	keyLeft      = "left"
	keyRight     = "right"
	keyTab       = "tab"
	keyBackspace = "backspace"
	keyPgUp      = "pgup"
	keyPgDown    = "pgdown"
	keyShiftDown = "shift+down"
	keyShiftUp   = "shift+up"
	keyShiftTab  = "shift+tab"

	// Action keys.
	keyEsc   = "esc"
	keyEnter = "enter"

	// Modifier keys.
	keyCtrlC = "ctrl+c"
	keyCtrlD = "ctrl+d"
	keyCtrlE = "ctrl+e"
	keyCtrlF = "ctrl+f"
	keyCtrlJ = "ctrl+j"
	keyCtrlK = "ctrl+k"
	keyCtrlN = "ctrl+n"
	keyCtrlO = "ctrl+o"
	keyCtrlP = "ctrl+p"
	keyCtrlB = "ctrl+b"
	keyCtrlQ = "ctrl+q"
	keyCtrlS = "ctrl+s"
	keyCtrlU = "ctrl+u"

	// Letters (lower).
	keyA = "a"
	keyB = "b"
	keyC = "c"
	keyD = "d"
	keyE = "e"
	keyF = "f"
	keyG = "g"
	keyH = "h"
	keyI = "i"
	keyJ = "j"
	keyK = "k"
	keyL = "l"
	keyN = "n"
	keyO = "o"
	keyP = "p"
	keyR = "r"
	keyS = "s"
	keyT = "t"
	keyU = "u"
	keyX = "x"
	keyY = "y"

	// Letters (upper / shift).
	keyShiftA = "A"
	keyShiftB = "B"
	keyShiftC = "C"
	keyShiftD = "D"
	keyShiftE = "E"
	keyShiftF = "F"
	keyShiftG = "G"
	keyShiftH = "H"
	keyShiftJ = "J"
	keyShiftK = "K"
	keyShiftL = "L"
	keyShiftN = "N"
	keyShiftS = "S"
	keyShiftU = "U"

	// Symbols.
	keyBang     = "!"
	keySlash    = "/"
	keyQuestion = "?"
	keyAt       = "@"
	keyCaret    = "^"
	keyDollar   = "$"
	keyLBracket = "["
	keyRBracket = "]"

	// Display symbols for key hints.
	symReturn = "\u21b5" // ↵
	symUp     = "\u2191" // ↑
	symDown   = "\u2193" // ↓
	symLeft   = "\u2190" // ←
	symRight  = "\u2192" // →
	symCtrlB  = "^b"
	symCtrlC  = "^c"

	// Box drawing.
	symHLine      = "\u2500" // ─
	symHLineHeavy = "\u2501" // ━
	symVLine      = "\u2502" // │
	symCross      = "\u253c" // ┼

	// Triangles / cursors.
	symTriUp      = "\u25b2" // ▲
	symTriDown    = "\u25bc" // ▼
	symTriDownSm  = "\u25be" // ▾
	symTriRightSm = "\u25b8" // ▸
	symTriLeft    = "\u25c0" // ◀
	symTriRight   = "\u25b6" // ▶

	// Text symbols.
	symEllipsis  = "\u2026" // …
	symEmptySet  = "\u2205" // ∅
	symEmDash    = "\u2014" // —
	symInfinity  = "\u221E" // ∞
	symMiddleDot = "\u00b7" // ·
)

// Key bindings for help viewport (g/G for top/bottom are not in the
// default viewport keymap).
var (
	helpGotoTop    = key.NewBinding(key.WithKeys(keyG))
	helpGotoBottom = key.NewBinding(key.WithKeys(keyShiftG))
)

// pullState groups fields for an in-progress Ollama model pull.
type pullState struct {
	active   bool               // true while a pull is in progress
	fromChat bool               // true when initiated from chat /model
	display  string             // status bar progress text
	peak     float64            // high-water mark for progress bar
	cancel   context.CancelFunc // cancel in-flight pull
	progress progress.Model     // bubbles progress bar widget
}

// dashState groups dashboard overlay fields.
type dashState struct {
	data         dashboardData
	cursor       int
	nav          []dashNavEntry
	expanded     map[string]bool
	scrollOffset int
	totalLines   int
	flash        string
}

// notePreviewState holds the text shown in the note preview overlay.
type notePreviewState struct {
	text  string
	title string
}

type Model struct {
	zones                 *zone.Manager
	store                 *data.Store
	dbPath                string
	configPath            string
	llmClient             *llm.Client
	chatCfg               chatConfig
	filePickerDir         string // starting directory for document file picker
	ex                    extractState
	pull                  pullState
	chat                  *chatState // non-nil when chat overlay is open
	styles                *Styles
	tabs                  []Tab
	active                int
	detailStack           []*detailContext // drilldown stack; top is active detail view
	width                 int
	height                int
	helpViewport          *viewport.Model
	showHouse             bool
	showDashboard         bool
	notePreview           *notePreviewState
	opsTree               *opsTreeState
	calendar              *calendarState
	columnFinder          *columnFinderState
	docSearch             *docSearchState
	dash                  dashState
	unitSystem            data.UnitSystem
	hasHouse              bool
	house                 data.HouseProfile
	mode                  Mode
	prevMode              Mode // mode to restore after form closes
	fs                    formState
	inlineInput           *inlineInputState
	magMode               bool        // easter egg: display numbers as order-of-magnitude
	confirm               confirmKind // active confirmation dialog (zero = none)
	hardDeleteID          string      // entity ID pending permanent deletion
	lastRowClick          rowClickState
	lastDashClick         rowClickState
	isDark                bool // terminal background is dark
	cur                   locale.Currency
	status                statusMsg
	projectTypes          []data.ProjectType
	maintenanceCategories []data.MaintenanceCategory
	vendors               []data.Vendor

	// Postal code auto-fill.
	addressClient   *http.Client
	addressBaseURL  string
	addressCountry  string
	addressAutofill bool

	// App lifecycle context: cancelled on quit, parent of all feature contexts.
	// Access via lifecycleCtx() which provides a nil-safe fallback for tests.
	appCtx    context.Context
	appCancel context.CancelFunc

	// Sync state (Pro background sync).
	syncStatus        syncStatus
	syncCfg           *syncConfig
	syncEngine        *sync.Engine
	syncCtx           context.Context
	syncCancel        context.CancelFunc
	syncDebounceGen   int
	syncPendingReload bool // true when pulled data awaits form close
}

func NewModel(store *data.Store, options Options) (*Model, error) {
	appCtx, appCancel := context.WithCancel(context.Background())
	defer func() {
		if appCancel != nil {
			appCancel()
		}
	}()

	var client *llm.Client
	chatCfg := options.ChatConfig
	if chatCfg.Enabled {
		model := chatCfg.Model
		// Prefer the last-used model from the database if available.
		if persisted, err := store.GetLastModel(); err == nil && persisted != "" {
			model = persisted
		} else if chatCfg.Provider == "ollama" {
			// No persisted model -- try auto-detecting if the server has exactly one.
			tempClient, err := llm.NewClient(chatCfg.Provider, chatCfg.BaseURL, model, chatCfg.APIKey, chatCfg.Timeout)
			if err == nil {
				if detected := autoDetectModel(appCtx, tempClient); detected != "" {
					model = detected
					_ = store.PutLastModel(model)
				}
			}
		}
		var err error
		client, err = llm.NewClient(
			chatCfg.Provider,
			chatCfg.BaseURL,
			model,
			chatCfg.APIKey,
			chatCfg.Timeout,
		)
		if err != nil {
			return nil, fmt.Errorf("create llm client: %w", err)
		}
		if chatCfg.Thinking != "" {
			client.SetThinking(chatCfg.Thinking)
		}
	}

	pprog := progress.New(
		progress.WithColors(textDimPair.resolve(true), accentPair.resolve(true)),
		progress.WithFillCharacters('━', '┄'),
	)
	pprog.PercentageStyle = appStyles.TextDim()

	model := &Model{
		appCtx:        appCtx,
		appCancel:     appCancel,
		zones:         zone.New(),
		store:         store,
		dbPath:        options.DBPath,
		configPath:    options.ConfigPath,
		llmClient:     client,
		chatCfg:       chatCfg,
		filePickerDir: options.FilePickerDir,
		ex: extractState{
			extractionProvider: options.ExtractionConfig.Provider,
			extractionBaseURL:  options.ExtractionConfig.BaseURL,
			extractionModel:    options.ExtractionConfig.Model,
			extractionAPIKey:   options.ExtractionConfig.APIKey,
			extractionTimeout:  options.ExtractionConfig.Timeout,
			extractionThinking: options.ExtractionConfig.Thinking,
			extractionEnabled:  options.ExtractionConfig.Enabled,
			ocrTSV:             options.ExtractionConfig.OCRTSV,
			ocrConfThreshold:   options.ExtractionConfig.OCRConfThreshold,
			extractors:         options.ExtractionConfig.Extractors,
		},
		pull:            pullState{progress: pprog},
		addressClient:   &http.Client{},
		addressBaseURL:  postalCodeAPIBaseURL,
		addressCountry:  options.AddressCountry,
		addressAutofill: options.AddressAutofill,
		styles:          appStyles,
		tabs:            NewTabs(),
		active:          0,
		showHouse:       false,
		mode:            modeNormal,
		cur:             store.Currency(),
		syncCfg:         options.syncCfg,
	}

	if cfg := options.syncCfg; cfg != nil {
		syncClient := sync.NewClient(cfg.relayURL, cfg.token, cfg.key)
		model.syncEngine = sync.NewEngine(store, syncClient, cfg.householdID)
		model.syncCtx, model.syncCancel = context.WithCancel(model.appCtx)
		model.syncStatus = syncSyncing
	}
	// Best-effort: fall back to locale detection if setting unreadable.
	model.unitSystem, _ = store.GetUnitSystem()
	if err := model.loadLookups(); err != nil {
		return nil, err
	}
	if err := model.loadHouse(); err != nil {
		return nil, err
	}
	if err := model.reloadAllTabs(); err != nil {
		return nil, err
	}
	if !model.hasHouse {
		model.startHouseForm()
	} else {
		// Best-effort: default to dashboard hidden if setting unreadable.
		show, _ := store.GetShowDashboard()
		model.showDashboard = show
		if show {
			// Best-effort: start without dashboard on load failure.
			_ = model.loadDashboard()
		}
	}
	appCancel = nil // prevent deferred cleanup; Model now owns the context
	return model, nil
}

// lifecycleCtx returns the app lifecycle context. Falls back to
// context.Background() when appCtx is nil (e.g., in tests that
// construct Model directly without NewModel).
func (m *Model) lifecycleCtx() context.Context {
	if m.appCtx != nil {
		return m.appCtx
	}
	return context.Background()
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.formInitCmd(), tea.RequestBackgroundColor}
	if m.syncEngine != nil {
		m.syncStatus = syncSyncing
		cmds = append(cmds, doSync(m.syncCtx, m.syncEngine), syncTick())
	}
	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevGen := m.syncDebounceGen
	model, cmd := m.update(msg)
	if m.syncEngine != nil && m.syncDebounceGen != prevGen {
		cmd = tea.Batch(cmd, syncDebounce(m.syncDebounceGen))
	}
	return model, cmd
}

func (m *Model) View() tea.View {
	v := tea.NewView(m.zones.Scan(m.buildView()))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) enterNormalMode() {
	m.mode = modeNormal
	m.setAllTableKeyMaps(normalTableKeyMap())
}

func (m *Model) enterEditMode() {
	m.mode = modeEdit
	m.setAllTableKeyMaps(editTableKeyMap())
}

// reloadAll refreshes lookups, house profile, all tabs, detail tab, and
// dashboard data. Used only at initialization; mutations should call
// reloadAfterMutation for targeted reload.
func (m *Model) reloadAll() {
	if m.store == nil {
		return
	}
	m.surfaceError(m.loadLookups())
	m.surfaceError(m.loadHouse())
	m.surfaceError(m.reloadAllTabs())
	if m.inDetail() {
		m.surfaceError(m.reloadDetailTab())
	}
	if m.showDashboard {
		m.surfaceError(m.loadDashboard())
	}
}

// reloadAfterMutation refreshes only the tab the user is looking at and
// marks all other tabs as stale for lazy reload on navigation. Dashboard
// is refreshed only when visible. This avoids reloading 4 idle tabs on
// every save.
func (m *Model) reloadAfterMutation() {
	if m.store == nil {
		return
	}
	if dc := m.detail(); dc != nil {
		dc.Mutated = true
	}
	m.surfaceError(m.reloadEffectiveTab())
	m.markNonEffectiveStale()
	if m.showDashboard {
		m.surfaceError(m.loadDashboard())
	}
	if m.syncEngine != nil {
		m.syncDebounceGen++
	}
}

// markNonEffectiveStale marks all tabs except the effective (active or
// detail-parent) tab as needing a reload on next navigation.
func (m *Model) markNonEffectiveStale() {
	effectiveIdx := m.active
	for i := range m.tabs {
		if i != effectiveIdx {
			m.tabs[i].Stale = true
		}
	}
}

// reloadIfStale reloads a tab only if it is marked stale. The stale flag
// is cleared by reloadTab on success.
func (m *Model) reloadIfStale(tab *Tab) error {
	if tab == nil || !tab.Stale {
		return nil
	}
	return m.reloadTab(tab)
}

// dashboardVisible reports whether the dashboard overlay should actually
// render. The preference may be on but there's nothing to show.
func (m *Model) dashboardVisible() bool {
	return m.showDashboard && !m.dash.data.empty()
}

func (m *Model) toggleUnitSystem() {
	if m.unitSystem == data.UnitsImperial {
		m.unitSystem = data.UnitsMetric
	} else {
		m.unitSystem = data.UnitsImperial
	}
	m.setStatusInfo("units: " + m.unitSystem.String())
	if m.store != nil {
		m.surfaceError(m.store.PutUnitSystem(m.unitSystem))
	}
}

func (m *Model) toggleDashboard() {
	m.showDashboard = !m.showDashboard
	if m.showDashboard {
		m.surfaceError(m.loadDashboard())
	}
	if m.store != nil {
		m.surfaceError(m.store.PutShowDashboard(m.showDashboard))
	}
}

// navigateToLink closes any open drilldown stack, switches to the target tab,
// and selects the row matching the FK.
func (m *Model) navigateToLink(link *columnLink, targetID string) error {
	m.closeAllDetails()
	m.switchToTab(tabIndex(link.TargetTab))
	tab := m.activeTab()
	if tab == nil {
		return fmt.Errorf("target tab not found")
	}
	if !selectRowByID(tab, targetID) {
		m.setStatusError(fmt.Sprintf("Linked item %s not found (deleted?).", targetID))
	}
	return nil
}

// selectedCell returns the cell at the given column for the currently selected row.
func (m *Model) selectedCell(col int) (cell, bool) {
	tab := m.effectiveTab()
	if tab == nil {
		return cell{}, false
	}
	cursor := tab.Table.Cursor()
	if cursor < 0 || cursor >= len(tab.CellRows) {
		return cell{}, false
	}
	row := tab.CellRows[cursor]
	if col < 0 || col >= len(row) {
		return cell{}, false
	}
	return row[col], true
}

func (m *Model) reloadEffectiveTab() error {
	if m.inDetail() {
		return m.reloadDetailTab()
	}
	return m.reloadActiveTab()
}

func (m *Model) toggleShowDeleted() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	tab.ShowDeleted = !tab.ShowDeleted
	tab.showDeletedExplicit = true
	if tab.ShowDeleted {
		m.setStatusInfo("Deleted shown.")
	} else {
		m.setStatusInfo("Deleted hidden.")
	}
	m.surfaceError(m.reloadEffectiveTab())
}

// activeProjectStatuses are the non-settled statuses that remain visible when
// the settled filter is toggled on via 't'.
var activeProjectStatuses = []string{
	data.ProjectStatusIdeating,
	data.ProjectStatusPlanned,
	data.ProjectStatusQuoted,
	data.ProjectStatusInProgress,
	data.ProjectStatusDelayed,
}

func (m *Model) toggleSettledFilter() bool {
	if m.inDetail() {
		return false
	}
	tab := m.activeTab()
	if tab == nil || tab.Kind != tabProjects {
		return false
	}
	col := statusColumnIndex(tab.Specs)
	if col < 0 {
		return false
	}
	if hasColumnPins(tab, col) {
		// Turn off: clear status column pins.
		clearPinsForColumn(tab, col)
		m.refreshTable(tab)
		m.setStatusInfo("Settled shown.")
	} else {
		// Turn on: pin all active (non-settled) statuses, activate filter.
		for _, status := range activeProjectStatuses {
			togglePin(tab, col, status)
		}
		tab.FilterActive = true
		m.refreshTable(tab)
		m.setStatusInfo("Settled hidden.")
	}
	return true
}

func (m *Model) selectedRowMeta() (rowMeta, bool) {
	tab := m.effectiveTab()
	if tab == nil || len(tab.Rows) == 0 {
		return rowMeta{}, false
	}
	cursor := tab.Table.Cursor()
	if cursor < 0 || cursor >= len(tab.Rows) {
		return rowMeta{}, false
	}
	return tab.Rows[cursor], true
}

func (m *Model) reloadActiveTab() error {
	if m.store == nil {
		return nil
	}
	tab := m.activeTab()
	if tab == nil {
		return nil
	}
	return m.reloadTab(tab)
}

func (m *Model) reloadAllTabs() error {
	if m.store == nil {
		return nil
	}
	for i := range m.tabs {
		if err := m.reloadTab(&m.tabs[i]); err != nil {
			return err
		}
	}
	return nil
}

func (m *Model) reloadTab(tab *Tab) error {
	rows, meta, cellRows, err := tab.Handler.Load(m.store, tab.ShowDeleted)
	if err != nil {
		return err
	}
	// Store the full data set for pin-and-filter to operate on without
	// re-querying the database.
	tab.FullRows = rows
	tab.FullMeta = meta
	tab.FullCellRows = cellRows
	tab.Stale = false
	m.refreshTable(tab)
	return nil
}

func (m *Model) loadHouse() error {
	profile, err := m.store.HouseProfile()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		m.hasHouse = false
		return nil
	}
	if err != nil {
		return err
	}
	m.house = profile
	m.hasHouse = true
	return nil
}

func (m *Model) loadLookups() error {
	var err error
	m.projectTypes, err = m.store.ProjectTypes()
	if err != nil {
		return err
	}
	m.maintenanceCategories, err = m.store.MaintenanceCategories()
	if err != nil {
		return err
	}
	m.vendors, err = m.store.ListVendors(false)
	if err != nil {
		return err
	}
	m.syncFixedValues()
	return nil
}

// syncFixedValues updates FixedValues on columns that reference dynamic lookup
// tables so columnWidths stays stable regardless of which values are displayed.
func (m *Model) syncFixedValues() {
	for i := range m.tabs {
		tab := &m.tabs[i]
		tab.Handler.SyncFixedValues(m, tab.Specs)
	}
}

func setFixedValues(specs []columnSpec, title string, values []string) {
	for i := range specs {
		if specs[i].Title == title {
			specs[i].FixedValues = values
			return
		}
	}
}

func (m *Model) resizeTables() {
	// Chrome: 1 blank after house + 1 tab/breadcrumb row + 1 underline = 3
	height := m.height - m.houseLines() - 3 - m.statusLines()
	if height < 4 {
		height = 4
	}
	tableHeight := height - 1
	if tableHeight < 2 {
		tableHeight = 2
	}
	for i := range m.tabs {
		m.tabs[i].cachedVP = nil
		m.tabs[i].Table.SetHeight(tableHeight)
		m.tabs[i].Table.SetWidth(m.width)
	}
	if dc := m.detail(); dc != nil {
		dc.Tab.cachedVP = nil
		dc.Tab.Table.SetHeight(tableHeight)
		dc.Tab.Table.SetWidth(m.width)
	}
}

func (m *Model) houseLines() int {
	return lipgloss.Height(m.houseView())
}

func (m *Model) statusLines() int {
	lines := 1
	if m.status.Text != "" {
		lines++
	}
	if m.pull.display != "" {
		lines++
	}
	return lines
}

// checkExtractionModelCmd returns a tea.Cmd that checks whether the extraction
// model is available. For local servers it verifies availability and initiates
// a pull if missing; for cloud providers it trusts the config.
func (m *Model) checkExtractionModelCmd() tea.Cmd {
	if !m.ex.extractionEnabled {
		return nil
	}

	client := m.extractionLLMClient()
	if client == nil {
		return nil
	}

	model := client.Model()
	if model == "" {
		return nil
	}

	timeout := client.Timeout()

	// Cloud providers that don't support model listing: trust the config.
	if !client.SupportsModelListing() {
		return func() tea.Msg {
			return pullProgressMsg{Done: true, Model: model}
		}
	}

	appCtx := m.lifecycleCtx()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(appCtx, timeout)
		defer cancel()
		models, err := client.ListModels(ctx)
		if err != nil {
			return pullProgressMsg{
				Err:   fmt.Errorf("check extraction model: %w", err),
				Done:  true,
				Model: model,
			}
		}
		for _, m := range models {
			if m == model || strings.HasPrefix(m, model+":") {
				return pullProgressMsg{
					Done:  true,
					Model: m,
				}
			}
		}
		return startPull(appCtx, client.BaseURL(), model)
	}
}

// handlePullProgress processes model pull progress for both chat-initiated
// and extraction-initiated pulls. Progress is shown in the status bar;
// completion actions depend on who started the pull.
func (m *Model) handlePullProgress(msg pullProgressMsg) tea.Cmd {
	if msg.Err != nil {
		fromChat := m.pull.fromChat
		m.clearPullState()
		if fromChat && m.chat != nil {
			m.chat.Messages = append(m.chat.Messages, chatMessage{
				Role:    roleError,
				Content: fmt.Sprintf("pull failed for '%s': %s", msg.Model, msg.Err),
			})
			m.refreshChatViewport()
		} else {
			m.setStatusError(fmt.Sprintf("model pull: %s", msg.Err))
		}
		m.resizeTables()
		return nil
	}
	if msg.Done {
		fromChat := m.pull.fromChat
		m.clearPullState()
		m.status = statusMsg{}
		// Mark extraction model as ready if it matches.
		exClient := m.extractionLLMClient()
		exModel := ""
		if exClient != nil {
			exModel = exClient.Model()
		}
		if msg.Model != "" && (msg.Model == exModel || strings.HasPrefix(msg.Model, exModel+":")) {
			m.ex.extractionReady = true
		}
		// Chat-initiated pulls switch the active model.
		if fromChat {
			if msg.Model != "" {
				m.llmClient.SetModel(msg.Model)
				// Best-effort: re-detected on next startup if persist fails.
				_ = m.store.PutLastModel(msg.Model)
			}
			if m.chat != nil {
				m.chat.Messages = append(m.chat.Messages, chatMessage{
					Role: roleNotice, Content: msg.Status,
				})
				m.refreshChatViewport()
			}
		}
		m.resizeTables()
		// Extract hints for the pending document now that the model is available.
		if m.ex.extractionReady && m.ex.pendingExtractionDocID != nil {
			docID := *m.ex.pendingExtractionDocID
			m.ex.pendingExtractionDocID = nil
			doc, err := m.store.GetDocument(docID)
			if err != nil {
				m.setStatusError("load document for extraction: " + err.Error())
			} else {
				return m.startExtractionOverlay(
					docID, doc.FileName, doc.Data, doc.MIMEType, doc.ExtractedText, doc.ExtractData,
				)
			}
		}
		// Auto-rerun extraction if the overlay is open and waiting for a
		// model that just finished pulling.
		if m.ex.extractionReady && m.ex.extraction != nil && m.ex.extraction.Done && !fromChat {
			return m.rerunLLMExtraction()
		}
		return nil
	}

	m.pull.active = true
	if msg.PullState != nil {
		m.pull.cancel = msg.PullState.Cancel
	}
	m.pull.display = m.formatPullProgress(msg)
	m.resizeTables()

	ps := msg.PullState
	return func() tea.Msg {
		return readNextPullChunk(ps)
	}
}

// formatPullProgress builds a compact single-line progress string.
func (m *Model) formatPullProgress(msg pullProgressMsg) string {
	label := cleanPullStatus(msg.Status, msg.Model)

	if msg.Percent < 0 {
		return label
	}
	pct := msg.Percent
	if pct > m.pull.peak {
		m.pull.peak = pct
	} else {
		pct = m.pull.peak
	}
	barW := m.width/3 - lipgloss.Width(label) - 2
	if barW < 15 {
		barW = 15
	}
	m.pull.progress.SetWidth(barW)
	return label + " " + m.pull.progress.ViewAs(pct)
}

// extractionLLMClient returns the LLM client configured for extraction,
// or nil if extraction is not available. The client is created once and
// cached. Each pipeline is fully independent -- no fallback to chat config.
func (m *Model) extractionLLMClient() *llm.Client {
	if m.ex.extractionClient != nil {
		return m.ex.extractionClient
	}

	provider := m.ex.extractionProvider
	baseURL := m.ex.extractionBaseURL
	apiKey := m.ex.extractionAPIKey
	timeout := m.ex.extractionTimeout
	model := m.ex.extractionModel

	if model == "" {
		return nil
	}

	c, err := llm.NewClient(provider, baseURL, model, apiKey, timeout)
	if err != nil {
		return nil
	}
	if m.ex.extractionThinking != "" {
		c.SetThinking(m.ex.extractionThinking)
	}
	m.ex.extractionClient = c
	return c
}

// afterDocumentSave returns a tea.Cmd to run async extraction (text and/or
// LLM) on the just-saved document. Opens the extraction overlay if any async
// steps are needed. If the LLM model isn't ready yet, it queues the doc for
// extraction after the model pull completes.
func (m *Model) afterDocumentSave() tea.Cmd {
	if m.fs.editID == nil {
		return nil
	}
	docID := *m.fs.editID

	// Load metadata (no BLOB) to decide whether extraction is needed.
	meta, err := m.store.GetDocumentMetadata(docID)
	if err != nil {
		m.setStatusError("load document for extraction: " + err.Error())
		return nil
	}

	// Check if LLM extraction is configured and ready.
	llmReady := m.ex.extractionEnabled && m.extractionLLMClient() != nil && m.ex.extractionReady

	// Determine if async extraction is needed. Skip OCR when the
	// document already has extracted text from a previous run.
	needsExtract := extract.NeedsOCR(m.ex.extractors, meta.MIMEType)
	if strings.TrimSpace(meta.ExtractedText) != "" {
		needsExtract = false
	}

	// If nothing async is needed, bail early.
	if !needsExtract && !llmReady {
		// If LLM is configured but model not ready, queue for after pull.
		if m.ex.extractionEnabled && m.extractionLLMClient() != nil && !m.ex.extractionReady {
			m.ex.pendingExtractionDocID = &docID
			if !m.pull.active {
				m.setStatusInfo("checking extraction model" + symEllipsis)
				return m.checkExtractionModelCmd()
			}
		}
		return nil
	}

	// Extraction needed -- load the full document with BLOB data.
	doc, err := m.store.GetDocument(docID)
	if err != nil {
		m.setStatusError("load document for extraction: " + err.Error())
		return nil
	}

	return m.startExtractionOverlay(
		docID,
		doc.FileName,
		doc.Data,
		doc.MIMEType,
		doc.ExtractedText,
		doc.ExtractData,
	)
}

// cancelPull cancels any in-flight model pull.
func (m *Model) cancelPull() {
	if m.pull.cancel != nil {
		m.pull.cancel()
	}
	m.clearPullState()
}

func (m *Model) clearPullState() {
	prog := m.pull.progress // preserve the progress bar widget
	m.pull = pullState{progress: prog}
}

func (m *Model) saveForm() tea.Cmd {
	// Deferred document creation: hold doc in memory, open extraction overlay.
	if fd, ok := m.fs.formData.(*documentFormData); ok && fd.DeferCreate {
		return m.saveDeferredDocumentForm()
	}

	isFirstHouse := m.fs.formKind() == formHouse && !m.hasHouse
	kind := m.fs.formKind()
	err := m.handleFormSubmit()
	if err != nil {
		m.setStatusError(err.Error())
		return nil
	}
	// Reload before exitForm so the new/updated row is in the table
	// when exitForm positions the cursor.
	m.reloadAfterFormSave(kind)
	cmd := m.afterDocumentSaveIfNeeded(kind)
	m.exitForm()
	if isFirstHouse {
		m.setStatusInfo("House set up. Press b/f to switch tabs, i to edit, ? for help.")
	}
	return cmd
}

// saveFormInPlace persists the form data without closing the form,
// so the user can continue editing after a Ctrl+S save.
//
// Ctrl+S is a quiet save: it never triggers the extraction pipeline.
// Extraction only runs on form completion (Enter) via saveForm.
func (m *Model) saveFormInPlace() tea.Cmd {
	// Quick-add documents: create the document directly without
	// triggering extraction. Use Enter for the deferred extraction flow.
	if fd, ok := m.fs.formData.(*documentFormData); ok && fd.DeferCreate {
		return m.saveQuickDocumentDirect()
	}
	kind := m.fs.formKind()
	isCreate := m.fs.editID == nil
	err := m.handleFormSubmit()
	if err != nil {
		m.setStatusError(err.Error())
		return nil
	}
	m.setStatusSaved()
	m.snapshotForm()
	m.reloadAfterFormSave(kind)
	// After a create, position the cursor on the new row so that
	// subsequent Esc leaves the user on the item they just created.
	if isCreate && m.fs.editID != nil {
		if tab := m.effectiveTab(); tab != nil {
			selectRowByID(tab, *m.fs.editID)
		}
	}
	return nil
}

// saveQuickDocumentDirect creates a document from the quick-add form
// without opening the extraction overlay. Called by ctrl+s; the Enter
// path (saveForm -> saveDeferredDocumentForm) handles deferred extraction.
func (m *Model) saveQuickDocumentDirect() tea.Cmd {
	result, err := m.parseDocumentFormData()
	if err != nil {
		m.setStatusError(err.Error())
		return nil
	}
	doc := result.Doc
	if err := m.store.CreateDocument(&doc); err != nil {
		m.setStatusError(err.Error())
		return nil
	}
	m.reloadAfterMutation()
	m.exitForm()
	if result.ExtractErr != nil {
		m.setStatusInfo(fmt.Sprintf("extraction incomplete: %s", result.ExtractErr))
	}
	return nil
}

// afterDocumentSaveIfNeeded triggers async LLM extraction for document forms.
func (m *Model) afterDocumentSaveIfNeeded(kind FormKind) tea.Cmd {
	if kind != formDocument || m.fs.notesEditMode {
		return nil
	}
	return m.afterDocumentSave()
}

// saveDeferredDocumentForm parses the quick-add form, holds the document in
// memory, and opens the extraction overlay. The document is not created in the
// database until the user accepts the extraction results.
func (m *Model) saveDeferredDocumentForm() tea.Cmd {
	result, err := m.parseDocumentFormData()
	if err != nil {
		m.setStatusError(err.Error())
		return nil
	}
	doc := result.Doc
	m.exitForm()

	cmd := m.startExtractionOverlay(
		"", // no DB ID yet
		doc.FileName,
		doc.Data,
		doc.MIMEType,
		doc.ExtractedText,
		doc.ExtractData,
	)
	if cmd == nil {
		// No extraction steps needed. Create the document immediately.
		if err := m.store.CreateDocument(&doc); err != nil {
			m.setStatusError(err.Error())
			return nil
		}
		m.reloadAfterMutation()
		return nil
	}
	m.ex.extraction.pendingDoc = &doc
	if result.ExtractErr != nil {
		m.setStatusInfo(fmt.Sprintf("extraction incomplete: %s", result.ExtractErr))
	}
	return cmd
}

// reloadAfterFormSave picks the minimal reload strategy based on which
// form was just saved. House and vendor mutations need broader refreshes;
// everything else uses the targeted reload.
func (m *Model) reloadAfterFormSave(kind FormKind) {
	if m.store == nil {
		return
	}
	switch kind {
	case formHouse:
		m.surfaceError(m.loadHouse())
		m.reloadAfterMutation()
	case formVendor:
		m.surfaceError(m.loadLookups())
		m.reloadAfterMutation()
	default:
		m.reloadAfterMutation()
	}
}

func (m *Model) snapshotForm() {
	m.fs.formSnapshot = cloneFormData(m.fs.formData)
	m.fs.formDirty = false
}

func (m *Model) checkFormDirty() {
	m.fs.formDirty = !reflect.DeepEqual(m.fs.formData, m.fs.formSnapshot)
}

// cloneFormData makes a shallow copy of the struct behind a form-data
// pointer so the snapshot is independent of later mutations. All form
// data types are pointer-to-struct with only value-type fields.
func cloneFormData(d formData) formData {
	if d == nil {
		return nil
	}
	v := reflect.ValueOf(d)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		cp := reflect.New(v.Elem().Type())
		cp.Elem().Set(v.Elem())
		cloned, ok := cp.Interface().(formData)
		if !ok {
			return d
		}
		return cloned
	}
	return d
}

// openHelp creates a viewport sized to fit the terminal and populated with
// the help content. The viewport handles scroll state and key delegation.
func (m *Model) openHelp() {
	content := m.helpContent()
	lines := strings.Split(content, "\n")

	// Chrome: border (2) + padding (2) + gap + rule + hint (4) = 8 lines.
	maxH := m.effectiveHeight() - 2
	if maxH < 10 {
		maxH = 10
	}
	viewH := maxH - 8
	if viewH < 3 {
		viewH = 3
	}
	// If content fits, no scrolling needed.
	if len(lines) <= viewH {
		viewH = len(lines)
	}

	// Lock width to the widest content line so the overlay never resizes.
	maxW := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxW {
			maxW = w
		}
	}

	vp := viewport.New(viewport.WithWidth(maxW), viewport.WithHeight(viewH))
	vp.SetContent(content)
	// Disable horizontal scroll to avoid conflicts with table navigation.
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)
	m.helpViewport = &vp
}

func (m *Model) handleInlineInputKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case keyEsc:
		m.closeInlineInput()
		return nil
	case keyEnter:
		m.submitInlineInput()
		return nil
	}
	var cmd tea.Cmd
	m.inlineInput.Input, cmd = m.inlineInput.Input.Update(key)
	return cmd
}

func (m *Model) submitInlineInput() {
	ii := m.inlineInput
	value := ii.Input.Value()
	if ii.Validate != nil {
		if err := ii.Validate(value); err != nil {
			m.setStatusError(err.Error())
			return
		}
	}
	*ii.FieldPtr = value
	kind := ii.FormData.formKind()
	if err := m.handleFormSubmit(); err != nil {
		m.setStatusError(err.Error())
		return
	}
	m.closeInlineInput()
	m.setStatusSaved()
	m.reloadAfterFormSave(kind)
}

// resetFormState zeroes all form-related fields. Every path that exits a
// form, inline edit, or calendar overlay should call this to prevent drift.
func (m *Model) resetFormState() {
	m.fs.form = nil
	m.fs.formData = nil
	m.fs.formSnapshot = nil
	m.fs.formDirty = false
	m.fs.pendingFormInit = nil
	m.fs.editID = nil
	m.fs.notesEditMode = false
	m.fs.notesFieldPtr = nil
	m.fs.postalCodeField = nil
	m.fs.cityInput = nil
	m.fs.stateInput = nil
	m.fs.lastPostalCode = ""
	m.fs.autoFilledCity = ""
	m.fs.autoFilledState = ""
	if m.confirm.isFormConfirm() {
		m.confirm = confirmNone
	}
}

func (m *Model) closeInlineInput() {
	m.inlineInput = nil
	m.resetFormState()
}

// exitForm closes the form and restores the previous mode. If editID is
// set (item was saved at least once), the cursor moves to that row so the
// user lands on the item they were just editing/creating.
func (m *Model) exitForm() {
	savedID := m.fs.editID
	m.mode = m.prevMode
	// Restore correct table key bindings for the returning mode.
	if m.mode == modeEdit {
		m.setAllTableKeyMaps(editTableKeyMap())
	} else {
		m.setAllTableKeyMaps(normalTableKeyMap())
	}
	m.resetFormState()
	if savedID != nil {
		if tab := m.effectiveTab(); tab != nil {
			selectRowByID(tab, *savedID)
		}
	}
	if m.syncPendingReload {
		m.syncPendingReload = false
		m.surfaceError(m.reloadAllTabs())
	}
}

const (
	defaultWidth    = 80
	defaultHeight   = 24
	minUsableWidth  = 80
	minUsableHeight = 24
)

func (m *Model) effectiveWidth() int {
	if m.width > 0 {
		return m.width
	}
	return defaultWidth
}

func (m *Model) effectiveHeight() int {
	if m.height > 0 {
		return m.height
	}
	return defaultHeight
}

// overlayContentWidth returns the clamped content width for overlay boxes
// (dashboard, note preview). Accounts for border (2), padding (4), and
// breathing room (6) = 12 total, clamped to [30, 72].
func (m *Model) overlayContentWidth() int {
	w := m.effectiveWidth() - 12
	if w > 72 {
		w = 72
	}
	if w < 30 {
		w = 30
	}
	return w
}

// overlayMaxHeight returns the clamped maximum height for overlay boxes.
func (m *Model) overlayMaxHeight() int {
	h := m.effectiveHeight() - 4
	if h < 10 {
		h = 10
	}
	return h
}

// scrollRule renders a horizontal rule with an embedded Vim-style scroll
// indicator (Top/Bot/N%) when content overflows the viewport.
func (m *Model) scrollRule(
	width, totalLines, viewportH int,
	atTop, atBottom bool,
	pct float64,
	ch string,
) string {
	if totalLines <= viewportH {
		return m.styles.Rule().Render(strings.Repeat(ch, width))
	}
	var label string
	switch {
	case atTop:
		label = "Top"
	case atBottom:
		label = "Bot"
	default:
		label = fmt.Sprintf("%d%%", int(pct*100))
	}
	indicator := m.styles.TextDim().Render(" " + label + " ")
	indicatorW := lipgloss.Width(indicator)
	rightW := max(0, width-indicatorW)
	return m.styles.Rule().Render(strings.Repeat(ch, rightW)) + indicator
}

func (m *Model) terminalTooSmall() bool {
	return m.effectiveWidth() < minUsableWidth || m.effectiveHeight() < minUsableHeight
}

// overlay unifies dispatch for UI surfaces that capture keyboard input when
// visible (help, calendar, chat, etc.). Rendering stays in view.go; this
// interface only standardises visibility checks and key handling.
type overlay interface {
	// isVisible reports whether the overlay is currently shown.
	isVisible() bool
	// handleKey processes a key press while the overlay is active.
	handleKey(key tea.KeyPressMsg) tea.Cmd
	// hidesMainKeys reports whether this overlay should suppress the main
	// tab keybindings from the status bar. Lightweight overlays like chat
	// and inline input return false.
	hidesMainKeys() bool
}

type helpOverlay struct{ m *Model }

func (o helpOverlay) isVisible() bool                       { return o.m.helpViewport != nil }
func (o helpOverlay) handleKey(key tea.KeyPressMsg) tea.Cmd { return o.m.helpOverlayKey(key) }
func (o helpOverlay) hidesMainKeys() bool                   { return true }

type extractionOverlay struct{ m *Model }

func (o extractionOverlay) isVisible() bool {
	return o.m.ex.extraction != nil && o.m.ex.extraction.Visible
}

func (o extractionOverlay) handleKey(key tea.KeyPressMsg) tea.Cmd {
	return o.m.handleExtractionKey(key)
}
func (o extractionOverlay) hidesMainKeys() bool { return true }

type chatOverlay struct{ m *Model }

func (o chatOverlay) isVisible() bool                       { return o.m.chat != nil && o.m.chat.Visible }
func (o chatOverlay) handleKey(key tea.KeyPressMsg) tea.Cmd { return o.m.handleChatKey(key) }
func (o chatOverlay) hidesMainKeys() bool                   { return false }

type notePreviewOverlay struct{ m *Model }

func (o notePreviewOverlay) isVisible() bool { return o.m.notePreview != nil }
func (o notePreviewOverlay) handleKey(tea.KeyPressMsg) tea.Cmd {
	o.m.notePreview = nil
	return nil
}
func (o notePreviewOverlay) hidesMainKeys() bool { return true }

type opsTreeOverlay struct{ m *Model }

func (o opsTreeOverlay) isVisible() bool                       { return o.m.opsTree != nil }
func (o opsTreeOverlay) handleKey(key tea.KeyPressMsg) tea.Cmd { return o.m.handleOpsTreeKey(key) }
func (o opsTreeOverlay) hidesMainKeys() bool                   { return true }

type calendarOverlay struct{ m *Model }

func (o calendarOverlay) isVisible() bool { return o.m.calendar != nil }

func (o calendarOverlay) handleKey(
	key tea.KeyPressMsg,
) tea.Cmd {
	return o.m.handleCalendarKey(key)
}
func (o calendarOverlay) hidesMainKeys() bool { return true }

type columnFinderOverlay struct{ m *Model }

func (o columnFinderOverlay) isVisible() bool { return o.m.columnFinder != nil }
func (o columnFinderOverlay) handleKey(key tea.KeyPressMsg) tea.Cmd {
	return o.m.handleColumnFinderKey(key)
}
func (o columnFinderOverlay) hidesMainKeys() bool { return true }

type docSearchOverlay struct{ m *Model }

func (o docSearchOverlay) isVisible() bool { return o.m.docSearch != nil }

func (o docSearchOverlay) handleKey(
	key tea.KeyPressMsg,
) tea.Cmd {
	return o.m.handleDocSearchKey(key)
}
func (o docSearchOverlay) hidesMainKeys() bool { return true }

type inlineInputOverlay struct{ m *Model }

func (o inlineInputOverlay) isVisible() bool { return o.m.inlineInput != nil }
func (o inlineInputOverlay) handleKey(key tea.KeyPressMsg) tea.Cmd {
	return o.m.handleInlineInputKey(key)
}
func (o inlineInputOverlay) hidesMainKeys() bool { return false }

// overlays returns all overlays in priority order (highest priority first).
// The dashboard is not included — it has special pre-handler nav interception
// in update() that doesn't fit this interface.
func (m *Model) overlays() []overlay {
	return []overlay{
		helpOverlay{m},
		extractionOverlay{m},
		chatOverlay{m},
		notePreviewOverlay{m},
		opsTreeOverlay{m},
		calendarOverlay{m},
		columnFinderOverlay{m},
		docSearchOverlay{m},
		inlineInputOverlay{m},
	}
}

// hasActiveOverlay returns true when any overlay is currently shown that hides
// main tab keybindings from the status bar. The dashboard is checked
// separately because it has its own dispatch path.
func (m *Model) hasActiveOverlay() bool {
	if m.dashboardVisible() {
		return true
	}
	for _, o := range m.overlays() {
		if o.isVisible() && o.hidesMainKeys() {
			return true
		}
	}
	return false
}

func (m *Model) dispatchOverlay(msg tea.Msg) (tea.Cmd, bool) {
	for _, o := range m.overlays() {
		if !o.isVisible() {
			continue
		}
		keyMsg, ok := msg.(tea.KeyPressMsg)
		if !ok {
			// Non-key messages (cursor blink, spinner ticks, etc.) should not
			// be swallowed by the overlay dispatcher. Return false so the
			// caller's normal Update path can handle them.
			return nil, false
		}
		return o.handleKey(keyMsg), true
	}
	return nil, false
}

func (m *Model) helpOverlayKey(keyMsg tea.KeyPressMsg) tea.Cmd {
	switch {
	case keyMsg.String() == keyEsc || keyMsg.String() == keyQuestion:
		m.helpViewport = nil
	case key.Matches(keyMsg, helpGotoTop):
		m.helpViewport.GotoTop()
	case key.Matches(keyMsg, helpGotoBottom):
		m.helpViewport.GotoBottom()
	default:
		vp, _ := m.helpViewport.Update(keyMsg)
		m.helpViewport = &vp
	}
	return nil
}

func selectRowByID(tab *Tab, id string) bool {
	for idx, meta := range tab.Rows {
		if meta.ID == id {
			tab.Table.SetCursor(idx)
			return true
		}
	}
	return false
}

// nextVisibleCol returns the next visible column index from current, clamping
// at boundaries. If forward is true it searches right; otherwise left. Returns
// current if already at the edge or no other visible columns exist.
func nextVisibleCol(specs []columnSpec, current int, forward bool) int {
	n := len(specs)
	if n == 0 {
		return 0
	}
	step := 1
	if !forward {
		step = -1
	}
	for i := current + step; i >= 0 && i < n; i += step {
		if specs[i].HideOrder == 0 {
			return i
		}
	}
	return current
}

// firstVisibleCol returns the index of the leftmost visible column.
func firstVisibleCol(specs []columnSpec) int {
	for i, s := range specs {
		if s.HideOrder == 0 {
			return i
		}
	}
	return 0
}

// lastVisibleCol returns the index of the rightmost visible column.
func lastVisibleCol(specs []columnSpec) int {
	for i := len(specs) - 1; i >= 0; i-- {
		if specs[i].HideOrder == 0 {
			return i
		}
	}
	return 0
}

// visibleCount returns the number of non-hidden columns.
func visibleCount(specs []columnSpec) int {
	count := 0
	for _, s := range specs {
		if s.HideOrder == 0 {
			count++
		}
	}
	return count
}

// nextHideOrder returns the next sequence number for hiding a column.
func nextHideOrder(specs []columnSpec) int {
	maxOrder := 0
	for _, s := range specs {
		if s.HideOrder > maxOrder {
			maxOrder = s.HideOrder
		}
	}
	return maxOrder + 1
}

func (m *Model) togglePinAtCursor() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	col := tab.ColCursor
	if col < 0 || col >= len(tab.Specs) {
		return
	}
	c, ok := m.selectedCell(col)
	if !ok {
		return
	}
	// Pin what the user sees: null cells use the sentinel key; in mag mode,
	// pin the magnitude representation; otherwise pin the raw value.
	pinValue := cellDisplayValue(c, m.magMode, m.cur.Symbol())
	pinned := togglePin(tab, col, pinValue)
	m.refreshTable(tab)
	if pinned {
		m.setStatusInfo("Pinned.")
	} else {
		m.setStatusInfo("Unpinned.")
	}
}

func (m *Model) toggleFilterActivation() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	// Toggle between active filter and preview. Works even with no pins
	// ("eager mode"): arm the filter first, then every n immediately filters.
	tab.FilterActive = !tab.FilterActive
	if hasPins(tab) {
		m.refreshTable(tab)
	}
}

func (m *Model) clearAllPins() {
	tab := m.effectiveTab()
	if tab == nil || !hasPins(tab) && !tab.FilterActive {
		return
	}
	clearPins(tab)
	m.refreshTable(tab)
	m.setStatusInfo("Pins cleared.")
}

func (m *Model) toggleFilterInvert() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	tab.FilterInverted = !tab.FilterInverted
	if hasPins(tab) {
		m.refreshTable(tab)
	}
}

func (m *Model) hideCurrentColumn() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	col := tab.ColCursor
	if col < 0 || col >= len(tab.Specs) {
		return
	}
	if tab.Specs[col].HideOrder > 0 {
		return
	}
	if visibleCount(tab.Specs) <= 1 {
		m.setStatusError("Cannot hide the last visible column.")
		return
	}
	tab.Specs[col].HideOrder = nextHideOrder(tab.Specs)
	// Clear any pins on the hidden column.
	clearPinsForColumn(tab, col)
	if hasPins(tab) {
		applyRowFilter(tab, m.magMode, m.cur.Symbol())
	}
	// Try forward first; if at the right edge fall back to backward.
	next := nextVisibleCol(tab.Specs, col, true)
	if next == col {
		next = nextVisibleCol(tab.Specs, col, false)
	}
	tab.ColCursor = next
	m.updateTabViewport(tab)
	m.setStatusInfo(
		fmt.Sprintf("Hidden: %s. Press C to show all.", tab.Specs[col].Title),
	)
}

func (m *Model) showAllColumns() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	changed := false
	for i := range tab.Specs {
		if tab.Specs[i].HideOrder > 0 {
			tab.Specs[i].HideOrder = 0
			changed = true
		}
	}
	if changed {
		m.updateTabViewport(tab)
		m.setStatusInfo("All columns visible.")
	}
}

func (m *Model) updateAllViewports() {
	if tab := m.activeTab(); tab != nil {
		m.updateTabViewport(tab)
	}
	if dc := m.detail(); dc != nil {
		m.updateTabViewport(&dc.Tab)
	}
}

// refreshTable reapplies row filters, sorts, and viewport layout for a tab.
// Use this after any change to pins, filter state, or row data.
func (m *Model) refreshTable(tab *Tab) {
	tab.cachedVP = nil
	applyRowFilter(tab, m.magMode, m.cur.Symbol())
	applySorts(tab)
	if tab.Table.Cursor() < 0 && len(tab.Rows) > 0 {
		tab.Table.SetCursor(0)
	}
	m.updateTabViewport(tab)
}

func (m *Model) updateTabViewport(tab *Tab) {
	if tab == nil {
		return
	}
	tab.cachedVP = nil
	visSpecs, visCells, visColCursor, _, _ := visibleProjection(tab)
	if len(visSpecs) == 0 || visColCursor < 0 {
		tab.ViewOffset = 0
		return
	}
	width := m.effectiveWidth()
	sepW := lipgloss.Width(" │ ")
	fullWidths := columnWidths(visSpecs, visCells, width, sepW, nil)
	ensureCursorVisible(tab, visColCursor, len(visSpecs))
	vpStart, _, _, _ := viewportRange(
		fullWidths, sepW, width, tab.ViewOffset, visColCursor,
	)
	tab.ViewOffset = vpStart
}

// tabViewport returns the cached tableViewport for the tab, computing and
// caching it if stale. The cache is populated during View() and reused by
// mouse handlers so they avoid O(rows*cols) recomputation per click.
func (m *Model) tabViewport(tab *Tab) tableViewport {
	if tab.cachedVP != nil {
		return *tab.cachedVP
	}
	normalSep := m.styles.TableSeparator().Render(" │ ")
	vp := computeTableViewport(tab, m.effectiveWidth(), normalSep, m.cur.Symbol())
	tab.cachedVP = &vp
	return vp
}

// tabIndex returns the position of the given TabKind in the canonical tab
// ordering defined by NewTabs. Derived from the actual slice at init time
// so adding a tab to NewTabs automatically keeps this in sync.
func tabIndex(kind TabKind) int {
	idx, ok := tabKindIndex[kind]
	if !ok {
		return 0
	}
	return idx
}

// tabKindIndex maps each TabKind to its position in the canonical tab slice.
// Populated once at init from NewTabs.
var tabKindIndex = func() map[TabKind]int {
	tabs := NewTabs()
	m := make(map[TabKind]int, len(tabs))
	for i, tab := range tabs {
		m[tab.Kind] = i
	}
	return m
}()

// editorBinary returns the user's preferred editor binary and any extra
// arguments parsed from $EDITOR or $VISUAL (e.g. "code --wait" yields
// binary="code", args=["--wait"]). On non-Windows platforms, the command
// is split using POSIX shell-style parsing to handle quoted paths; on
// Windows, config.SplitEditorCommand uses windows.DecomposeCommandLine to
// support quoted paths and spaces, with a plain-whitespace split only as a
// fallback if parsing fails. The binary is resolved via exec.LookPath to
// verify it exists and is executable.
func editorBinary() (string, []string, error) {
	raw := os.Getenv("EDITOR")
	if raw == "" {
		raw = os.Getenv("VISUAL")
	}
	if raw == "" {
		return "", nil, errors.New(
			"no editor configured: set $EDITOR or $VISUAL to an executable (e.g. export EDITOR=vim)",
		)
	}

	parts, err := config.SplitEditorCommand(raw)
	if err != nil {
		return "", nil, fmt.Errorf("parse editor command: %w", err)
	}
	if len(parts) == 0 {
		return "", nil, errors.New(
			"no editor configured: set $EDITOR or $VISUAL to an executable (e.g. export EDITOR=vim)",
		)
	}
	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return "", nil, fmt.Errorf(
			"editor %q not found on $PATH: install it or set $EDITOR to a valid executable: %w",
			parts[0], err,
		)
	}
	return bin, parts[1:], nil
}

// launchExternalEditor writes the current notes text to a temp file and
// launches $EDITOR via tea.ExecProcess. The textarea is closed so the
// terminal is fully released to the editor.
func (m *Model) launchExternalEditor() tea.Cmd {
	editor, editorArgs, err := editorBinary()
	if err != nil {
		m.setStatusError(err.Error())
		return nil
	}
	if m.fs.notesFieldPtr == nil {
		return nil
	}

	f, err := os.CreateTemp("", "micasa-notes-*.txt")
	if err != nil {
		m.setStatusError(fmt.Sprintf("create temp file: %s", err))
		return nil
	}
	if _, err := f.WriteString(*m.fs.notesFieldPtr); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name()) //nolint:gosec // temp file we just created
		m.setStatusError(fmt.Sprintf("write temp file: %s", err))
		return nil
	}
	_ = f.Close()

	m.fs.pendingEditor = &editorState{
		EditID:   "",
		FormData: m.fs.formData,
		FieldPtr: m.fs.notesFieldPtr,
		TempFile: f.Name(),
	}
	if m.fs.editID != nil {
		m.fs.pendingEditor.EditID = *m.fs.editID
	}

	m.exitForm()

	cmdArgs := make([]string, len(editorArgs)+1)
	copy(cmdArgs, editorArgs)
	cmdArgs[len(editorArgs)] = f.Name()
	cmd := exec.Command( //nolint:gosec,noctx // user-configured editor validated via LookPath
		editor,
		cmdArgs...,
	)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{Err: err}
	})
}

// handleEditorFinished reads the edited temp file, updates the field pointer,
// and reopens the textarea so the user can review before saving.
func (m *Model) handleEditorFinished(msg editorFinishedMsg) tea.Cmd {
	pe := m.fs.pendingEditor
	m.fs.pendingEditor = nil
	if pe == nil {
		return nil
	}
	defer func() { _ = os.Remove(pe.TempFile) }()

	if msg.Err != nil {
		m.setStatusError(fmt.Sprintf("editor: %s", msg.Err))
		// Reopen textarea with the original text so the user can retry.
		m.reopenNotesEdit(pe)
		return m.formInitCmd()
	}

	content, err := os.ReadFile(pe.TempFile)
	if err != nil {
		m.setStatusError(fmt.Sprintf("read temp file: %s", err))
		m.reopenNotesEdit(pe)
		return m.formInitCmd()
	}

	// Strip trailing newline that editors typically add.
	text := strings.TrimRight(string(content), "\n")
	*pe.FieldPtr = text

	m.reopenNotesEdit(pe)
	return m.formInitCmd()
}

// reopenNotesEdit restores the textarea overlay from a pending editor state.
func (m *Model) reopenNotesEdit(pe *editorState) {
	if pe.EditID != "" {
		id := pe.EditID
		m.fs.editID = &id
	}
	m.fs.formData = pe.FormData
	m.openNotesTextarea(pe.FieldPtr, pe.FormData)
}

// autoDetectModel checks if the LLM server has exactly one model available
// and returns it. Returns "" if the server is unreachable or has zero/multiple
// models (ambiguous cases where manual config is safer).
func autoDetectModel(ctx context.Context, client *llm.Client) string {
	ctx, cancel := context.WithTimeout(ctx, client.Timeout())
	defer cancel()

	models, err := client.ListModels(ctx)
	if err != nil || len(models) != 1 {
		return ""
	}
	return models[0]
}
