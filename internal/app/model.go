// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/cpcloud/micasa/internal/llm"
	"github.com/cpcloud/micasa/internal/locale"
	zone "github.com/lrstanley/bubblezone"
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
	llmConfig             *llmConfig // saved for extraction client creation
	llmExtraContext       string     // user-provided context appended to prompts
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
	calendar              *calendarState
	columnFinder          *columnFinderState
	dash                  dashState
	unitSystem            data.UnitSystem
	hasHouse              bool
	house                 data.HouseProfile
	mode                  Mode
	prevMode              Mode // mode to restore after form closes
	fs                    formState
	inlineInput           *inlineInputState
	magMode               bool // easter egg: display numbers as order-of-magnitude
	confirmHardDelete     bool // true while waiting for y/n on permanent delete
	hardDeleteID          uint // entity ID pending permanent deletion
	lastRowClick          rowClickState
	lastDashClick         rowClickState
	cur                   locale.Currency
	status                statusMsg
	projectTypes          []data.ProjectType
	maintenanceCategories []data.MaintenanceCategory
	vendors               []data.Vendor
}

func NewModel(store *data.Store, options Options) (*Model, error) {
	var client *llm.Client
	var extraContext string
	if options.LLMConfig != nil {
		model := options.LLMConfig.Model
		cfg := options.LLMConfig
		// Prefer the last-used model from the database if available.
		if persisted, err := store.GetLastModel(); err == nil && persisted != "" {
			model = persisted
		} else if cfg.Provider == "ollama" {
			// No persisted model -- try auto-detecting if the server has exactly one.
			tempClient, err := llm.NewClient(cfg.Provider, cfg.BaseURL, model, cfg.APIKey, cfg.Timeout)
			if err == nil {
				if detected := autoDetectModel(tempClient); detected != "" {
					model = detected
					_ = store.PutLastModel(model)
				}
			}
		}
		var err error
		client, err = llm.NewClient(cfg.Provider, cfg.BaseURL, model, cfg.APIKey, cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("create llm client: %w", err)
		}
		if cfg.Thinking != "" {
			client.SetThinking(cfg.Thinking)
		}
		extraContext = cfg.ExtraContext
	}

	pprog := progress.New(
		progress.WithGradient(textDim.Dark, accent.Dark),
		progress.WithFillCharacters('━', '┄'),
	)
	pprog.PercentageStyle = appStyles.TextDim()

	model := &Model{
		zones:           zone.New(),
		store:           store,
		dbPath:          options.DBPath,
		configPath:      options.ConfigPath,
		llmClient:       client,
		llmConfig:       options.LLMConfig,
		llmExtraContext: extraContext,
		ex: extractState{
			extractionProvider:  options.ExtractionConfig.Provider,
			extractionBaseURL:   options.ExtractionConfig.BaseURL,
			extractionModel:     options.ExtractionConfig.Model,
			extractionAPIKey:    options.ExtractionConfig.APIKey,
			extractionTimeout:   options.ExtractionConfig.Timeout,
			extractionThinking:  options.ExtractionConfig.Thinking,
			extractionEnabled:   options.ExtractionConfig.Enabled,
			extractors:          options.ExtractionConfig.Extractors,
			llmInferenceTimeout: options.ExtractionConfig.LLMInferenceTimeout,
		},
		pull:      pullState{progress: pprog},
		styles:    appStyles,
		tabs:      NewTabs(),
		active:    0,
		showHouse: false,
		mode:      modeNormal,
		cur:       store.Currency(),
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
	return model, nil
}

func (m *Model) Init() tea.Cmd {
	return m.formInitCmd()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.resizeTables()
		m.updateAllViewports()
	case tea.KeyMsg:
		if typed.String() == keyCtrlQ {
			if m.mode == modeForm && m.fs.formDirty {
				m.fs.confirmDiscard = true
				m.fs.confirmQuit = true
				return m, nil
			}
			m.cancelChatOperations()
			m.cancelAllExtractions()
			m.cancelPull()
			return m, tea.Quit
		}
		if typed.String() == keyCtrlC {
			// When the extraction overlay is open and running,
			// interrupt just that extraction instead of canceling
			// everything. The overlay stays visible so the user can
			// inspect partial results.
			if ex := m.ex.extraction; ex != nil && ex.Visible && !ex.Done {
				m.interruptExtraction()
				return m, nil
			}
			// Cancel any ongoing LLM operations but don't quit.
			m.cancelChatOperations()
			m.cancelAllExtractions()
			return m, nil
		}
	case chatChunkMsg:
		// Chunks arriving after chat is closed are harmlessly dropped.
		if m.chat != nil {
			return m, m.handleChatChunk(typed)
		}
		return m, nil
	case sqlStreamStartedMsg:
		if m.chat != nil {
			return m, m.handleSQLStreamStarted(typed)
		}
		return m, nil
	case sqlChunkMsg:
		if m.chat != nil {
			return m, m.handleSQLChunk(typed)
		}
		return m, nil
	case sqlResultMsg:
		if m.chat != nil {
			return m, m.handleSQLResult(typed)
		}
		return m, nil
	case pullProgressMsg:
		return m, m.handlePullProgress(typed)
	case extractionProgressMsg:
		return m, m.handleExtractionProgress(typed)
	case extractionLLMStartedMsg:
		return m, m.handleExtractionLLMStarted(typed)
	case extractionLLMChunkMsg:
		return m, m.handleExtractionLLMChunk(typed)
	case modelsListMsg:
		// Feed the extraction model picker first if it's waiting.
		if ex := m.ex.extraction; ex != nil && ex.modelPicker != nil && ex.modelPicker.Loading {
			ex.modelPicker.Loading = false
			if typed.Err == nil {
				ex.modelPicker.All = mergeModelLists(typed.Models)
			} else {
				ex.modelPicker.All = mergeModelLists(nil)
			}
			refilterModelCompleter(ex.modelPicker, ex.modelFilter, m.extractionModelLabel())
			return m, nil
		}
		if m.chat != nil {
			m.handleModelsListMsg(typed)
		}
		return m, nil
	case spinner.TickMsg:
		var cmds []tea.Cmd
		if m.chat != nil && m.chat.Streaming {
			var cmd tea.Cmd
			m.chat.Spinner, cmd = m.chat.Spinner.Update(msg)
			m.refreshChatViewport()
			cmds = append(cmds, cmd)
		}
		if m.ex.extraction != nil && !m.ex.extraction.Done {
			var cmd tea.Cmd
			m.ex.extraction.Spinner, cmd = m.ex.extraction.Spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		for _, bg := range m.ex.bgExtractions {
			if !bg.Done {
				var cmd tea.Cmd
				bg.Spinner, cmd = bg.Spinner.Update(msg)
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	case tea.MouseMsg:
		return m.handleMouse(typed)
	case openFileResultMsg:
		if typed.Err != nil {
			m.setStatusError(fmt.Sprintf("open: %s", typed.Err))
		}
		return m, nil
	case editorFinishedMsg:
		return m, m.handleEditorFinished(typed)
	}

	if cmd, handled := m.dispatchOverlay(msg); handled {
		return m, cmd
	}

	if m.mode == modeForm && m.fs.form != nil {
		return m.updateForm(msg)
	}

	switch typed := msg.(type) {
	case tea.KeyMsg:
		if m.confirmHardDelete {
			m.handleConfirmHardDelete(typed)
			return m, nil
		}
		// Dashboard intercepts nav keys before other handlers.
		if m.dashboardVisible() {
			if cmd, handled := m.handleDashboardKeys(typed); handled {
				return m, cmd
			}
		}
		if cmd, handled := m.handleCommonKeys(typed); handled {
			return m, cmd
		}
		if m.mode == modeNormal {
			if cmd, handled := m.handleNormalKeys(typed); handled {
				return m, cmd
			}
		}
		if m.mode == modeEdit {
			if cmd, handled := m.handleEditKeys(typed); handled {
				return m, cmd
			}
		}
	}

	// Dashboard absorbs remaining keys so they don't reach the table.
	if m.dashboardVisible() {
		return m, nil
	}

	// Pass unhandled messages to the effective table (handles j/k, g/G, etc.).
	tab := m.effectiveTab()
	if tab == nil {
		return m, nil
	}
	var cmd tea.Cmd
	tab.Table, cmd = tab.Table.Update(msg)
	return m, cmd
}

// updateForm handles input while a form is active.
func (m *Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle confirm-discard dialog: only y/n/esc are active.
	if m.fs.confirmDiscard {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			return m.handleConfirmDiscard(keyMsg)
		}
		return m, nil
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == keyCtrlS {
		return m, m.saveFormInPlace()
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == keyCtrlE && m.fs.notesEditMode {
		return m, m.launchExternalEditor()
	}
	// Block huh's deferred WindowSizeMsg from reaching the form. Without
	// this, Form.Init's tea.WindowSize() triggers height equalization that
	// pads shorter groups to the tallest group's height, pushing the "next"
	// indicator far below the last field. activateForm already sets the
	// correct width before Init, so this message is redundant.
	if _, isResize := msg.(tea.WindowSizeMsg); isResize {
		return m, nil
	}
	// Toggle hidden files in the filepicker with ".".
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == keyShiftH {
		if field := m.fs.form.GetFocusedField(); field != nil {
			if fp, ok := field.(*huh.FilePicker); ok {
				current := filePickerShowHidden(fp)
				newVal := !current
				fp.ShowHidden(newVal)
				// Reset cursor to top so it doesn't point past the new list.
				goToTop := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
				updated, _ := m.fs.form.Update(goToTop)
				if form, ok := updated.(*huh.Form); ok {
					m.fs.form = form
				}
				syncFilePickerTitle(m.fs.form)
				syncFilePickerDescription(m.fs.form)
				if newVal {
					m.setStatusInfo("Showing hidden files.")
				} else {
					m.setStatusInfo("Hiding hidden files.")
				}
				return m, fp.Init()
			}
		}
	}
	// Intercept 1-9 on Select fields to jump to the Nth option.
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if n, isOrdinal := selectOrdinal(keyMsg); isOrdinal && isSelectField(m.fs.form) {
			m.jumpSelectToOrdinal(n)
			return m, nil
		}
	}
	// Intercept ESC on dirty forms to confirm before discarding.
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == keyEsc {
		mandatoryHouse := m.fs.formKind == formHouse && !m.hasHouse
		if m.fs.formDirty && !mandatoryHouse {
			m.fs.confirmDiscard = true
			return m, nil
		}
	}
	updated, cmd := m.fs.form.Update(msg)
	form, ok := updated.(*huh.Form)
	if ok {
		m.fs.form = form
	}
	syncFilePickerTitle(m.fs.form)
	syncFilePickerDescription(m.fs.form)
	m.checkFormDirty()
	switch m.fs.form.State {
	case huh.StateCompleted:
		return m, m.saveForm()
	case huh.StateAborted:
		if m.fs.formKind == formHouse && !m.hasHouse {
			m.setStatusError("House profile required.")
			m.startHouseForm()
			return m, m.formInitCmd()
		}
		m.exitForm()
	default:
	}
	return m, cmd
}

// handleConfirmDiscard processes keys while the "discard unsaved changes?"
// prompt is active. Only y (discard) and n/esc (keep editing) are recognized.
func (m *Model) handleConfirmDiscard(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case keyY:
		m.fs.confirmDiscard = false
		if m.fs.confirmQuit {
			m.fs.confirmQuit = false
			m.cancelChatOperations()
			m.cancelPull()
			return m, tea.Quit
		}
		m.exitForm()
	case keyN, keyEsc:
		m.fs.confirmDiscard = false
		m.fs.confirmQuit = false
	}
	return m, nil
}

// handleDashboardKeys intercepts keys that belong to the dashboard (j/k
// navigation, enter to jump) and blocks keys that affect backgrounded
// widgets. Keys like D, b/f, ?, q fall through to the normal handlers.
func (m *Model) handleDashboardKeys(key tea.KeyMsg) (tea.Cmd, bool) {
	if key.String() != keyEnter {
		m.dash.flash = ""
	}
	switch key.String() {
	case keyJ, keyDown:
		m.dashDown()
		return nil, true
	case keyK, keyUp:
		m.dashUp()
		return nil, true
	case keyShiftJ, keyShiftDown:
		m.dashNextSection()
		return nil, true
	case keyShiftK, keyShiftUp:
		m.dashPrevSection()
		return nil, true
	case keyG:
		m.dashTop()
		return nil, true
	case keyShiftG:
		m.dashBottom()
		return nil, true
	case keyE:
		m.dashToggleCurrent()
		return nil, true
	case keyShiftE:
		m.dashToggleAll()
		return nil, true
	case keyEnter:
		m.dashJump()
		return nil, true
	case keyTab:
		// Block house profile toggle on dashboard.
		return nil, true
	case keyH, keyL, keyLeft, keyRight:
		// Block column movement on dashboard.
		return nil, true
	case keyS, keyShiftS, keyC, keyShiftC, keyI, keySlash, keyN, keyShiftN, keyBang:
		// Block table-specific keys on dashboard.
		return nil, true
	}
	return nil, false
}

// handleCommonKeys processes keys available in both Normal and Edit modes.
func (m *Model) handleCommonKeys(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case keyQuestion:
		m.openHelp()
		return nil, true
	case keyTab:
		m.showHouse = !m.showHouse
		m.resizeTables()
		return nil, true
	case keyCtrlO:
		m.magMode = !m.magMode
		if m.chat != nil && m.chat.Visible {
			m.refreshChatViewport()
		}
		// Translate pin values on ALL tabs (not just the active one)
		// so non-visible tabs don't retain stale pin formats.
		for i := range m.tabs {
			tab := &m.tabs[i]
			if !hasPins(tab) {
				continue
			}
			translatePins(tab, m.magMode, m.cur.Symbol())
			applyRowFilter(tab, m.magMode, m.cur.Symbol())
			applySorts(tab)
		}
		for _, dc := range m.detailStack {
			if hasPins(&dc.Tab) {
				translatePins(&dc.Tab, m.magMode, m.cur.Symbol())
				applyRowFilter(&dc.Tab, m.magMode, m.cur.Symbol())
				applySorts(&dc.Tab)
			}
		}
		if tab := m.effectiveTab(); tab != nil {
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyH, keyLeft:
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = nextVisibleCol(tab.Specs, tab.ColCursor, false)
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyL, keyRight:
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = nextVisibleCol(tab.Specs, tab.ColCursor, true)
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyCaret:
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = firstVisibleCol(tab.Specs)
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyDollar:
		if tab := m.effectiveTab(); tab != nil {
			tab.ColCursor = lastVisibleCol(tab.Specs)
			m.updateTabViewport(tab)
		}
		return nil, true
	case keyCtrlB:
		if len(m.ex.bgExtractions) > 0 {
			m.foregroundExtraction()
			return nil, true
		}
	}
	return nil, false
}

// handleNormalKeys processes keys unique to Normal mode.
func (m *Model) handleNormalKeys(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case keyShiftD:
		m.toggleDashboard()
		return nil, true
	case keyF:
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.nextTab()
		}
		return nil, true
	case keyB:
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.prevTab()
		}
		return nil, true
	case keyShiftF:
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.switchToTab(len(m.tabs) - 1)
		}
		return nil, true
	case keyShiftB:
		if !m.inDetail() {
			if m.showDashboard {
				m.showDashboard = false
			}
			m.switchToTab(0)
		}
		return nil, true
	case keyN:
		m.togglePinAtCursor()
		return nil, true
	case keyShiftN:
		m.toggleFilterActivation()
		return nil, true
	case keyCtrlN:
		m.clearAllPins()
		return nil, true
	case keyBang:
		m.toggleFilterInvert()
		return nil, true
	case keyS:
		if tab := m.effectiveTab(); tab != nil {
			toggleSort(tab, tab.ColCursor)
			applySorts(tab)
		}
		return nil, true
	case keyShiftS:
		if tab := m.effectiveTab(); tab != nil {
			clearSorts(tab)
			applySorts(tab)
		}
		return nil, true
	case keyShiftU:
		m.toggleUnitSystem()
		return nil, true
	case keyT:
		if m.toggleSettledFilter() {
			return nil, true
		}
	case keyC:
		m.hideCurrentColumn()
		return nil, true
	case keyShiftC:
		m.showAllColumns()
		return nil, true
	case keySlash:
		m.openColumnFinder()
		return nil, true
	case keyO:
		if cmd := m.openSelectedDocument(); cmd != nil {
			return cmd, true
		}
	case keyI:
		m.enterEditMode()
		return nil, true
	case keyEnter:
		if err := m.handleNormalEnter(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		if m.mode == modeForm {
			return m.formInitCmd(), true
		}
		return nil, true
	case keyAt:
		return m.openChat(), true
	case keyEsc:
		if m.inDetail() {
			m.closeDetail()
			return nil, true
		}
		m.status = statusMsg{}
		return nil, true
	}
	return nil, false
}

