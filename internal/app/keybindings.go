// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import "charm.land/bubbles/v2/key"

// AppKeyMap defines all keybindings as structured key.Binding values.
// Each binding carries the actual keys for dispatch (via key.Matches)
// and display text for the help overlay (via key.Binding.Help).
//
// Bindings are grouped by the dispatch handler that uses them.
// Some bindings are shared across handlers (noted in comments).
// Some bindings are help-display-only (no WithKeys; never matched).
// Table-delegated bindings (j/k row nav, g/G, d/u half-page) are
// NOT here — they live in the bubbles table.KeyMap.
type AppKeyMap struct {
	// --- Global (pre-overlay, model_update.go) ---
	Quit   key.Binding
	Cancel key.Binding

	// --- Common (handleCommonKeys — both normal + edit) ---
	ColLeft     key.Binding
	ColRight    key.Binding
	ColStart    key.Binding
	ColEnd      key.Binding
	Help        key.Binding
	HouseToggle key.Binding
	MagToggle   key.Binding // also used in handleChatKey
	FgExtract   key.Binding

	// --- Normal mode (handleNormalKeys) ---
	TabNext       key.Binding
	TabPrev       key.Binding
	TabFirst      key.Binding
	TabLast       key.Binding
	EnterEditMode key.Binding
	Enter         key.Binding
	Dashboard     key.Binding
	Sort          key.Binding
	SortClear     key.Binding
	ToggleSettled key.Binding
	FilterPin     key.Binding
	FilterToggle  key.Binding
	FilterClear   key.Binding
	FilterNegate  key.Binding
	ColHide       key.Binding
	ColShowAll    key.Binding
	ColFinder     key.Binding
	DocSearch     key.Binding
	DocOpen       key.Binding // also used in handleEditKeys
	ToggleUnits   key.Binding
	Chat          key.Binding
	Escape        key.Binding
	YankCell      key.Binding

	// --- Edit mode (handleEditKeys) ---
	Add         key.Binding
	QuickAdd    key.Binding
	EditCell    key.Binding
	EditFull    key.Binding
	Delete      key.Binding
	HardDelete  key.Binding
	ReExtract   key.Binding
	ShowDeleted key.Binding
	HouseEdit   key.Binding
	ExitEdit    key.Binding

	// --- Forms (model_update.go:updateForm if-guards) ---
	FormSave        key.Binding
	FormCancel      key.Binding
	FormNextField   key.Binding // help-only; huh dispatches internally
	FormPrevField   key.Binding // help-only; huh dispatches internally
	FormEditor      key.Binding
	FormHiddenFiles key.Binding

	// --- Chat (handleChatKey main) ---
	ChatSend      key.Binding
	ChatToggleSQL key.Binding
	ChatHistoryUp key.Binding
	ChatHistoryDn key.Binding
	ChatHide      key.Binding

	// --- Chat completer (handleChatKey completer) ---
	CompleterUp      key.Binding
	CompleterDown    key.Binding
	CompleterConfirm key.Binding
	CompleterCancel  key.Binding

	// --- Calendar (handleCalendarKey) ---
	CalLeft      key.Binding
	CalRight     key.Binding
	CalUp        key.Binding
	CalDown      key.Binding
	CalPrevMonth key.Binding
	CalNextMonth key.Binding
	CalPrevYear  key.Binding
	CalNextYear  key.Binding
	CalToday     key.Binding
	CalConfirm   key.Binding
	CalCancel    key.Binding

	// --- Dashboard (handleDashboardKeys) ---
	DashUp          key.Binding
	DashDown        key.Binding
	DashNextSection key.Binding
	DashPrevSection key.Binding
	DashTop         key.Binding
	DashBottom      key.Binding
	DashToggle      key.Binding
	DashToggleAll   key.Binding
	DashJump        key.Binding

	// --- Doc search (handleDocSearchKey) ---
	DocSearchUp      key.Binding
	DocSearchDown    key.Binding
	DocSearchConfirm key.Binding
	DocSearchCancel  key.Binding

	// --- Column finder (handleColumnFinderKey) ---
	ColFinderUp        key.Binding
	ColFinderDown      key.Binding
	ColFinderConfirm   key.Binding
	ColFinderCancel    key.Binding
	ColFinderClear     key.Binding
	ColFinderBackspace key.Binding

	// --- Ops tree (handleOpsTreeKey) ---
	OpsUp       key.Binding
	OpsDown     key.Binding
	OpsExpand   key.Binding
	OpsCollapse key.Binding
	OpsTabNext  key.Binding
	OpsTabPrev  key.Binding
	OpsTop      key.Binding
	OpsBottom   key.Binding
	OpsClose    key.Binding

	// --- Extraction pipeline (handleExtractionPipelineKey) ---
	ExtCancel     key.Binding
	ExtInterrupt  key.Binding
	ExtUp         key.Binding
	ExtDown       key.Binding
	ExtToggle     key.Binding
	ExtRemodel    key.Binding
	ExtToggleTSV  key.Binding
	ExtAccept     key.Binding
	ExtExplore    key.Binding
	ExtBackground key.Binding

	// --- Extraction explore (handleExtractionExploreKey) ---
	ExploreUp       key.Binding
	ExploreDown     key.Binding
	ExploreLeft     key.Binding
	ExploreRight    key.Binding
	ExploreColStart key.Binding
	ExploreColEnd   key.Binding
	ExploreTop      key.Binding
	ExploreBottom   key.Binding
	ExploreTabNext  key.Binding
	ExploreTabPrev  key.Binding
	ExploreAccept   key.Binding
	ExploreExit     key.Binding

	// --- Extraction model picker (handleExtractionModelPickerKey) ---
	ExtModelUp        key.Binding
	ExtModelDown      key.Binding
	ExtModelConfirm   key.Binding
	ExtModelCancel    key.Binding
	ExtModelBackspace key.Binding

	// --- Help overlay (helpOverlayKey) ---
	HelpGotoTop    key.Binding
	HelpGotoBottom key.Binding
	HelpClose      key.Binding

	// --- Confirmations (handleConfirmDiscard, handleConfirmHardDelete) ---
	ConfirmYes key.Binding
	ConfirmNo  key.Binding

	// --- Inline input (handleInlineInputKey) ---
	InlineConfirm key.Binding
	InlineCancel  key.Binding
}

