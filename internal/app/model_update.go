// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = typed.IsDark()
		appIsDark = m.isDark
		appStyles = DefaultStyles(m.isDark)
		m.styles = appStyles
		return m, nil
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.resizeTables()
		m.updateAllViewports()
	case tea.KeyPressMsg:
		if typed.String() == keyCtrlQ {
			if m.mode == modeForm && m.fs.formDirty {
				m.confirm = confirmFormQuitDiscard
				return m, nil
			}
			if m.appCancel != nil {
				m.appCancel()
			}
			m.cancelChatOperations()
			m.cancelAllExtractions()
			m.cancelPull()
			if m.syncCancel != nil {
				m.syncCancel()
			}
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
	case extractionLLMPingMsg:
		return m, m.handleExtractionLLMPing(typed)
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
	case tea.MouseClickMsg:
		return m.handleMouseClick(typed)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(typed)
	case openFileResultMsg:
		if typed.Err != nil {
			m.setStatusError(fmt.Sprintf("open: %s", typed.Err))
		}
		return m, nil
	case syncDoneMsg:
		if typed.BlobErrs > 0 {
			m.setStatusError(fmt.Sprintf("sync: %d blob error(s)", typed.BlobErrs))
		}
		if typed.Conflicts > 0 {
			m.syncStatus = syncConflict
		} else {
			m.syncStatus = syncSynced
		}
		if typed.Pulled > 0 {
			if m.mode == modeForm {
				m.syncPendingReload = true
			} else {
				m.surfaceError(m.reloadAllTabs())
			}
		}
		return m, nil
	case syncErrorMsg:
		m.syncStatus = syncOffline
		m.setStatusError(fmt.Sprintf("sync: %s", typed.Err))
		return m, nil
	case syncTickMsg:
		if m.syncEngine == nil || m.syncStatus == syncSyncing {
			return m, syncTick()
		}
		m.syncStatus = syncSyncing
		return m, tea.Batch(doSync(m.syncCtx, m.syncEngine), syncTick())
	case syncDebounceMsg:
		if typed.gen != m.syncDebounceGen || m.syncEngine == nil || m.syncStatus == syncSyncing {
			return m, nil
		}
		m.syncStatus = syncSyncing
		return m, doSync(m.syncCtx, m.syncEngine)
	case postalCodeLookupMsg:
		if typed.Err != nil {
			m.setStatusError(fmt.Sprintf("postal code lookup: %v", typed.Err))
			return m, nil
		}
		values, ok := m.fs.formData.(*houseFormData)
		if !ok {
			return m, nil
		}
		cityAutoFilled := values.City == "" || values.City == m.fs.autoFilledCity
		stateAutoFilled := values.State == "" || values.State == m.fs.autoFilledState
		// No results: clear previously auto-filled values (e.g., user
		// edited the postal code to an invalid prefix).
		if typed.City == "" && typed.State == "" {
			if cityAutoFilled && m.fs.autoFilledCity != "" {
				values.City = ""
				m.fs.autoFilledCity = ""
				if m.fs.cityInput != nil {
					m.fs.cityInput.Value(&values.City)
				}
			}
			if stateAutoFilled && m.fs.autoFilledState != "" {
				values.State = ""
				m.fs.autoFilledState = ""
				if m.fs.stateInput != nil {
					m.fs.stateInput.Value(&values.State)
				}
			}
			return m, nil
		}
		// Overwrite city/state if they're empty or were set by a previous
		// autofill (not manually typed by the user).
		if cityAutoFilled && typed.City != "" {
			values.City = typed.City
			m.fs.autoFilledCity = typed.City
			if m.fs.cityInput != nil {
				m.fs.cityInput.Value(&values.City)
			}
		}
		if stateAutoFilled && typed.State != "" {
			values.State = typed.State
			m.fs.autoFilledState = typed.State
			if m.fs.stateInput != nil {
				m.fs.stateInput.Value(&values.State)
			}
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
	case tea.KeyPressMsg:
		if m.confirm == confirmHardDelete {
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
	if m.confirm.isFormConfirm() {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			return m.handleConfirmDiscard(keyMsg)
		}
		return m, nil
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == keyCtrlS {
		return m, m.saveFormInPlace()
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == keyCtrlE &&
		m.fs.notesEditMode {
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
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == keyShiftH {
		if field := m.fs.form.GetFocusedField(); field != nil {
			if fp, ok := field.(*huh.FilePicker); ok {
				current := filePickerShowHidden(fp)
				newVal := !current
				fp.ShowHidden(newVal)
				// Reset cursor to top so it doesn't point past the new list.
				goToTop := tea.KeyPressMsg{Code: 'g', Text: "g"}
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
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if n, isOrdinal := selectOrdinal(keyMsg); isOrdinal && isSelectField(m.fs.form) {
			m.jumpSelectToOrdinal(n)
			return m, nil
		}
	}
	// Intercept ESC on dirty forms to confirm before discarding.
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == keyEsc {
		mandatoryHouse := m.fs.formKind() == formHouse && !m.hasHouse
		if m.fs.formDirty && !mandatoryHouse {
			m.confirm = confirmFormDiscard
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
	// Postal code autofill: watch for the postal code value to stabilize
	// at >= 3 characters. Once it does, dispatch a single lookup. We track
	// the last-seen value to avoid re-dispatching on every keystroke.
	if m.fs.postalCodeField != nil && m.addressAutofill {
		if values, ok := m.fs.formData.(*houseFormData); ok {
			pc := values.PostalCode
			if len(pc) >= postalCodeMinLength && pc != m.fs.lastPostalCode {
				m.fs.lastPostalCode = pc
				cmd = tea.Batch(cmd, lookupPostalCodeCmd(
					m.lifecycleCtx(), m.addressClient, m.addressBaseURL,
					m.addressCountry, pc,
				))
			}
		}
	}
	switch m.fs.form.State {
	case huh.StateCompleted:
		return m, m.saveForm()
	case huh.StateAborted:
		if m.fs.formKind() == formHouse && !m.hasHouse {
			m.setStatusError("House profile required.")
			m.startHouseForm()
			return m, m.formInitCmd()
		}
		m.exitForm()
	default:
	}
	return m, cmd
}

func (m *Model) formInitCmd() tea.Cmd {
	cmd := m.fs.pendingFormInit
	m.fs.pendingFormInit = nil
	return cmd
}