// handleNormalEnter handles enter in Normal mode: drill into detail views
// on drilldown columns, or follow FK links.
func (m *Model) handleNormalEnter() error {
	tab := m.effectiveTab()
	if tab == nil {
		return nil
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		return nil
	}

	col := tab.ColCursor
	if col < 0 || col >= len(tab.Specs) {
		return nil
	}
	spec := tab.Specs[col]

	// On a notes column, show the note preview overlay.
	if spec.Kind == cellNotes {
		if c, ok := m.selectedCell(col); ok && c.Value != "" {
			m.notePreview = &notePreviewState{text: c.Value, title: spec.Title}
		}
		return nil
	}

	// On a drilldown column, open the detail view for that row.
	if spec.Kind == cellDrilldown {
		return m.openDetailForRow(tab, meta.ID, spec.Title)
	}

	// On a linked column with a target, follow the FK.
	if spec.Link != nil {
		if c, ok := m.selectedCell(col); ok {
			if c.LinkID > 0 {
				return m.navigateToLink(spec.Link, c.LinkID)
			}
			m.setStatusInfo("Nothing to follow.")
		}
		return nil
	}

	// On a polymorphic entity cell, resolve the target tab from the kind letter.
	if spec.Kind == cellEntity {
		if c, ok := m.selectedCell(col); ok {
			if c.LinkID > 0 && len(c.Value) > 0 {
				if target, ok := entityLetterTab[c.Value[0]]; ok {
					return m.navigateToLink(&columnLink{TargetTab: target}, c.LinkID)
				}
			}
			m.setStatusInfo("Nothing to follow.")
		}
		return nil
	}

	// On the Documents tab, hint at the open-file shortcut.
	if tab.Kind == tabDocuments {
		m.setStatusInfo("Press o to open.")
		return nil
	}

	m.setStatusInfo("Press i to edit.")
	return nil
}