func newAppKeyMap() AppKeyMap {
	return AppKeyMap{
		// Global
		Quit: key.NewBinding(key.WithKeys(keyCtrlQ), key.WithHelp("ctrl+q", "quit")),
		Cancel: key.NewBinding(
			key.WithKeys(keyCtrlC),
			key.WithHelp("ctrl+c", "cancel LLM operation"),
		),

		// Common
		ColLeft: key.NewBinding(
			key.WithKeys(keyH, keyLeft),
			key.WithHelp(keyH+"/"+keyL+"/"+symLeft+"/"+symRight, "columns"),
		),
		ColRight: key.NewBinding(key.WithKeys(keyL, keyRight)),
		ColStart: key.NewBinding(
			key.WithKeys(keyCaret),
			key.WithHelp(keyCaret+"/"+keyDollar, "first/last column"),
		),
		ColEnd:      key.NewBinding(key.WithKeys(keyDollar)),
		Help:        key.NewBinding(key.WithKeys(keyQuestion), key.WithHelp(keyQuestion, "help")),
		HouseToggle: key.NewBinding(key.WithKeys(keyTab), key.WithHelp("tab", "house profile")),
		MagToggle:   key.NewBinding(key.WithKeys(keyCtrlO)),
		FgExtract:   key.NewBinding(key.WithKeys(keyCtrlB)),

		// Normal mode
		TabNext: key.NewBinding(
			key.WithKeys(keyF),
			key.WithHelp(keyB+"/"+keyF, "switch tabs"),
		),
		TabPrev: key.NewBinding(key.WithKeys(keyB)),
		TabFirst: key.NewBinding(
			key.WithKeys(keyShiftB),
			key.WithHelp(keyShiftB+"/"+keyShiftF, "first/last tab"),
		),
		TabLast:       key.NewBinding(key.WithKeys(keyShiftF)),
		EnterEditMode: key.NewBinding(key.WithKeys(keyI), key.WithHelp(keyI, "edit mode")),
		Enter: key.NewBinding(
			key.WithKeys(keyEnter),
			key.WithHelp(symReturn, drilldownArrow+" drill / "+linkArrow+" follow / preview"),
		),
		Dashboard: key.NewBinding(key.WithKeys(keyShiftD), key.WithHelp(keyShiftD, "summary")),
		Sort: key.NewBinding(
			key.WithKeys(keyS),
			key.WithHelp(keyS+"/"+keyShiftS, "sort / clear sorts"),
		),
		SortClear: key.NewBinding(key.WithKeys(keyShiftS)),
		ToggleSettled: key.NewBinding(
			key.WithKeys(keyT),
			key.WithHelp(keyT, "toggle settled projects"),
		),
		FilterPin: key.NewBinding(key.WithKeys(keyN), key.WithHelp(keyN, "pin/unpin")),
		FilterToggle: key.NewBinding(
			key.WithKeys(keyShiftN),
			key.WithHelp(keyShiftN, "toggle filter"),
		),
		FilterClear: key.NewBinding(
			key.WithKeys(keyCtrlN),
			key.WithHelp("ctrl+n", "clear pins and filter"),
		),
		FilterNegate: key.NewBinding(
			key.WithKeys(keyBang),
			key.WithHelp(keyBang, "invert filter"),
		),
		ColHide: key.NewBinding(
			key.WithKeys(keyC),
			key.WithHelp(keyC+"/"+keyShiftC, "toggle column visibility"),
		),
		ColShowAll: key.NewBinding(key.WithKeys(keyShiftC)),
		ColFinder: key.NewBinding(
			key.WithKeys(keySlash),
			key.WithHelp(keySlash, "find column"),
		),
		DocSearch: key.NewBinding(
			key.WithKeys(keyCtrlF),
			key.WithHelp("ctrl+f", "search documents"),
		),
		DocOpen: key.NewBinding(key.WithKeys(keyO), key.WithHelp(keyO, "open document")),
		ToggleUnits: key.NewBinding(
			key.WithKeys(keyShiftU),
			key.WithHelp(keyShiftU, "toggle units"),
		),
		Chat: key.NewBinding(key.WithKeys(keyAt), key.WithHelp(keyAt, "ask LLM")),
		Escape: key.NewBinding(
			key.WithKeys(keyEsc),
			key.WithHelp("esc", "close detail / clear status"),
		),
		YankCell: key.NewBinding(key.WithKeys(keyY), key.WithHelp(keyY, "copy cell")),

		// Edit mode
		Add: key.NewBinding(key.WithKeys(keyA), key.WithHelp(keyA, "add entry")),
		QuickAdd: key.NewBinding(
			key.WithKeys(keyShiftA),
			key.WithHelp(keyShiftA, "add document with extraction"),
		),
		EditCell: key.NewBinding(key.WithKeys(keyE), key.WithHelp(keyE, "edit cell or row")),
		EditFull: key.NewBinding(
			key.WithKeys(keyShiftE),
			key.WithHelp(keyShiftE, "edit row (full form)"),
		),
		Delete: key.NewBinding(key.WithKeys(keyD), key.WithHelp(keyD, "del/restore")),
		HardDelete: key.NewBinding(
			key.WithKeys(keyShiftD),
			key.WithHelp(keyShiftD, "permanently delete"),
		),
		ReExtract:   key.NewBinding(key.WithKeys(keyR), key.WithHelp(keyR, "re-extract")),
		ShowDeleted: key.NewBinding(key.WithKeys(keyX), key.WithHelp(keyX, "show deleted")),
		HouseEdit:   key.NewBinding(key.WithKeys(keyP), key.WithHelp(keyP, "house profile")),
		ExitEdit:    key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "nav mode")),

		// Forms
		FormSave:      key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "save")),
		FormCancel:    key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "cancel")),
		FormNextField: key.NewBinding(key.WithHelp("tab", "next field")),
		FormPrevField: key.NewBinding(key.WithHelp("shift+tab", "previous field")),
		FormEditor: key.NewBinding(
			key.WithKeys(keyCtrlE),
			key.WithHelp("ctrl+e", "open notes in $EDITOR"),
		),
		FormHiddenFiles: key.NewBinding(
			key.WithKeys(keyShiftH),
			key.WithHelp(keyShiftH, "toggle hidden files"),
		),

		// Chat
		ChatSend: key.NewBinding(
			key.WithKeys(keyEnter),
			key.WithHelp(symReturn, "send message"),
		),
		ChatToggleSQL: key.NewBinding(
			key.WithKeys(keyCtrlS),
			key.WithHelp("ctrl+s", "toggle SQL display"),
		),
		ChatHistoryUp: key.NewBinding(
			key.WithKeys(keyUp, keyCtrlP),
			key.WithHelp(symUp+"/"+symDown, "prompt history"),
		),
		ChatHistoryDn: key.NewBinding(key.WithKeys(keyDown, keyCtrlN)),
		ChatHide:      key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "hide chat")),

		// Chat completer
		CompleterUp:      key.NewBinding(key.WithKeys(keyUp, keyCtrlP)),
		CompleterDown:    key.NewBinding(key.WithKeys(keyDown, keyCtrlN)),
		CompleterConfirm: key.NewBinding(key.WithKeys(keyEnter)),
		CompleterCancel:  key.NewBinding(key.WithKeys(keyEsc)),

		// Calendar
		CalLeft:      key.NewBinding(key.WithKeys(keyH, keyLeft)),
		CalRight:     key.NewBinding(key.WithKeys(keyL, keyRight)),
		CalUp:        key.NewBinding(key.WithKeys(keyK, keyUp)),
		CalDown:      key.NewBinding(key.WithKeys(keyJ, keyDown)),
		CalPrevMonth: key.NewBinding(key.WithKeys(keyShiftH)),
		CalNextMonth: key.NewBinding(key.WithKeys(keyShiftL)),
		CalPrevYear:  key.NewBinding(key.WithKeys(keyLBracket)),
		CalNextYear:  key.NewBinding(key.WithKeys(keyRBracket)),
		CalToday:     key.NewBinding(key.WithKeys(keyT)),
		CalConfirm:   key.NewBinding(key.WithKeys(keyEnter)),
		CalCancel:    key.NewBinding(key.WithKeys(keyEsc)),

		// Dashboard
		DashUp:          key.NewBinding(key.WithKeys(keyK, keyUp)),
		DashDown:        key.NewBinding(key.WithKeys(keyJ, keyDown)),
		DashNextSection: key.NewBinding(key.WithKeys(keyShiftJ, keyShiftDown)),
		DashPrevSection: key.NewBinding(key.WithKeys(keyShiftK, keyShiftUp)),
		DashTop:         key.NewBinding(key.WithKeys(keyG)),
		DashBottom:      key.NewBinding(key.WithKeys(keyShiftG)),
		DashToggle:      key.NewBinding(key.WithKeys(keyE)),
		DashToggleAll:   key.NewBinding(key.WithKeys(keyShiftE)),
		DashJump:        key.NewBinding(key.WithKeys(keyEnter)),

		// Doc search
		DocSearchUp:      key.NewBinding(key.WithKeys(keyUp, keyCtrlP, keyCtrlK)),
		DocSearchDown:    key.NewBinding(key.WithKeys(keyDown, keyCtrlN, keyCtrlJ)),
		DocSearchConfirm: key.NewBinding(key.WithKeys(keyEnter)),
		DocSearchCancel:  key.NewBinding(key.WithKeys(keyEsc)),

		// Column finder
		ColFinderUp:        key.NewBinding(key.WithKeys(keyUp, keyCtrlP)),
		ColFinderDown:      key.NewBinding(key.WithKeys(keyDown, keyCtrlN)),
		ColFinderConfirm:   key.NewBinding(key.WithKeys(keyEnter)),
		ColFinderCancel:    key.NewBinding(key.WithKeys(keyEsc)),
		ColFinderClear:     key.NewBinding(key.WithKeys(keyCtrlU)),
		ColFinderBackspace: key.NewBinding(key.WithKeys(keyBackspace)),

		// Ops tree
		OpsUp:       key.NewBinding(key.WithKeys(keyK, keyUp)),
		OpsDown:     key.NewBinding(key.WithKeys(keyJ, keyDown)),
		OpsExpand:   key.NewBinding(key.WithKeys(keyEnter, keyL, keyRight)),
		OpsCollapse: key.NewBinding(key.WithKeys(keyH, keyLeft)),
		OpsTabNext:  key.NewBinding(key.WithKeys(keyF)),
		OpsTabPrev:  key.NewBinding(key.WithKeys(keyB)),
		OpsTop:      key.NewBinding(key.WithKeys(keyG)),
		OpsBottom:   key.NewBinding(key.WithKeys(keyShiftG)),
		OpsClose:    key.NewBinding(key.WithKeys(keyEsc)),

		// Extraction pipeline
		ExtCancel:     key.NewBinding(key.WithKeys(keyEsc)),
		ExtInterrupt:  key.NewBinding(key.WithKeys(keyCtrlC)),
		ExtUp:         key.NewBinding(key.WithKeys(keyK, keyUp)),
		ExtDown:       key.NewBinding(key.WithKeys(keyJ, keyDown)),
		ExtToggle:     key.NewBinding(key.WithKeys(keyEnter)),
		ExtRemodel:    key.NewBinding(key.WithKeys(keyR)),
		ExtToggleTSV:  key.NewBinding(key.WithKeys(keyT)),
		ExtAccept:     key.NewBinding(key.WithKeys(keyA)),
		ExtExplore:    key.NewBinding(key.WithKeys(keyX)),
		ExtBackground: key.NewBinding(key.WithKeys(keyCtrlB)),

		// Extraction explore
		ExploreUp:       key.NewBinding(key.WithKeys(keyK, keyUp)),
		ExploreDown:     key.NewBinding(key.WithKeys(keyJ, keyDown)),
		ExploreLeft:     key.NewBinding(key.WithKeys(keyH, keyLeft)),
		ExploreRight:    key.NewBinding(key.WithKeys(keyL, keyRight)),
		ExploreColStart: key.NewBinding(key.WithKeys(keyCaret)),
		ExploreColEnd:   key.NewBinding(key.WithKeys(keyDollar)),
		ExploreTop:      key.NewBinding(key.WithKeys(keyG)),
		ExploreBottom:   key.NewBinding(key.WithKeys(keyShiftG)),
		ExploreTabNext:  key.NewBinding(key.WithKeys(keyF)),
		ExploreTabPrev:  key.NewBinding(key.WithKeys(keyB)),
		ExploreAccept:   key.NewBinding(key.WithKeys(keyA)),
		ExploreExit:     key.NewBinding(key.WithKeys(keyEsc, keyX)),

		// Extraction model picker
		ExtModelUp:        key.NewBinding(key.WithKeys(keyUp, keyCtrlP)),
		ExtModelDown:      key.NewBinding(key.WithKeys(keyDown, keyCtrlN)),
		ExtModelConfirm:   key.NewBinding(key.WithKeys(keyEnter)),
		ExtModelCancel:    key.NewBinding(key.WithKeys(keyEsc)),
		ExtModelBackspace: key.NewBinding(key.WithKeys(keyBackspace)),

		// Help overlay
		HelpGotoTop:    key.NewBinding(key.WithKeys(keyG)),
		HelpGotoBottom: key.NewBinding(key.WithKeys(keyShiftG)),
		HelpClose:      key.NewBinding(key.WithKeys(keyEsc, keyQuestion)),

		// Confirmations
		ConfirmYes: key.NewBinding(key.WithKeys(keyY)),
		ConfirmNo:  key.NewBinding(key.WithKeys(keyN, keyEsc)),

		// Inline input
		InlineConfirm: key.NewBinding(key.WithKeys(keyEnter)),
		InlineCancel:  key.NewBinding(key.WithKeys(keyEsc)),
	}
}

