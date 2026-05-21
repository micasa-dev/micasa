// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"errors"
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// handleConfirmDiscard processes keys while the "discard unsaved changes?"
// prompt is active. Only y (discard) and n/esc (keep editing) are recognized.
func (m *Model) handleConfirmDiscard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.ConfirmYes):
		if m.confirm == confirmFormQuitDiscard {
			m.confirm = confirmNone
			if m.appCancel != nil {
				m.appCancel()
			}
			m.cancelChatOperations()
			m.cancelPull()
			resetPointerShape(m.pointerWriter, m.inTmux)
			return m, tea.Quit
		}
		m.confirm = confirmNone
		m.exitForm()
	case key.Matches(msg, m.keys.ConfirmNo):
		m.confirm = confirmNone
	}
	return m, nil
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
		return errors.New("no active tab")
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		return errors.New("nothing selected")
	}
	if meta.Deleted {
		return errors.New("cannot edit a deleted item")
	}
	return tab.Handler.StartEditForm(m, meta.ID)
}

func (m *Model) startCellOrFormEdit() error {
	tab := m.effectiveTab()
	if tab == nil {
		return errors.New("no active tab")
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		return errors.New("nothing selected")
	}
	if meta.Deleted {
		return errors.New("cannot edit a deleted item")
	}
	col := tab.ColCursor
	if col < 0 || col >= len(tab.Specs) {
		col = 0
	}
	spec := tab.Specs[col]

	// If the column is linked and the cell has a target ID, navigate cross-tab.
	if spec.Link != nil {
		if c, ok := m.selectedCell(col); ok && c.LinkID != "" {
			return m.navigateToLink(spec.Link, c.LinkID)
		}
	}

	if spec.Kind == cellReadonly || spec.Kind == cellDrilldown || spec.Kind == cellOps {
		return m.startEditForm()
	}
	return tab.Handler.InlineEdit(m, meta.ID, col)
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
		if handlerFormKind(tab) == formIncident {
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
	if !tab.showDeletedExplicit {
		tab.ShowDeleted = true
	}
	if handlerFormKind(tab) == formIncident {
		m.setStatusInfo("Resolved. Press d to reopen.")
	} else {
		m.setStatusInfo("Deleted. Press d to restore.")
	}
	m.surfaceError(m.reloadEffectiveTab())
}

// handlerFormKind returns the FormKind of a tab's handler, or formNone
// when the tab or its handler is nil. Prefer this over tab.Kind when
// identifying entity semantics, because drill-down tabs inherit the
// parent tab's Kind rather than the entity's kind.
func handlerFormKind(tab *Tab) FormKind {
	if tab == nil || tab.Handler == nil {
		return formNone
	}
	return tab.Handler.FormKind()
}

func (m *Model) promptHardDelete() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	kind := handlerFormKind(tab)
	switch kind {
	case formIncident, formMaintenance:
		// hard-deletable: continue
	case formNone, formHouse, formProject, formQuote, formAppliance,
		formServiceLog, formVendor, formDocument:
		return
	default:
		panic(fmt.Sprintf("unhandled FormKind: %d", kind))
	}
	meta, ok := m.selectedRowMeta()
	if !ok {
		m.setStatusError("Nothing selected.")
		return
	}
	if !meta.Deleted {
		if kind == formIncident {
			m.setStatusError("Resolve the incident first (d), then permanently delete (D).")
		} else {
			m.setStatusError("Delete the item first (d), then permanently delete (D).")
		}
		return
	}
	m.confirm = confirmHardDelete
	m.hardDeleteID = meta.ID
}

func (m *Model) handleConfirmHardDelete(msg tea.KeyPressMsg) {
	switch {
	case key.Matches(msg, m.keys.ConfirmYes):
		m.confirm = confirmNone
		tab := m.effectiveTab()
		var err error
		if handlerFormKind(tab) == formMaintenance {
			err = m.store.HardDeleteMaintenance(m.hardDeleteID)
		} else {
			err = m.store.HardDeleteIncident(m.hardDeleteID)
		}
		if err != nil {
			m.setStatusError(err.Error())
			return
		}
		m.setStatusInfo("Permanently deleted.")
		m.surfaceError(m.reloadEffectiveTab())
	case key.Matches(msg, m.keys.ConfirmNo):
		m.confirm = confirmNone
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