// handleEditKeys processes keys unique to Edit mode.
func (m *Model) handleEditKeys(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case keyA:
		m.startAddForm()
		return m.formInitCmd(), true
	case keyShiftA:
		if tab := m.effectiveTab(); tab != nil && tab.Kind == tabDocuments {
			if err := m.startQuickDocumentForm(); err != nil {
				m.setStatusError(err.Error())
			}
			return m.formInitCmd(), true
		}
		return nil, false
	case keyE:
		if err := m.startCellOrFormEdit(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		return m.formInitCmd(), true
	case keyShiftE:
		if err := m.startEditForm(); err != nil {
			m.setStatusError(err.Error())
			return nil, true
		}
		return m.formInitCmd(), true
	case keyD:
		m.toggleDeleteSelected()
		return nil, true
	case keyShiftD:
		m.promptHardDelete()
		return nil, true
	case keyO:
		if cmd := m.openSelectedDocument(); cmd != nil {
			return cmd, true
		}
	case keyX:
		m.toggleShowDeleted()
		return nil, true
	case keyP:
		m.startHouseForm()
		return m.formInitCmd(), true
	case keyEsc:
		m.enterNormalMode()
		return nil, true
	}
	return nil, false
}

func (m *Model) handleCalendarKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case keyH, keyLeft:
		calendarMove(m.calendar, -1)
	case keyL, keyRight:
		calendarMove(m.calendar, 1)
	case keyJ, keyDown:
		calendarMove(m.calendar, 7)
	case keyK, keyUp:
		calendarMove(m.calendar, -7)
	case keyShiftH:
		calendarMoveMonth(m.calendar, -1)
	case keyShiftL:
		calendarMoveMonth(m.calendar, 1)
	case keyLBracket:
		calendarMoveYear(m.calendar, -1)
	case keyRBracket:
		calendarMoveYear(m.calendar, 1)
	case "t":
		calendarToday(m.calendar)
	case keyEnter:
		m.confirmCalendar()
	case keyEsc:
		m.calendar = nil
		m.resetFormState()
	}
	return nil
}