// ShortHelp returns context-dependent key bindings for the status bar hint
// line. The returned slice varies by mode and current tab state.
func (m *Model) ShortHelp() []key.Binding {
	if m.mode == modeEdit {
		return m.editModeShortHelp()
	}
	return m.normalModeShortHelp()
}

func (m *Model) normalModeShortHelp() []key.Binding {
	// Help is high-priority — place it first so truncation drops
	// optional items before help.
	bindings := []key.Binding{m.keys.Help}

	// Context-dependent action: what enter does on the current column.
	if hint := m.enterHint(); hint != "" {
		bindings = append(bindings, key.NewBinding(
			key.WithKeys(keyEnter),
			key.WithHelp(symReturn, hint),
		))
	}

	bindings = append(bindings, m.keys.EnterEditMode)

	if m.effectiveTab().isDocumentTab() {
		bindings = append(bindings, m.keys.DocOpen, m.keys.DocSearch)
	}
	if m.llmClient != nil {
		bindings = append(bindings, m.keys.Chat)
	}

	if m.inDetail() {
		bindings = append(bindings, m.keys.Escape)
	}

	return bindings
}

func (m *Model) editModeShortHelp() []key.Binding {
	var bindings []key.Binding

	// Add: on document tabs show a/A, otherwise just a.
	if m.effectiveTab().isDocumentTab() {
		bindings = append(bindings, key.NewBinding(
			key.WithKeys(keyA, keyShiftA),
			key.WithHelp(keyA+"/"+keyShiftA, "add"),
		))
	} else {
		bindings = append(bindings, m.keys.Add)
	}

	// Edit: always show e/E with contextual hint.
	bindings = append(bindings,
		key.NewBinding(
			key.WithKeys(keyE, keyShiftE),
			key.WithHelp(keyE+"/"+keyShiftE, m.editHint()),
		),
		m.keys.Delete,
	)

	if m.effectiveTab().isDocumentTab() {
		bindings = append(bindings, m.keys.DocOpen, m.keys.ReExtract)
	}

	bindings = append(bindings, m.keys.ExitEdit)

	return bindings
}