func (m *Model) confirmCalendar() {
	if m.calendar == nil {
		return
	}
	dateStr := m.calendar.Cursor.Format("2006-01-02")
	if m.calendar.FieldPtr != nil {
		*m.calendar.FieldPtr = dateStr
	}
	if m.calendar.OnConfirm != nil {
		m.calendar.OnConfirm()
	}
	m.calendar = nil
}

// openCalendar opens the date picker for a form field value pointer.
func (m *Model) openCalendar(fieldPtr *string, onConfirm func()) {
	cursor := time.Now()
	var selected time.Time
	hasValue := false
	if fieldPtr != nil && *fieldPtr != "" {
		if t, err := time.ParseInLocation("2006-01-02", *fieldPtr, time.Local); err == nil {
			cursor = t
			selected = t
			hasValue = true
		}
	}
	m.calendar = &calendarState{
		Cursor:    cursor,
		Selected:  selected,
		HasValue:  hasValue,
		FieldPtr:  fieldPtr,
		OnConfirm: onConfirm,
	}
}

func (m *Model) View() string {
	return m.zones.Scan(m.buildView())
}

func (m *Model) enterNormalMode() {
	m.mode = modeNormal
	m.setAllTableKeyMaps(normalTableKeyMap())
}

func (m *Model) enterEditMode() {
	m.mode = modeEdit
	m.setAllTableKeyMaps(editTableKeyMap())
}

func (m *Model) activeTab() *Tab {
	if m.active < 0 || m.active >= len(m.tabs) {
		return nil
	}
	return &m.tabs[m.active]
}

// detail returns the top of the drilldown stack, or nil if no detail view
// is active.
func (m *Model) detail() *detailContext {
	if len(m.detailStack) == 0 {
		return nil
	}
	return m.detailStack[len(m.detailStack)-1]
}

// inDetail reports whether a detail drilldown is active.
func (m *Model) inDetail() bool {
	return len(m.detailStack) > 0
}

// effectiveTab returns the detail tab when a detail view is open, otherwise
// the main active tab. All interaction code should use this.
func (m *Model) effectiveTab() *Tab {
	if dc := m.detail(); dc != nil {
		return &dc.Tab
	}
	return m.activeTab()
}

// detailDef describes how to construct a detail drilldown view. Shared by the
// named openXxxDetail helpers and the table-driven openDetailForRow dispatch.
type detailDef struct {
	tabKind    TabKind
	subName    string
	specs      func() []columnSpec
	handler    func(parentID uint) TabHandler
	breadcrumb func(m *Model, parentName string) string
	getName    func(store *data.Store, id uint) (string, error) // resolve parent display name
}

// stdBreadcrumb returns a breadcrumb builder that produces the standard
// "prefix > parentName > subName" format. Pass "" for subName to omit it.
func stdBreadcrumb(prefix, subName string) func(*Model, string) string {
	return func(_ *Model, parentName string) string {
		if subName == "" {
			return prefix + breadcrumbSep + parentName
		}
		return prefix + breadcrumbSep + parentName + breadcrumbSep + subName
	}
}

var (
	serviceLogDef = detailDef{
		tabKind: tabMaintenance,
		subName: "Service Log",
		specs:   serviceLogColumnSpecs,
		handler: func(id uint) TabHandler { return serviceLogHandler{maintenanceItemID: id} },
		breadcrumb: func(m *Model, parentName string) string {
			// When drilled from the top-level Maintenance tab, the breadcrumb
			// starts with "Maintenance"; when nested (e.g. Appliances > … >
			// Maint item), the parent context is already on the stack.
			bc := parentName + breadcrumbSep + "Service Log"
			if !m.inDetail() {
				bc = "Maintenance" + breadcrumbSep + bc
			}
			return bc
		},
		getName: func(s *data.Store, id uint) (string, error) {
			item, err := s.GetMaintenance(id)
			if err != nil {
				return "", fmt.Errorf("load maintenance item: %w", err)
			}
			return item.Name, nil
		},
	}
	applianceMaintenanceDef = detailDef{
		tabKind:    tabAppliances,
		subName:    "Maintenance",
		specs:      applianceMaintenanceColumnSpecs,
		handler:    func(id uint) TabHandler { return newApplianceMaintenanceHandler(id) },
		breadcrumb: stdBreadcrumb("Appliances", ""),
		getName: func(s *data.Store, id uint) (string, error) {
			a, err := s.GetAppliance(id)
			if err != nil {
				return "", fmt.Errorf("load appliance: %w", err)
			}
			return a.Name, nil
		},
	}
	vendorQuoteDef = detailDef{
		tabKind:    tabVendors,
		subName:    tabQuotes.String(),
		specs:      vendorQuoteColumnSpecs,
		handler:    func(id uint) TabHandler { return newVendorQuoteHandler(id) },
		breadcrumb: stdBreadcrumb("Vendors", tabQuotes.String()),
		getName:    getVendorName,
	}
	vendorJobsDef = detailDef{
		tabKind:    tabVendors,
		subName:    "Jobs",
		specs:      vendorJobsColumnSpecs,
		handler:    func(id uint) TabHandler { return newVendorJobsHandler(id) },
		breadcrumb: stdBreadcrumb("Vendors", "Jobs"),
		getName:    getVendorName,
	}
	projectQuoteDef = detailDef{
		tabKind:    tabProjects,
		subName:    tabQuotes.String(),
		specs:      projectQuoteColumnSpecs,
		handler:    func(id uint) TabHandler { return newProjectQuoteHandler(id) },
		breadcrumb: stdBreadcrumb("Projects", tabQuotes.String()),
		getName:    getProjectTitle,
	}
	projectDocumentDef = detailDef{
		tabKind:    tabProjects,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id uint) TabHandler { return newEntityDocumentHandler(data.DocumentEntityProject, id) },
		breadcrumb: stdBreadcrumb("Projects", tabDocuments.String()),
		getName:    getProjectTitle,
	}
	incidentDocumentDef = detailDef{
		tabKind:    tabIncidents,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id uint) TabHandler { return newEntityDocumentHandler(data.DocumentEntityIncident, id) },
		breadcrumb: stdBreadcrumb("Incidents", tabDocuments.String()),
		getName:    getIncidentTitle,
	}
	applianceDocumentDef = detailDef{
		tabKind:    tabAppliances,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id uint) TabHandler { return newEntityDocumentHandler(data.DocumentEntityAppliance, id) },
		breadcrumb: stdBreadcrumb("Appliances", tabDocuments.String()),
		getName: func(s *data.Store, id uint) (string, error) {
			a, err := s.GetAppliance(id)
			if err != nil {
				return "", fmt.Errorf("load appliance: %w", err)
			}
			return a.Name, nil
		},
	}
	serviceLogDocumentDef = detailDef{
		tabKind: tabMaintenance,
		subName: tabDocuments.String(),
		specs:   entityDocumentColumnSpecs,
		handler: func(id uint) TabHandler {
			return newEntityDocumentHandler(data.DocumentEntityServiceLog, id)
		},
		breadcrumb: func(_ *Model, parentName string) string {
			// Service log docs are always opened from within a service log
			// detail, so the parent breadcrumb is already on the stack.
			return parentName + breadcrumbSep + tabDocuments.String()
		},
		getName: func(s *data.Store, id uint) (string, error) {
			entry, err := s.GetServiceLog(id)
			if err != nil {
				return "", fmt.Errorf("load service log: %w", err)
			}
			return entry.ServicedAt.Format(data.DateLayout), nil
		},
	}
	maintenanceDocumentDef = detailDef{
		tabKind:    tabMaintenance,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id uint) TabHandler { return newEntityDocumentHandler(data.DocumentEntityMaintenance, id) },
		breadcrumb: stdBreadcrumb("Maintenance", tabDocuments.String()),
		getName:    getMaintenanceName,
	}
	quoteDocumentDef = detailDef{
		tabKind:    tabQuotes,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id uint) TabHandler { return newEntityDocumentHandler(data.DocumentEntityQuote, id) },
		breadcrumb: stdBreadcrumb("Quotes", tabDocuments.String()),
		getName:    getQuoteDisplayName,
	}
	vendorDocumentDef = detailDef{
		tabKind:    tabVendors,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id uint) TabHandler { return newEntityDocumentHandler(data.DocumentEntityVendor, id) },
		breadcrumb: stdBreadcrumb("Vendors", tabDocuments.String()),
		getName:    getVendorName,
	}
)

// Shared getName helpers for defs that resolve the same entity type.
func getVendorName(s *data.Store, id uint) (string, error) {
	v, err := s.GetVendor(id)
	if err != nil {
		return "", fmt.Errorf("load vendor: %w", err)
	}
	return v.Name, nil
}

func getIncidentTitle(s *data.Store, id uint) (string, error) {
	inc, err := s.GetIncident(id)
	if err != nil {
		return "", fmt.Errorf("load incident: %w", err)
	}
	return inc.Title, nil
}

func getProjectTitle(s *data.Store, id uint) (string, error) {
	p, err := s.GetProject(id)
	if err != nil {
		return "", fmt.Errorf("load project: %w", err)
	}
	return p.Title, nil
}

func getMaintenanceName(s *data.Store, id uint) (string, error) {
	item, err := s.GetMaintenance(id)
	if err != nil {
		return "", fmt.Errorf("load maintenance item: %w", err)
	}
	return item.Name, nil
}

func getQuoteDisplayName(s *data.Store, id uint) (string, error) {
	q, err := s.GetQuote(id)
	if err != nil {
		return "", fmt.Errorf("load quote: %w", err)
	}
	return fmt.Sprintf("%s #%d", q.Vendor.Name, q.ID), nil
}

// openDetailFromDef opens a drilldown using a detail definition.
func (m *Model) openDetailFromDef(def detailDef, parentID uint, parentName string) error {
	specs := def.specs()
	return m.openDetailWith(detailContext{
		ParentTabIndex: m.active,
		ParentRowID:    parentID,
		Breadcrumb:     def.breadcrumb(m, parentName),
		Tab: Tab{
			Kind:    def.tabKind,
			Name:    def.subName,
			Handler: def.handler(parentID),
			Specs:   specs,
			Table:   newTable(specsToColumns(specs)),
		},
	})
}

func (m *Model) openServiceLogDetail(maintID uint, maintName string) error {
	return m.openDetailFromDef(serviceLogDef, maintID, maintName)
}

func (m *Model) openApplianceMaintenanceDetail(applianceID uint, applianceName string) error {
	return m.openDetailFromDef(applianceMaintenanceDef, applianceID, applianceName)
}

func (m *Model) openVendorQuoteDetail(vendorID uint, vendorName string) error {
	return m.openDetailFromDef(vendorQuoteDef, vendorID, vendorName)
}

func (m *Model) openVendorJobsDetail(vendorID uint, vendorName string) error {
	return m.openDetailFromDef(vendorJobsDef, vendorID, vendorName)
}

func (m *Model) openProjectQuoteDetail(projectID uint, projectTitle string) error {
	return m.openDetailFromDef(projectQuoteDef, projectID, projectTitle)
}

func (m *Model) openProjectDocumentDetail(projectID uint, projectTitle string) error {
	return m.openDetailFromDef(projectDocumentDef, projectID, projectTitle)
}

func (m *Model) openApplianceDocumentDetail(applianceID uint, applianceName string) error {
	return m.openDetailFromDef(applianceDocumentDef, applianceID, applianceName)
}

// detailRoute maps a (TabKind, colTitle) pair to a detailDef for dispatch.
// When formKind is set (non-zero), the route only matches if the tab handler's
// FormKind equals it. This disambiguates nested detail views that share a
// parent tabKind but belong to different entity types (e.g. appliance
// maintenance detail has tabKind=tabAppliances but formKind=formMaintenance).
type detailRoute struct {
	tabKinds []TabKind
	colTitle string
	def      detailDef
	formKind FormKind
}

var detailRoutes = []detailRoute{
	{tabKinds: []TabKind{tabMaintenance, tabAppliances}, colTitle: "Log", def: serviceLogDef},
	{tabKinds: []TabKind{tabAppliances}, colTitle: "Maint", def: applianceMaintenanceDef},
	{tabKinds: []TabKind{tabVendors}, colTitle: tabQuotes.String(), def: vendorQuoteDef},
	{tabKinds: []TabKind{tabVendors}, colTitle: "Jobs", def: vendorJobsDef},
	{tabKinds: []TabKind{tabProjects}, colTitle: tabQuotes.String(), def: projectQuoteDef},
	// Handler-scoped document routes: match nested detail views where the
	// parent tabKind is shared but the handler identifies the entity type.
	{
		tabKinds: []TabKind{tabMaintenance, tabAppliances},
		colTitle: tabDocuments.String(),
		def:      serviceLogDocumentDef,
		formKind: formServiceLog,
	},
	{
		tabKinds: []TabKind{tabMaintenance, tabAppliances},
		colTitle: tabDocuments.String(),
		def:      maintenanceDocumentDef,
		formKind: formMaintenance,
	},
	{
		tabKinds: []TabKind{tabQuotes, tabVendors, tabProjects},
		colTitle: tabDocuments.String(),
		def:      quoteDocumentDef,
		formKind: formQuote,
	},
	// Generic document routes: match top-level tabs (no formKind filter).
	{tabKinds: []TabKind{tabProjects}, colTitle: tabDocuments.String(), def: projectDocumentDef},
	{tabKinds: []TabKind{tabIncidents}, colTitle: tabDocuments.String(), def: incidentDocumentDef},
	{
		tabKinds: []TabKind{tabAppliances},
		colTitle: tabDocuments.String(),
		def:      applianceDocumentDef,
	},
	{tabKinds: []TabKind{tabVendors}, colTitle: tabDocuments.String(), def: vendorDocumentDef},
}

// openDetailForRow dispatches a drilldown based on the current tab kind and the
// column that was activated. Supports nested drilldowns (e.g. Appliance →
// Maintenance → Service Log).
func (m *Model) openDetailForRow(tab *Tab, rowID uint, colTitle string) error {
	for _, route := range detailRoutes {
		if route.colTitle != colTitle {
			continue
		}
		if route.formKind != formNone &&
			(tab.Handler == nil || tab.Handler.FormKind() != route.formKind) {
			continue
		}
		for _, kind := range route.tabKinds {
			if tab.Kind == kind {
				name, err := route.def.getName(m.store, rowID)
				if err != nil {
					return err
				}
				return m.openDetailFromDef(route.def, rowID, name)
			}
		}
	}
	return nil
}

func (m *Model) openDetailWith(dc detailContext) error {
	m.detailStack = append(m.detailStack, &dc)
	if err := m.reloadDetailTab(); err != nil {
		m.detailStack = m.detailStack[:len(m.detailStack)-1]
		return err
	}
	m.resizeTables()
	m.status = statusMsg{}
	return nil
}

func (m *Model) closeDetail() {
	if len(m.detailStack) == 0 {
		return
	}
	top := m.detailStack[len(m.detailStack)-1]
	m.detailStack = m.detailStack[:len(m.detailStack)-1]

	// If there's still a parent detail view on the stack, reload it and
	// restore the cursor to the row that opened the now-closed child.
	if parent := m.detail(); parent != nil {
		if m.store != nil {
			m.surfaceError(m.reloadTab(&parent.Tab))
		}
		selectRowByID(&parent.Tab, top.ParentRowID)
	} else {
		// Back to a top-level tab.
		m.active = top.ParentTabIndex
		if m.store != nil {
			tab := m.activeTab()
			if tab != nil && tab.Stale {
				m.surfaceError(m.reloadIfStale(tab))
			} else {
				m.surfaceError(m.reloadActiveTab())
			}
		}
		if tab := m.activeTab(); tab != nil {
			selectRowByID(tab, top.ParentRowID)
		}
	}
	m.resizeTables()
	m.status = statusMsg{}
}

// closeAllDetails collapses the entire drilldown stack back to the top-level tab.
// Unlike calling closeDetail() in a loop, this pops the whole stack and only
// reloads the final destination tab once, avoiding wasted intermediate reloads.
func (m *Model) closeAllDetails() {
	if len(m.detailStack) == 0 {
		return
	}
	// The bottom entry holds the original top-level tab that we're returning to.
	bottom := m.detailStack[0]
	m.detailStack = nil
	m.active = bottom.ParentTabIndex
	if m.store != nil {
		tab := m.activeTab()
		if tab != nil && tab.Stale {
			m.surfaceError(m.reloadIfStale(tab))
		} else {
			m.surfaceError(m.reloadActiveTab())
		}
	}
	if tab := m.activeTab(); tab != nil {
		selectRowByID(tab, bottom.ParentRowID)
	}
	m.resizeTables()
	m.status = statusMsg{}
}

func (m *Model) reloadDetailTab() error {
	dc := m.detail()
	if dc == nil || m.store == nil {
		return nil
	}
	return m.reloadTab(&dc.Tab)
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
	m.surfaceError(m.reloadEffectiveTab())
	m.markNonEffectiveStale()
	if m.showDashboard {
		m.surfaceError(m.loadDashboard())
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
		// Close all drilldown levels when returning to dashboard.
		m.closeAllDetails()
	}
	if m.store != nil {
		m.surfaceError(m.store.PutShowDashboard(m.showDashboard))
	}
}

// switchToTab sets the active tab index, reloads it (lazy if stale), and
// clears the status message. Centralizes the reload-after-switch pattern.
func (m *Model) switchToTab(idx int) {
	m.active = idx
	m.status = statusMsg{}
	tab := m.activeTab()
	if tab != nil && tab.Stale {
		m.surfaceError(m.reloadIfStale(tab))
	} else {
		m.surfaceError(m.reloadActiveTab())
	}
}

func (m *Model) nextTab() {
	if len(m.tabs) == 0 {
		return
	}
	next := m.active + 1
	if next >= len(m.tabs) {
		return
	}
	m.switchToTab(next)
}

func (m *Model) prevTab() {
	if len(m.tabs) == 0 {
		return
	}
	if m.active <= 0 {
		return
	}
	m.switchToTab(m.active - 1)
}

func (m *Model) startAddForm() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	if err := tab.Handler.StartAddForm(m); err != nil {
		m.setStatusError(err.Error())
	}
}

func (m *Model) startEditForm() error {
	tab := m.effectiveTab()
	if tab == nil {
		return fmt.Errorf("no active tab")
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		return fmt.Errorf("nothing selected")
	}
	if meta.Deleted {
		return fmt.Errorf("cannot edit a deleted item")
	}
	return tab.Handler.StartEditForm(m, meta.ID)
}

func (m *Model) startCellOrFormEdit() error {
	tab := m.effectiveTab()
	if tab == nil {
		return fmt.Errorf("no active tab")
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		return fmt.Errorf("nothing selected")
	}
	if meta.Deleted {
		return fmt.Errorf("cannot edit a deleted item")
	}
	col := tab.ColCursor
	if col < 0 || col >= len(tab.Specs) {
		col = 0
	}
	spec := tab.Specs[col]

	// If the column is linked and the cell has a target ID, navigate cross-tab.
	if spec.Link != nil {
		if c, ok := m.selectedCell(col); ok && c.LinkID > 0 {
			return m.navigateToLink(spec.Link, c.LinkID)
		}
	}

	if spec.Kind == cellReadonly || spec.Kind == cellDrilldown {
		return m.startEditForm()
	}
	return tab.Handler.InlineEdit(m, meta.ID, col)
}

// navigateToLink closes any open drilldown stack, switches to the target tab,
// and selects the row matching the FK.
func (m *Model) navigateToLink(link *columnLink, targetID uint) error {
	m.closeAllDetails()
	m.switchToTab(tabIndex(link.TargetTab))
	tab := m.activeTab()
	if tab == nil {
		return fmt.Errorf("target tab not found")
	}
	if !selectRowByID(tab, targetID) {
		m.setStatusError(fmt.Sprintf("Linked item %d not found (deleted?).", targetID))
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

func (m *Model) toggleDeleteSelected() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		m.setStatusError("Nothing selected.")
		return
	}
	if meta.Deleted {
		if err := tab.Handler.Restore(m.store, meta.ID); err != nil {
			m.setStatusError(err.Error())
			return
		}
		if tab.LastDeleted != nil && *tab.LastDeleted == meta.ID {
			tab.LastDeleted = nil
		}
		if tab.Kind == tabIncidents {
			m.setStatusInfo("Reopened.")
		} else {
			m.setStatusInfo("Restored.")
		}
		m.surfaceError(m.reloadEffectiveTab())
		return
	}
	if err := tab.Handler.Delete(m.store, meta.ID); err != nil {
		m.setStatusError(err.Error())
		return
	}
	tab.LastDeleted = &meta.ID
	tab.ShowDeleted = true
	if tab.Kind == tabIncidents {
		m.setStatusInfo("Resolved. Press d to reopen.")
	} else {
		m.setStatusInfo("Deleted. Press d to restore.")
	}
	m.surfaceError(m.reloadEffectiveTab())
}

func (m *Model) promptHardDelete() {
	tab := m.effectiveTab()
	if tab == nil || tab.Kind != tabIncidents {
		return
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		m.setStatusError("Nothing selected.")
		return
	}
	if !meta.Deleted {
		m.setStatusError("Resolve the incident first (d), then permanently delete (D).")
		return
	}
	m.confirmHardDelete = true
	m.hardDeleteID = meta.ID
}

func (m *Model) handleConfirmHardDelete(key tea.KeyMsg) {
	switch key.String() {
	case keyY:
		m.confirmHardDelete = false
		if err := m.store.HardDeleteIncident(m.hardDeleteID); err != nil {
			m.setStatusError(err.Error())
			return
		}
		m.setStatusInfo("Permanently deleted.")
		m.surfaceError(m.reloadEffectiveTab())
	case keyN, keyEsc:
		m.confirmHardDelete = false
	}
}

func (m *Model) toggleShowDeleted() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	tab.ShowDeleted = !tab.ShowDeleted
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
		m.tabs[i].Table.SetHeight(tableHeight)
		m.tabs[i].Table.SetWidth(m.width)
	}
	if dc := m.detail(); dc != nil {
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

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
		return startPull(client.BaseURL(), model)
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
					docID, doc.FileName, doc.Data, doc.MIMEType, doc.ExtractedText,
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
	m.pull.progress.Width = barW
	return label + " " + m.pull.progress.ViewAs(pct)
}

// extractionLLMClient returns the LLM client configured for extraction,
// or nil if extraction is not available. The client is created once and cached.
// When extraction has its own provider/connection settings, a fully independent
// client is created; otherwise it clones the chat client with a model override.
func (m *Model) extractionLLMClient() *llm.Client {
	if m.ex.extractionClient != nil {
		return m.ex.extractionClient
	}

	provider := m.ex.extractionProvider
	baseURL := m.ex.extractionBaseURL
	apiKey := m.ex.extractionAPIKey
	timeout := m.ex.extractionTimeout
	model := m.ex.extractionModel

	// Fill gaps from the chat client config when extraction doesn't have
	// its own connection settings.
	if provider == "" || baseURL == "" || apiKey == "" || timeout == 0 {
		if m.llmConfig == nil {
			return nil
		}
		if provider == "" {
			provider = m.llmConfig.Provider
		}
		if baseURL == "" {
			baseURL = m.llmConfig.BaseURL
		}
		if apiKey == "" {
			apiKey = m.llmConfig.APIKey
		}
		if timeout == 0 {
			timeout = m.llmConfig.Timeout
		}
	}

	if model == "" {
		if m.llmClient != nil {
			model = m.llmClient.Model()
		} else if m.llmConfig != nil {
			model = m.llmConfig.Model
		}
	}
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

	// Determine if async extraction is needed.
	needsExtract := extract.NeedsOCR(m.ex.extractors, meta.MIMEType)

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

	isFirstHouse := m.fs.formKind == formHouse && !m.hasHouse
	kind := m.fs.formKind
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
	kind := m.fs.formKind
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
		0, // no DB ID yet
		doc.FileName,
		doc.Data,
		doc.MIMEType,
		doc.ExtractedText,
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
func cloneFormData(data any) any {
	if data == nil {
		return nil
	}
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr && !v.IsNil() {
		cp := reflect.New(v.Elem().Type())
		cp.Elem().Set(v.Elem())
		return cp.Interface()
	}
	return data
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

	vp := viewport.New(maxW, viewH)
	vp.SetContent(content)
	// Disable horizontal scroll to avoid conflicts with table navigation.
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)
	m.helpViewport = &vp
}

func (m *Model) handleInlineInputKey(key tea.KeyMsg) tea.Cmd {
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
	kind := ii.FormKind
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
	m.fs.formKind = formNone
	m.fs.form = nil
	m.fs.formData = nil
	m.fs.formSnapshot = nil
	m.fs.formDirty = false
	m.fs.confirmDiscard = false
	m.fs.pendingFormInit = nil
	m.fs.editID = nil
	m.fs.notesEditMode = false
	m.fs.notesFieldPtr = nil
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
}

func (m *Model) setStatusInfo(text string) {
	m.status = statusMsg{Text: text, Kind: statusInfo}
}

func (m *Model) setStatusSaved() {
	m.setStatusInfo("Saved.")
}

func (m *Model) setStatusError(text string) {
	m.status = statusMsg{Text: text, Kind: statusError}
}

// surfaceError shows a reload failure in the status bar. Used in
// fire-and-forget reload paths where the caller cannot return an error.
func (m *Model) surfaceError(err error) {
	if err != nil {
		m.setStatusError(err.Error())
	}
}

func (m *Model) formInitCmd() tea.Cmd {
	cmd := m.fs.pendingFormInit
	m.fs.pendingFormInit = nil
	return cmd
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

// hasActiveOverlay returns true when any overlay is currently shown. Overlays
// include dashboard, calendar, note preview, column finder, and help. When
// true, main tab keybindings should be hidden from the status bar since they
// are not accessible.
func (m *Model) hasActiveOverlay() bool {
	return m.dashboardVisible() ||
		m.calendar != nil ||
		m.notePreview != nil ||
		m.columnFinder != nil ||
		(m.ex.extraction != nil && m.ex.extraction.Visible) ||
		m.helpViewport != nil
}

func (m *Model) dispatchOverlay(msg tea.Msg) (tea.Cmd, bool) {
	var handler func(tea.KeyMsg) tea.Cmd
	switch {
	case m.helpViewport != nil:
		handler = m.helpOverlayKey
	case m.ex.extraction != nil && m.ex.extraction.Visible:
		handler = m.handleExtractionKey
	case m.chat != nil && m.chat.Visible:
		handler = m.handleChatKey
	case m.notePreview != nil:
		handler = func(tea.KeyMsg) tea.Cmd { m.notePreview = nil; return nil }
	case m.calendar != nil:
		handler = m.handleCalendarKey
	case m.columnFinder != nil:
		handler = m.handleColumnFinderKey
	case m.inlineInput != nil:
		handler = m.handleInlineInputKey
	default:
		return nil, false
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		// Non-key messages (cursor blink, spinner ticks, etc.) should not
		// be swallowed by the overlay dispatcher. Return false so the
		// caller's normal Update path can handle them.
		return nil, false
	}
	return handler(keyMsg), true
}

func (m *Model) helpOverlayKey(keyMsg tea.KeyMsg) tea.Cmd {
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

func selectRowByID(tab *Tab, id uint) bool {
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

// editorBinary returns the user's preferred editor from $EDITOR or $VISUAL.
// Returns "" if neither is set.
func editorBinary() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return os.Getenv("VISUAL")
}

// launchExternalEditor writes the current notes text to a temp file and
// launches $EDITOR via tea.ExecProcess. The textarea is closed so the
// terminal is fully released to the editor.
func (m *Model) launchExternalEditor() tea.Cmd {
	editor := editorBinary()
	if editor == "" {
		m.setStatusError("Set $EDITOR or $VISUAL to use an external editor.")
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
		EditID:   0,
		FormKind: m.fs.formKind,
		FormData: m.fs.formData,
		FieldPtr: m.fs.notesFieldPtr,
		TempFile: f.Name(),
	}
	if m.fs.editID != nil {
		m.fs.pendingEditor.EditID = *m.fs.editID
	}

	m.exitForm()

	cmd := exec.Command( //nolint:gosec,noctx // user-configured editor; no context in tea.Cmd
		editor,
		f.Name(),
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
	if pe.EditID > 0 {
		id := pe.EditID
		m.fs.editID = &id
	}
	m.fs.formData = pe.FormData
	m.openNotesTextarea(pe.FormKind, pe.FieldPtr, pe.FormData)
}

// autoDetectModel checks if the LLM server has exactly one model available
// and returns it. Returns "" if the server is unreachable or has zero/multiple
// models (ambiguous cases where manual config is safer).
func autoDetectModel(client *llm.Client) string {
	ctx, cancel := context.WithTimeout(context.Background(), client.Timeout())
	defer cancel()

	models, err := client.ListModels(ctx)
	if err != nil || len(models) != 1 {
		return ""
	}
	return models[0]
}
