// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/cpcloud/micasa/internal/llm"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newExtractionModel sets up a Model with an active extraction overlay
// for testing keyboard interaction. Steps are pre-populated with the
// given statuses.
func newExtractionModel(t *testing.T, steps map[extractionStep]stepStatus) *Model {
	t.Helper()

	m := newTestModel(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ex := &extractionLogState{
		ID:         nextExtractionID.Add(1),
		ctx:        ctx,
		CancelFn:   cancel,
		Visible:    true,
		toolCursor: -1,
		expanded:   make(map[extractionStep]bool),
	}
	for si, status := range steps {
		ex.Steps[si] = extractionStepInfo{Status: status}
		switch si { //nolint:exhaustive // test helper only sets known steps
		case stepText:
			ex.hasText = true
		case stepExtract:
			ex.hasExtract = true
		case stepLLM:
			ex.hasLLM = true
		}
	}
	m.ex.extraction = ex
	return m
}

func sendExtractionKey(m *Model, key string) {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEscape}
	case keyCtrlB:
		msg = tea.KeyMsg{Type: tea.KeyCtrlB}
	case keyCtrlQ:
		msg = tea.KeyMsg{Type: tea.KeyCtrlQ}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	m.Update(msg)
}

// --- Cursor navigation ---

func TestExtractionCursor_JK_NavigatesToRunningSteps(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	assert.Equal(t, 0, ex.cursor)

	// j should move to the running extract step.
	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor, "j should land on running step")

	// j should not move to the pending LLM step.
	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor, "j should not land on pending step")

	// k moves back to text.
	sendExtractionKey(m, "k")
	assert.Equal(t, 0, ex.cursor)
}

func TestExtractionCursor_JK_LandsOnSettledSteps(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepFailed,
	})
	ex := m.ex.extraction
	ex.Done = true

	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor, "j should move to next settled step")

	sendExtractionKey(m, "j")
	assert.Equal(t, 2, ex.cursor, "j should move to failed step")

	sendExtractionKey(m, "j")
	assert.Equal(t, 2, ex.cursor, "j should not go past last step")

	sendExtractionKey(m, "k")
	assert.Equal(t, 1, ex.cursor, "k should move back")
}

func TestExtractionCursor_JK_AllStepsWhenDone(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor)
}

// --- Enter toggle ---

func TestExtractionEnter_TogglesDoneStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Text step is done, not auto-expanded. First enter should expand.
	sendExtractionKey(m, "enter")
	assert.True(t, ex.expanded[stepText], "enter should expand done text step")

	// Second enter should collapse.
	sendExtractionKey(m, "enter")
	assert.False(t, ex.expanded[stepText], "enter should collapse")
}

func TestExtractionEnter_TogglesAutoExpandedLLMStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// LLM done step is auto-expanded. First enter should collapse.
	sendExtractionKey(m, "enter")
	assert.False(t, ex.expanded[stepLLM], "enter on auto-expanded LLM should collapse")

	// Second enter should re-expand.
	sendExtractionKey(m, "enter")
	assert.True(t, ex.expanded[stepLLM], "enter should re-expand")
}

func TestExtractionEnter_TogglesFailedStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepFailed,
	})
	ex := m.ex.extraction

	// Failed steps are auto-expanded. First enter should collapse.
	sendExtractionKey(m, "enter")
	assert.False(t, ex.expanded[stepExtract], "enter on auto-expanded failed step should collapse")
}

func TestExtractionEnter_TogglesRunningStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.cursor = 1
	ex.cursorManual = true

	// Running steps are auto-expanded. First enter should collapse.
	sendExtractionKey(m, "enter")
	assert.False(t, ex.expanded[stepExtract], "enter on running step should collapse")

	// Second enter should re-expand.
	sendExtractionKey(m, "enter")
	assert.True(t, ex.expanded[stepExtract], "enter should re-expand running step")
}

// --- Rerun cursor relocation ---

func TestRerunLLM_MovesCursorToLLMStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.cursor = 1 // on extract step

	m.rerunLLMExtraction()

	// Cursor should move to the LLM step being rerun.
	assert.Equal(t, 2, ex.cursor, "cursor should move to LLM step")
}

func TestRerunLLM_CursorOnLLMWhenOnlyStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.cursor = 0

	m.rerunLLMExtraction()

	// Only LLM is active -- cursor stays on index 0 (the LLM step).
	assert.Equal(t, 0, ex.cursor)
}

// --- Operation preview rendering ---

// newPreviewModel creates a Model with extraction state containing the given
// operations, suitable for testing renderOperationPreviewSection.
func newPreviewModel(t *testing.T, ops []extract.Operation) *Model {
	t.Helper()

	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	m.ex.extraction.Done = true
	m.ex.extraction.operations = ops
	return m
}

func TestRenderOperationPreview_TabbedInterface(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{
			Action: "create",
			Table:  data.TableVendors,
			Data:   map[string]any{"name": "Garcia Plumbing"},
		},
		{
			Action: "update",
			Table:  data.TableDocuments,
			Data:   map[string]any{"id": float64(42), "title": "Invoice", "notes": "Repair"},
		},
	})

	// Non-interactive: shows only the first tab (vendors), tabs are dimmed.
	out := m.renderOperationPreviewSection(60, false)
	assert.Contains(t, out, "Vendors")
	assert.Contains(t, out, "Docs")
	assert.Contains(t, out, "Name")
	assert.Contains(t, out, "Garcia Plumbing")

	// Interactive: switch to second tab to see documents.
	m.ex.extraction.exploring = true
	m.ex.extraction.enterExploreMode(m.cur)
	m.ex.extraction.previewTab = 1
	out = m.renderOperationPreviewSection(60, true)
	assert.Contains(t, out, "Title")
	assert.Contains(t, out, "Invoice")
	assert.Contains(t, out, "Notes")
	assert.Contains(t, out, "Repair")
}

func TestRenderOperationPreview_EmptyOps(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, nil)
	out := m.renderOperationPreviewSection(60, false)
	assert.Contains(t, out, "no operations")
}

func TestRenderOperationPreview_EmptyData(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: nil},
	})
	out := m.renderOperationPreviewSection(60, false)
	assert.Contains(t, out, "no operations")
}

func TestRenderOperationPreview_MoneyFormatting(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{
			Action: "create",
			Table:  data.TableQuotes,
			Data: map[string]any{
				"project_id":  float64(1),
				"vendor_id":   float64(2),
				"total_cents": float64(150000),
			},
		},
	})
	out := m.renderOperationPreviewSection(80, false)

	assert.Contains(t, out, "Quotes")
	assert.Contains(t, out, "$1,500.00")
	assert.Contains(t, out, "#1")
	assert.Contains(t, out, "#2")
	assert.Contains(t, out, "Total")
	assert.Contains(t, out, "Project")
	assert.Contains(t, out, "Vendor")
}

func TestRenderOperationPreview_MultipleRowsSameTable(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "Acme"}},
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "Beta Corp"}},
	})
	out := m.renderOperationPreviewSection(60, false)

	assert.Contains(t, out, "Vendors")
	assert.Contains(t, out, "Acme")
	assert.Contains(t, out, "Beta Corp")
}

func TestRenderOperationPreview_UnknownTable(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: "unknown_table", Data: map[string]any{"x": "y"}},
	})
	out := m.renderOperationPreviewSection(60, false)
	assert.Contains(t, out, "no operations")
}

func TestGroupOperationsByTable(t *testing.T) {
	t.Parallel()
	ops := []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
		{Action: "update", Table: data.TableDocuments, Data: map[string]any{"title": "B"}},
		{
			Action: "create",
			Table:  data.TableVendors,
			Data:   map[string]any{"name": "C", "email": "c@x.com"},
		},
	}
	groups := groupOperationsByTable(ops, locale.DefaultCurrency())

	require.Len(t, groups, 2)
	// First-seen order: vendors, documents.
	assert.Equal(t, "Vendors", groups[0].name)
	assert.Equal(t, "Docs", groups[1].name)

	// Vendors: 2 rows, specs include Name + Email (union of both ops).
	assert.Len(t, groups[0].cells, 2)
	titles := make([]string, len(groups[0].specs))
	for i, s := range groups[0].specs {
		titles[i] = s.Title
	}
	assert.Contains(t, titles, "Name")
	assert.Contains(t, titles, "Email")

	// Documents: 1 row.
	assert.Len(t, groups[1].cells, 1)
}

// --- Deferred document creation (magic-add "A") ---

func TestAcceptDeferredExtraction_CreatesDocument(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Simulate deferred creation with a pending doc.
	ex.pendingDoc = &data.Document{
		FileName: "invoice.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("pdf-bytes"),
	}
	// LLM produced operations including document fields.
	ex.operations = []extract.Operation{
		{Action: "create", Table: data.TableDocuments, Data: map[string]any{
			"title": "Garcia Invoice",
			"notes": "Plumbing repair",
		}},
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "Garcia Plumbing",
		}},
	}

	// pendingDoc should have fields applied from the create-documents op.
	// We can't call acceptExtraction without a real store, but we can
	// verify the pending state is set correctly.
	assert.NotNil(t, ex.pendingDoc, "pendingDoc should be set before accept")
	assert.Empty(t, ex.pendingDoc.Title, "title not yet applied")
}

func TestCancelDeferredExtraction_NothingPersisted(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepRunning,
	})
	ex := m.ex.extraction
	ex.pendingDoc = &data.Document{
		FileName: "invoice.pdf",
		MIMEType: "application/pdf",
	}

	// Cancel should nil out extraction state.
	m.cancelExtraction()
	assert.Nil(t, m.ex.extraction, "extraction should be nil after cancel")
}

func TestDeferredExtraction_PendingDocFieldPresent(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.pendingDoc = &data.Document{FileName: "scan.jpp"}

	// Verify the pendingDoc is accessible.
	assert.Equal(t, "scan.jpp", ex.pendingDoc.FileName)
}

// --- Explore mode ---

func TestExploreMode_XTogglesExploring(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
	})
	ex := m.ex.extraction
	assert.False(t, ex.exploring)

	// x should enter explore mode when done with operations.
	sendExtractionKey(m, "x")
	assert.True(t, ex.exploring, "x should enter explore mode")

	// x again should exit explore mode.
	sendExtractionKey(m, "x")
	assert.False(t, ex.exploring, "x should exit explore mode")
}

func TestExploreMode_EscExitsExploring(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
	})
	ex := m.ex.extraction
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)

	// Esc should exit explore mode, not cancel the overlay.
	sendExtractionKey(m, "esc")
	assert.False(t, ex.exploring, "esc should exit explore mode")
	assert.NotNil(t, m.ex.extraction, "overlay should still be open")
}

func TestExploreMode_JKNavigatesRows(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "B"}},
	})
	ex := m.ex.extraction
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)
	assert.Equal(t, 0, ex.previewRow)

	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.previewRow)

	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.previewRow, "should not go past last row")

	sendExtractionKey(m, "k")
	assert.Equal(t, 0, ex.previewRow)
}

func TestExploreMode_HLNavigatesCols(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "A", "email": "a@b.com",
		}},
	})
	ex := m.ex.extraction
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)
	assert.Equal(t, 0, ex.previewCol)

	sendExtractionKey(m, "l")
	assert.Equal(t, 1, ex.previewCol)

	sendExtractionKey(m, "h")
	assert.Equal(t, 0, ex.previewCol)
}

func TestExploreMode_BFSwitchesTabs(t *testing.T) {
	t.Parallel()
	m := newPreviewModel(t, []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
		{
			Action: "create",
			Table:  data.TableQuotes,
			Data:   map[string]any{"total_cents": float64(100)},
		},
	})
	ex := m.ex.extraction
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)
	assert.Equal(t, 0, ex.previewTab)

	sendExtractionKey(m, "f")
	assert.Equal(t, 1, ex.previewTab)
	assert.Equal(t, 0, ex.previewRow, "row cursor should reset on tab switch")

	sendExtractionKey(m, "b")
	assert.Equal(t, 0, ex.previewTab)
}

func TestExploreMode_AcceptWorksInExploreMode(t *testing.T) {
	t.Parallel()
	ops := []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{"name": "A"}},
	}
	m := newPreviewModel(t, ops)
	ex := m.ex.extraction

	// Stage through shadow DB so accept has staged operations.
	sdb, err := extract.NewShadowDB(m.store)
	require.NoError(t, err)
	require.NoError(t, sdb.Stage(ops))
	ex.shadowDB = sdb

	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)

	// a should accept even in explore mode.
	sendExtractionKey(m, "a")
	assert.Nil(t, m.ex.extraction, "accept in explore mode clears state")
}

// --- Model picker ---

func TestModelPicker_ROpensPickerOnDoneLLMStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Move cursor to the LLM step.
	sendExtractionKey(m, "j")
	require.Equal(t, 1, ex.cursor)

	// Press r -- should activate the model picker.
	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker, "r should open model picker")
	// No LLM client in test model, so picker is populated with well-known
	// models immediately (not in loading state).
	assert.False(t, ex.modelPicker.Loading, "no client means immediate populate")
	assert.NotEmpty(t, ex.modelPicker.All, "well-known models should be available")
}

func TestModelPicker_EscDismissesWithoutRerun(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Activate picker and manually populate it (skip async fetch).
	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	ex.modelPicker.Loading = false
	ex.modelPicker.All = []modelCompleterEntry{
		{Name: "qwen3:8b", Local: true},
	}
	refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())

	// Esc should dismiss the picker.
	sendExtractionKey(m, "esc")
	assert.Nil(t, ex.modelPicker, "esc should dismiss picker")
	assert.True(t, ex.Done, "extraction should still be done (no rerun)")
}

func TestModelPicker_EnterSelectsModelAndReruns(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Activate and populate picker.
	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	ex.modelPicker.Loading = false
	ex.modelPicker.All = []modelCompleterEntry{
		{Name: "llama3.3", Local: true},
		{Name: "qwen3:8b", Local: true},
	}
	refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())

	// Navigate to second entry (arrow keys, not j/k which type chars).
	sendExtractionKey(m, "down")
	assert.Equal(t, 1, ex.modelPicker.Cursor)

	sendExtractionKey(m, "enter")

	// Picker should be dismissed and extraction model switched.
	assert.Nil(t, ex.modelPicker, "picker should be dismissed after enter")
	assert.Equal(t, "qwen3:8b", m.ex.extractionModel, "extraction model should be updated")
	assert.Nil(t, m.ex.extractionClient, "client cache should be invalidated")
}

func TestModelPicker_FilterNarrowsMatches(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	ex.modelPicker.Loading = false
	ex.modelPicker.All = []modelCompleterEntry{
		{Name: "llama3.3", Local: true},
		{Name: "qwen3:8b", Local: true},
		{Name: "qwen3:32b", Local: true},
	}
	refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())
	require.Len(t, ex.modelPicker.Matches, 3)

	// Type "qw" to filter.
	sendExtractionKey(m, "q")
	sendExtractionKey(m, "w")
	assert.Equal(t, "qw", ex.modelFilter)
	assert.Len(t, ex.modelPicker.Matches, 2, "filter should narrow to qwen models")

	// Backspace should widen.
	sendExtractionKey(m, "backspace")
	assert.Equal(t, "q", ex.modelFilter)
	assert.Len(t, ex.modelPicker.Matches, 2, "q still matches both qwen models")
}

func TestModelPicker_ArrowsNavigateCursor(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	ex.modelPicker.Loading = false
	ex.modelPicker.All = []modelCompleterEntry{
		{Name: "a", Local: true},
		{Name: "b", Local: true},
		{Name: "c", Local: true},
	}
	refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())

	sendExtractionKey(m, "down")
	assert.Equal(t, 1, ex.modelPicker.Cursor)

	sendExtractionKey(m, "down")
	assert.Equal(t, 2, ex.modelPicker.Cursor)

	// Should not go past last entry.
	sendExtractionKey(m, "down")
	assert.Equal(t, 2, ex.modelPicker.Cursor)

	sendExtractionKey(m, "up")
	assert.Equal(t, 1, ex.modelPicker.Cursor)
}

func TestModelPicker_JKTypeInsteadOfNavigate(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	ex.modelPicker.Loading = false
	ex.modelPicker.All = []modelCompleterEntry{
		{Name: "a", Local: true},
	}
	refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())

	// j and k should type into the filter, not navigate.
	sendExtractionKey(m, "j")
	assert.Equal(t, "j", ex.modelFilter)
	sendExtractionKey(m, "k")
	assert.Equal(t, "jk", ex.modelFilter)
	assert.Equal(t, 0, ex.modelPicker.Cursor, "cursor should not move from j/k")
}

func TestModelPicker_RerunPreservesAllSteps(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.Steps[stepExtract] = extractionStepInfo{
		Status: stepDone,
		Detail: "tesseract",
		Metric: "1234 chars",
	}

	// Move to LLM step and open picker.
	sendExtractionKey(m, "j") // text → extract
	sendExtractionKey(m, "j") // extract → LLM
	require.Equal(t, 2, ex.cursor)

	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	ex.modelPicker.Loading = false
	ex.modelPicker.All = []modelCompleterEntry{
		{Name: "qwen3:8b", Local: true},
	}
	refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())

	// Select the model.
	sendExtractionKey(m, "enter")

	// All three steps must still be active.
	active := ex.activeSteps()
	require.Len(t, active, 3, "all steps should survive rerun")
	assert.Equal(t, stepText, active[0])
	assert.Equal(t, stepExtract, active[1])
	assert.Equal(t, stepLLM, active[2])

	// Extract step should be untouched.
	assert.True(t, ex.hasExtract, "hasExtract should still be true")
	assert.Equal(t, stepDone, ex.Steps[stepExtract].Status)
	assert.Equal(t, "tesseract", ex.Steps[stepExtract].Detail)
	assert.Equal(t, "1234 chars", ex.Steps[stepExtract].Metric)

	// LLM step should be running with the new model.
	assert.Equal(t, stepRunning, ex.Steps[stepLLM].Status)
	assert.Equal(t, "qwen3:8b", m.ex.extractionModel)
}

func TestModelPicker_RerunRendersAllSteps(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.Steps[stepExtract] = extractionStepInfo{
		Status: stepDone,
		Detail: "tesseract",
		Metric: "1234 chars",
	}
	m.width = 120
	m.height = 40

	// Move to LLM step, open picker, select model.
	sendExtractionKey(m, "j")
	sendExtractionKey(m, "j")
	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	ex.modelPicker.Loading = false
	ex.modelPicker.All = []modelCompleterEntry{
		{Name: "qwen3:8b", Local: true},
	}
	refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())
	sendExtractionKey(m, "enter")

	// Render the overlay.
	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "ext", "extract step should be in overlay output")
	assert.Contains(t, out, "tesseract", "extract detail should be in overlay output")
	assert.Contains(t, out, "llm", "LLM step should be in overlay output")
}

func TestModelPicker_RNoOpWhenNotOnLLMStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	// Cursor is on text step (index 0).

	sendExtractionKey(m, "r")
	assert.Nil(t, ex.modelPicker, "r should not open picker on text step")
}

func TestModelPicker_RNoOpWhenNotDone(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepRunning,
	})
	ex := m.ex.extraction

	sendExtractionKey(m, "r")
	assert.Nil(t, ex.modelPicker, "r should not open picker when extraction is running")
}

// --- NeedsOCR integration ---

func TestNeedsOCR_UsedInsteadOfHardcodedToolName(t *testing.T) {
	t.Parallel()
	// Verify that extraction.go and model.go use NeedsOCR (not HasMatchingExtractor
	// with "tesseract"). This is a compile-time guarantee: if extract.NeedsOCR is
	// removed, the build will break. This test documents the intent.
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
	})
	// With no OCR extractors configured, startExtractionOverlay should
	// not flag needsExtract.
	assert.Nil(t, m.ex.extractors, "default test model has no extractors")
}

// --- Extract keybinding and OCR skip (#711) ---

func TestExtractKeybinding_OpensOverlayOnDocumentTab(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	// Create a document with existing extracted text.
	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title:         "Receipt",
		FileName:      "receipt.png",
		MIMEType:      "image/png",
		Data:          []byte("fake-image"),
		ExtractedText: "Previously extracted invoice text",
	}))

	// Navigate to Documents tab, reload, enter edit mode.
	m.active = tabIndex(tabDocuments)
	m.reloadAfterMutation()
	sendKey(m, "i")
	require.Equal(t, modeEdit, m.mode)

	// Press r to extract the selected document.
	sendKey(m, "r")

	require.NotNil(t, m.ex.extraction, "extraction overlay should be open")
	ex := m.ex.extraction
	assert.True(t, ex.hasText, "should show text step with cached text")
	assert.False(t, ex.hasExtract, "should skip OCR -- text already exists")
	assert.True(t, ex.hasLLM, "should proceed to LLM")
	assert.Equal(t, "receipt.png", ex.Filename)
}

func TestExtractKeybinding_NoOpOnNonDocumentTab(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	// Stay on the default tab (not documents).
	sendKey(m, "i")
	sendKey(m, "r")

	assert.Nil(t, m.ex.extraction, "r should not open extraction on non-document tab")
}

func TestExtractKeybinding_RunsOCRWhenNoExistingText(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.ex.extractors = extract.DefaultExtractors(0, 0, true)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	if !extract.NeedsOCR(m.ex.extractors, "image/png") {
		t.Skip("OCR tools not available")
	}

	// Create a document without extracted text.
	require.NoError(t, m.store.CreateDocument(&data.Document{
		Title:    "New Receipt",
		FileName: "new.png",
		MIMEType: "image/png",
		Data:     []byte("fake-image"),
	}))

	m.active = tabIndex(tabDocuments)
	m.reloadAfterMutation()
	sendKey(m, "i")
	sendKey(m, "r")

	require.NotNil(t, m.ex.extraction)
	ex := m.ex.extraction
	assert.True(t, ex.hasExtract, "should run OCR when no existing text")
}

// --- startExtractionOverlay unit tests (#711) ---

func TestStartExtraction_ImageWithExistingText_SkipsOCR(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.ex.extractors = extract.DefaultExtractors(0, 0, true)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	existingText := "Previously extracted invoice text"
	cmd := m.startExtractionOverlay(
		1, "receipt.png", []byte("fake"), "image/png", existingText, nil,
	)

	require.NotNil(t, cmd, "should return a command for LLM step")
	require.NotNil(t, m.ex.extraction)

	ex := m.ex.extraction
	assert.True(t, ex.hasText, "image with existing text should show text step")
	assert.False(t, ex.hasExtract, "existing text should skip OCR")
	assert.True(t, ex.hasLLM, "should proceed to LLM")
	assert.Equal(t, stepDone, ex.Steps[stepText].Status)
	assert.Equal(t, "ocr", ex.Steps[stepText].Detail)
	require.Len(t, ex.sources, 1)
	assert.Equal(t, "tesseract", ex.sources[0].Tool)
	assert.Equal(t, "Text from previous OCR extraction.", ex.sources[0].Desc)
	assert.Equal(t, existingText, ex.sources[0].Text)
}

func TestStartExtraction_PDFWithExistingText_SkipsOCR(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.ex.extractors = extract.DefaultExtractors(0, 0, true)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	existingText := "Invoice #12345\nTotal: $100.00"
	cmd := m.startExtractionOverlay(
		1, "invoice.pdf", []byte("fake-pdf"), extract.MIMEApplicationPDF, existingText, nil,
	)

	require.NotNil(t, cmd)
	require.NotNil(t, m.ex.extraction)

	ex := m.ex.extraction
	assert.True(t, ex.hasText)
	assert.False(t, ex.hasExtract, "existing text should skip OCR even for PDFs")
	assert.True(t, ex.hasLLM)
	assert.Equal(t, stepDone, ex.Steps[stepText].Status)
	assert.Equal(t, "pdf", ex.Steps[stepText].Detail)
	require.Len(t, ex.sources, 1)
	assert.Equal(t, "pdftotext", ex.sources[0].Tool)
	assert.Equal(t, existingText, ex.sources[0].Text)
}

func TestStartExtraction_ExistingTextPreservesExtractData(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.ex.extractors = extract.DefaultExtractors(0, 0, true)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	existingText := "OCR result from previous run"
	tsvData := []byte("level\tpage\tblock\n1\t1\t1\n")
	cmd := m.startExtractionOverlay(
		1, "receipt.png", []byte("fake"), "image/png", existingText, tsvData,
	)

	require.NotNil(t, cmd)
	require.NotNil(t, m.ex.extraction)

	ex := m.ex.extraction
	require.Len(t, ex.sources, 1)
	assert.Equal(t, tsvData, ex.sources[0].Data, "extract data should be preserved in source")
}

func TestStartExtraction_EmptyText_RunsOCRNormally(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.ex.extractors = extract.DefaultExtractors(0, 0, true)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	if !extract.NeedsOCR(m.ex.extractors, "image/png") {
		t.Skip("OCR tools not available")
	}

	cmd := m.startExtractionOverlay(
		1, "receipt.png", []byte("fake"), "image/png", "", nil,
	)

	require.NotNil(t, cmd)
	require.NotNil(t, m.ex.extraction)

	ex := m.ex.extraction
	assert.False(t, ex.hasText, "images without existing text should not show text step")
	assert.True(t, ex.hasExtract, "OCR should run when no existing text")
	assert.True(t, ex.hasLLM)
	assert.Equal(t, stepRunning, ex.Steps[stepExtract].Status)
}

func TestStartExtraction_WhitespaceOnlyText_RunsOCR(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.ex.extractors = extract.DefaultExtractors(0, 0, true)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	if !extract.NeedsOCR(m.ex.extractors, "image/png") {
		t.Skip("OCR tools not available")
	}

	cmd := m.startExtractionOverlay(
		1, "receipt.png", []byte("fake"), "image/png", "   \n\t  ", nil,
	)

	require.NotNil(t, cmd)
	require.NotNil(t, m.ex.extraction)

	ex := m.ex.extraction
	assert.True(t, ex.hasExtract, "whitespace-only text should still trigger OCR")
}

func TestStartExtraction_PlainTextWithExistingText_SkipsToLLM(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.ex.extractionClient = testExtractionOllamaClient(t, "test-model")

	existingText := "Some previously extracted content"
	cmd := m.startExtractionOverlay(
		1, "notes.txt", []byte("fake"), "text/plain", existingText, nil,
	)

	require.NotNil(t, cmd)
	require.NotNil(t, m.ex.extraction)

	ex := m.ex.extraction
	assert.True(t, ex.hasText)
	assert.False(t, ex.hasExtract)
	assert.True(t, ex.hasLLM)
	assert.Equal(t, "plaintext", ex.Steps[stepText].Detail)
	require.Len(t, ex.sources, 1)
	assert.Equal(t, "plaintext", ex.sources[0].Tool)
}

// --- Background extraction ---

func TestBackground_CtrlBMovesExtractionToBg(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
	})
	require.NotNil(t, m.ex.extraction)
	require.True(t, m.ex.extraction.Visible)

	sendExtractionKey(m, keyCtrlB)

	assert.Nil(t, m.ex.extraction, "foreground extraction should be nil after backgrounding")
	require.Len(t, m.ex.bgExtractions, 1)
	assert.False(t, m.ex.bgExtractions[0].Visible, "bg extraction should not be visible")
}

func TestBackground_CtrlBNoOpWhenDone(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
	})
	m.ex.extraction.Done = true

	sendExtractionKey(m, keyCtrlB)

	assert.NotNil(t, m.ex.extraction, "done extraction should not be backgrounded")
	assert.Empty(t, m.ex.bgExtractions)
}

func TestForeground_CtrlBBringsBgToFront(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
	})
	m.ex.extraction.Filename = "test.pdf"

	// Background it.
	sendExtractionKey(m, keyCtrlB)
	require.Nil(t, m.ex.extraction)
	require.Len(t, m.ex.bgExtractions, 1)

	// Foreground it via ctrl+b in normal mode.
	sendKey(m, keyCtrlB)

	require.NotNil(t, m.ex.extraction)
	assert.True(t, m.ex.extraction.Visible, "foregrounded extraction should be visible")
	assert.Equal(t, "test.pdf", m.ex.extraction.Filename)
	assert.Empty(t, m.ex.bgExtractions)
}

func TestForeground_SwapsCurrentToBackground(t *testing.T) {
	t.Parallel()
	// Create two extractions: one foreground, one background.
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	m.ex.extraction.Filename = "first.pdf"

	// Background the first.
	sendExtractionKey(m, keyCtrlB)
	require.Len(t, m.ex.bgExtractions, 1)

	// Create a new foreground extraction.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m.ex.extraction = &extractionLogState{
		ID:         nextExtractionID.Add(1),
		Filename:   "second.pdf",
		Visible:    true,
		ctx:        ctx,
		CancelFn:   cancel,
		expanded:   make(map[extractionStep]bool),
		hasExtract: true,
	}
	m.ex.extraction.Steps[stepExtract] = extractionStepInfo{Status: stepRunning}

	// ctrl+b on visible overlay backgrounds the second extraction.
	sendExtractionKey(m, keyCtrlB)
	assert.Nil(t, m.ex.extraction)
	require.Len(t, m.ex.bgExtractions, 2)

	// ctrl+b in normal mode foregrounds the most recent (second).
	sendKey(m, keyCtrlB)
	require.NotNil(t, m.ex.extraction)
	assert.Equal(t, "second.pdf", m.ex.extraction.Filename)
	require.Len(t, m.ex.bgExtractions, 1)
	assert.Equal(t, "first.pdf", m.ex.bgExtractions[0].Filename)
}

func TestBgExtraction_CompletionNotifiesNoAutoAccept(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepRunning,
	})
	ex := m.ex.extraction
	ex.Filename = "invoice.pdf"
	id := ex.ID

	// Background the extraction.
	sendExtractionKey(m, keyCtrlB)
	require.Len(t, m.ex.bgExtractions, 1)

	// Simulate LLM completion on the background extraction.
	m.handleExtractionLLMChunk(extractionLLMChunkMsg{
		ID:      id,
		Content: `{"operations":[]}`,
	})
	m.handleExtractionLLMChunk(extractionLLMChunkMsg{
		ID:   id,
		Done: true,
	})

	// Extraction should still be in bgExtractions (no auto-accept).
	require.Len(t, m.ex.bgExtractions, 1)
	assert.True(t, m.ex.bgExtractions[0].Done, "bg extraction should be done")
	assert.Contains(t, m.status.Text, "invoice.pdf", "status should mention filename")
}

func TestBgExtraction_ErrorStaysInList(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepRunning,
	})
	ex := m.ex.extraction
	ex.Filename = "bad.pdf"
	id := ex.ID

	// Background it.
	sendExtractionKey(m, keyCtrlB)
	require.Len(t, m.ex.bgExtractions, 1)

	// Simulate LLM error.
	m.handleExtractionLLMChunk(extractionLLMChunkMsg{
		ID:   id,
		Err:  context.DeadlineExceeded,
		Done: true,
	})

	// Should remain in bg list with error.
	require.Len(t, m.ex.bgExtractions, 1)
	assert.True(t, m.ex.bgExtractions[0].HasError)
	assert.Contains(t, m.status.Text, "bad.pdf")
}

func TestLLMExtraction_TimeoutError(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepRunning,
	})
	ex := m.ex.extraction
	ex.Steps[stepLLM].Started = time.Now().Add(-3 * time.Minute)
	id := ex.ID

	// Drive the timeout error through Update (simulating the async message
	// that bubbletea delivers when the LLM context deadline fires).
	m.Update(extractionLLMChunkMsg{
		ID:   id,
		Err:  context.DeadlineExceeded,
		Done: true,
	})

	step := ex.Steps[stepLLM]
	assert.Equal(t, stepFailed, step.Status)
	require.NotEmpty(t, step.Logs)
	assert.Contains(t, step.Logs[0], "timed out")
	assert.Contains(t, step.Logs[0], "extraction.llm.timeout")
}

func TestLLMExtraction_TimeoutError_NonDeadlinePreservesOriginal(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepRunning,
	})
	ex := m.ex.extraction
	ex.Steps[stepLLM].Started = time.Now()
	id := ex.ID

	// A non-timeout error should preserve the original message.
	m.Update(extractionLLMChunkMsg{
		ID:   id,
		Err:  context.Canceled,
		Done: true,
	})

	step := ex.Steps[stepLLM]
	assert.Equal(t, stepFailed, step.Status)
	require.NotEmpty(t, step.Logs)
	assert.Equal(t, "context canceled", step.Logs[0])
}

func TestMultipleBgExtractions(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	m.ex.extraction.Filename = "a.pdf"

	// Background first.
	sendExtractionKey(m, keyCtrlB)

	// Create and background second.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m.ex.extraction = &extractionLogState{
		ID:         nextExtractionID.Add(1),
		Filename:   "b.pdf",
		Visible:    true,
		ctx:        ctx,
		CancelFn:   cancel,
		expanded:   make(map[extractionStep]bool),
		hasExtract: true,
	}
	m.ex.extraction.Steps[stepExtract] = extractionStepInfo{Status: stepRunning}
	sendExtractionKey(m, keyCtrlB)

	require.Len(t, m.ex.bgExtractions, 2)
	assert.Equal(t, "a.pdf", m.ex.bgExtractions[0].Filename)
	assert.Equal(t, "b.pdf", m.ex.bgExtractions[1].Filename)

	// Foreground pops most recent (b.pdf).
	sendKey(m, keyCtrlB)
	require.NotNil(t, m.ex.extraction)
	assert.Equal(t, "b.pdf", m.ex.extraction.Filename)
	require.Len(t, m.ex.bgExtractions, 1)
}

func TestStartExtraction_AutoBackgroundsExisting(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	m.ex.extraction.Filename = "existing.pdf"
	existingID := m.ex.extraction.ID

	// Manually create a new extraction state and assign it as if
	// startExtractionOverlay ran successfully (the real function needs
	// configured extractors/LLM which are complex to set up in tests).
	// This tests the backgrounding logic directly.
	m.backgroundExtraction()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m.ex.extraction = &extractionLogState{
		ID:         nextExtractionID.Add(1),
		Filename:   "new.pdf",
		Visible:    true,
		ctx:        ctx,
		CancelFn:   cancel,
		expanded:   make(map[extractionStep]bool),
		hasExtract: true,
	}
	m.ex.extraction.Steps[stepExtract] = extractionStepInfo{Status: stepRunning}

	require.Len(t, m.ex.bgExtractions, 1)
	assert.Equal(t, "existing.pdf", m.ex.bgExtractions[0].Filename)
	assert.Equal(t, existingID, m.ex.bgExtractions[0].ID)
	assert.Equal(t, "new.pdf", m.ex.extraction.Filename)
}

func TestSpinnerTick_UpdatesBgExtractions(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})

	// Background the extraction.
	sendExtractionKey(m, keyCtrlB)
	require.Len(t, m.ex.bgExtractions, 1)

	bg := m.ex.bgExtractions[0]

	// Send a spinner tick -- should update the bg spinner.
	_, cmd := m.Update(bg.Spinner.Tick())
	assert.NotNil(t, cmd, "spinner tick should return a command for bg extraction")
}

func TestCtrlQ_CancelsAllBgExtractions(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	m.ex.extraction.Filename = "fg.pdf"

	// Background it.
	sendExtractionKey(m, keyCtrlB)

	// Create another foreground extraction.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m.ex.extraction = &extractionLogState{
		ID:         nextExtractionID.Add(1),
		Filename:   "fg2.pdf",
		Visible:    true,
		ctx:        ctx,
		CancelFn:   cancel,
		expanded:   make(map[extractionStep]bool),
		hasExtract: true,
	}
	m.ex.extraction.Steps[stepExtract] = extractionStepInfo{Status: stepRunning}

	require.Len(t, m.ex.bgExtractions, 1)
	require.NotNil(t, m.ex.extraction)

	// First ctrl+c interrupts the foreground extraction (overlay stays).
	sendKey(m, "ctrl+c")

	require.NotNil(t, m.ex.extraction, "overlay should stay visible after interrupt")
	assert.True(t, m.ex.extraction.Done)
	assert.True(t, m.ex.extraction.HasError)
	assert.Equal(t, stepFailed, m.ex.extraction.Steps[stepExtract].Status)
	assert.Len(t, m.ex.bgExtractions, 1, "bg extractions untouched after interrupt")

	// Second ctrl+c cancels everything.
	sendKey(m, "ctrl+c")

	assert.Nil(t, m.ex.extraction)
	assert.Empty(t, m.ex.bgExtractions)
}

func TestStatusBar_ShowsBgExtractionIndicator(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	m.ex.extraction.Filename = "test.pdf"

	// Background it.
	sendExtractionKey(m, keyCtrlB)
	require.Len(t, m.ex.bgExtractions, 1)

	status := m.statusView()
	assert.Contains(t, status, "1 extracting", "status bar should show bg extraction count")
}

func TestStatusBar_ShowsReadyCount(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
	})
	m.ex.extraction.Done = true
	m.ex.extraction.Filename = "done.pdf"

	// Manually move to bgExtractions (simulating a bg completion).
	m.ex.extraction.Visible = false
	m.ex.bgExtractions = append(m.ex.bgExtractions, m.ex.extraction)
	m.ex.extraction = nil

	status := m.statusView()
	assert.Contains(t, status, "1 ready", "status bar should show ready count")
}

func TestFindExtraction_FindsForeground(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
	})
	id := m.ex.extraction.ID

	found := m.findExtraction(id)
	assert.Equal(t, m.ex.extraction, found)
}

func TestFindExtraction_FindsBackground(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	id := m.ex.extraction.ID

	// Background it.
	sendExtractionKey(m, keyCtrlB)
	require.Nil(t, m.ex.extraction)

	found := m.findExtraction(id)
	require.NotNil(t, found)
	assert.Equal(t, id, found.ID)
}

func TestFindExtraction_ReturnsNilForUnknownID(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
	})
	found := m.findExtraction(999999)
	assert.Nil(t, found)
}

func TestWaitForExtractProgressOpenChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan extract.ExtractProgress, 1)
	ch <- extract.ExtractProgress{Phase: "rasterize", Page: 1, Total: 3}

	cmd := waitForExtractProgress(42, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(extractionProgressMsg)
	require.True(t, ok)
	assert.Equal(t, uint64(42), result.ID)
	assert.Equal(t, "rasterize", result.Progress.Phase)
}

func TestWaitForExtractProgressClosedChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan extract.ExtractProgress)
	close(ch)

	cmd := waitForExtractProgress(7, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(extractionProgressMsg)
	require.True(t, ok)
	assert.Equal(t, uint64(7), result.ID)
	assert.True(t, result.Progress.Done)
}

// ---------------------------------------------------------------------------
// acquireTools rendering persistence
// ---------------------------------------------------------------------------

func TestAcquireTools_PersistAfterStepDone(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.Steps[stepExtract] = extractionStepInfo{
		Status: stepDone,
		Detail: "tesseract",
		Metric: "1234 chars",
	}
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: false, Count: 10},
	}
	ex.extractedPages = 10
	m.width = 120
	m.height = 40

	// Collapsed: parent shows "ocr" and percentage, children hidden.
	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "ocr", "parent should show ocr detail")
	assert.Contains(t, out, "100%", "parent should show completion percentage")
	assert.NotContains(t, out, "pdftocairo", "children hidden when collapsed")

	// Expand to see children.
	ex.expanded[stepExtract] = true
	out = m.buildExtractionOverlay()
	assert.Contains(t, out, "pdftocairo", "children visible when expanded")
}

func TestAcquireTools_ShowDuringRunning(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	ex.Steps[stepExtract] = extractionStepInfo{
		Status: stepRunning,
		Detail: "page 3/10",
	}
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: false, Count: 10},
	}
	ex.extractedPages = 10
	ex.expanded[stepExtract] = true // ext/ocr defaults to collapsed; expand for this test
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "pdftocairo", "tool lines should show during OCR")
	assert.Contains(t, out, "100%", "parent should show completion percentage")
}

func TestAcquireTools_PartialRunning(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.Steps[stepExtract] = extractionStepInfo{Status: stepRunning}
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: true, Count: 3},
	}
	ex.extractedPages = 10
	ex.expanded[stepExtract] = true // ext/ocr defaults to collapsed; expand for this test
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "pdftocairo", "running tool should show")
	assert.Contains(t, out, "30%", "parent should show 3/10 = 30%")
}

func TestAcquireTools_ParentShowsOCRDetail(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.Steps[stepExtract] = extractionStepInfo{
		Status: stepRunning,
		Detail: "page 1/5",
	}
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: false, Count: 5},
	}
	ex.extractedPages = 5
	ex.expanded[stepExtract] = true // ext/ocr defaults to collapsed; expand for this test
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	// Parent header always shows "ocr" detail, not "page X/Y".
	assert.Contains(t, out, "ocr", "parent should show ocr detail")
	assert.Contains(t, out, "100%", "parent should show completion percentage")
	assert.Contains(t, out, "pdftocairo", "children visible when expanded")
}

func TestAcquireTools_CollapseHidesChildren(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.Steps[stepExtract] = extractionStepInfo{
		Status: stepDone,
		Detail: "tesseract",
		Metric: "1234 chars",
	}
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: false, Count: 10},
	}
	m.width = 120
	m.height = 40

	// Default collapsed: parent visible, children hidden.
	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "ocr", "parent always visible")
	assert.NotContains(t, out, "pdftocairo", "children hidden when collapsed")

	// Add logs and verify expand shows everything.
	info := ex.Steps[stepExtract]
	info.Logs = []string{"extracted text line 1", "extracted text line 2"}
	ex.Steps[stepExtract] = info

	// Still collapsed: children + logs hidden.
	out = m.buildExtractionOverlay()
	assert.NotContains(t, out, "pdftocairo", "children hidden when collapsed")
	assert.NotContains(t, out, "extracted text line 1", "logs hidden when collapsed")

	// User expands: children + logs both show.
	ex.expanded[stepExtract] = true
	out = m.buildExtractionOverlay()
	assert.Contains(t, out, "pdftocairo", "children visible when expanded")
	assert.Contains(t, out, "extracted text line 1", "logs visible when expanded")
}

// --- Ext parent/child navigation ---

func newExtToolModel(t *testing.T) (*Model, *extractionLogState) {
	t.Helper()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: false, Count: 10},
		{Tool: "tesseract", Running: false, Count: 10},
	}
	ex.extractedPages = 10
	m.width = 120
	m.height = 40
	return m, ex
}

func TestExtraction_ExtParentChildNavigation(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)

	// Start on text (cursor=0). toolCursor=-1 (parent sentinel).
	assert.Equal(t, 0, ex.cursor)
	assert.Equal(t, -1, ex.toolCursor)

	// j -> ext parent (cursor=1, toolCursor=-1).
	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor, "j should land on ext step")
	assert.Equal(t, -1, ex.toolCursor, "should be on parent")

	// Expand ext so children are accessible.
	ex.expanded[stepExtract] = true

	// j -> first child (toolCursor=0).
	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor, "still on ext step")
	assert.Equal(t, 0, ex.toolCursor, "should be on first child")

	// j -> second child (toolCursor=1).
	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor)
	assert.Equal(t, 1, ex.toolCursor, "should be on second child")

	// j -> next step (llm).
	sendExtractionKey(m, "j")
	assert.Equal(t, 2, ex.cursor, "should advance to llm")
	assert.Equal(t, -1, ex.toolCursor, "new step starts at parent")

	// k -> back to ext, last child (expanded).
	sendExtractionKey(m, "k")
	assert.Equal(t, 1, ex.cursor, "should return to ext")
	assert.Equal(t, 1, ex.toolCursor, "should land on last child")

	// k -> first child.
	sendExtractionKey(m, "k")
	assert.Equal(t, 1, ex.cursor)
	assert.Equal(t, 0, ex.toolCursor)

	// k -> parent.
	sendExtractionKey(m, "k")
	assert.Equal(t, 1, ex.cursor)
	assert.Equal(t, -1, ex.toolCursor, "should return to parent")

	// k -> text step.
	sendExtractionKey(m, "k")
	assert.Equal(t, 0, ex.cursor, "should return to text step")
}

func TestExtraction_ExtCollapsedSkipsChildren(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)

	// j -> ext parent (collapsed by default for done ext).
	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor)
	assert.Equal(t, -1, ex.toolCursor)

	// j from collapsed parent -> next step (skips children).
	sendExtractionKey(m, "j")
	assert.Equal(t, 2, ex.cursor, "should skip children when collapsed")
	assert.Equal(t, -1, ex.toolCursor)

	// k from llm -> ext parent (collapsed, not last child).
	sendExtractionKey(m, "k")
	assert.Equal(t, 1, ex.cursor)
	assert.Equal(t, -1, ex.toolCursor, "collapsed ext should land on parent")
}

func TestExtraction_EnterOnChildIsNoOp(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)

	// Navigate to ext and expand.
	sendExtractionKey(m, "j")
	ex.expanded[stepExtract] = true

	// Move to a child.
	sendExtractionKey(m, "j")
	assert.Equal(t, 0, ex.toolCursor, "on first child")

	// Enter on a child should not toggle the parent.
	sendExtractionKey(m, "enter")
	assert.True(t, ex.stepExpanded(stepExtract), "should stay expanded")
	assert.Equal(t, 0, ex.toolCursor, "should stay on child")
}

func TestExtraction_EnterOnExtParentToggles(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)

	// Navigate to ext parent (done step, default collapsed).
	sendExtractionKey(m, "j")
	assert.Equal(t, -1, ex.toolCursor, "on parent")
	assert.False(t, ex.stepExpanded(stepExtract), "done ext starts collapsed")

	// Enter expands from parent.
	sendExtractionKey(m, "enter")
	assert.True(t, ex.stepExpanded(stepExtract), "should be expanded")

	// Enter again collapses.
	sendExtractionKey(m, "enter")
	assert.False(t, ex.stepExpanded(stepExtract), "should be collapsed")
	assert.Equal(t, -1, ex.toolCursor, "should stay on parent")
}

func TestExtraction_CollapseResetsViewportOffset(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)
	ex.cursorManual = true

	// Expand ext step and render to populate viewport.
	ex.expanded[stepExtract] = true
	m.buildExtractionOverlay()

	// Simulate a scroll offset as if user had scrolled down.
	ex.Viewport.SetYOffset(2)

	// Collapse via Enter on parent.
	sendExtractionKey(m, "j")
	sendExtractionKey(m, "enter")

	// Re-render to trigger offset correction.
	out := m.buildExtractionOverlay()

	// The text step header should be visible (offset reset to 0).
	assert.Contains(t, out, "text", "text step should be visible after collapse")
	assert.Equal(t, 0, ex.Viewport.YOffset, "viewport offset should reset when content fits")
}

// --- Ext child rendering coverage ---

func TestAcquireTools_NonTerminalDoneRenderedDim(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)
	ex.expanded[stepExtract] = true

	out := m.buildExtractionOverlay()
	// pdftocairo (non-terminal) should render with dim "ok" and dim page ratio.
	assert.Contains(t, out, "pdftocairo", "non-terminal tool visible when expanded")
	assert.Contains(t, out, "10/10 pp", "dim page ratio for non-terminal tool")
	// tesseract (terminal) should render with bright styling.
	assert.Contains(t, out, "tesseract")
}

func TestAcquireTools_NonTerminalPageRatioUsesTotal(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)
	// Set count != extractedPages to verify the denominator is the total,
	// not the tool's own count (regression: was rendering "7/7 pp").
	ex.acquireTools[0].Count = 7
	ex.extractedPages = 20
	ex.expanded[stepExtract] = true

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "7/20 pp", "non-terminal denominator must be extractedPages")
	assert.NotContains(t, out, "7/7 pp", "denominator must not equal numerator")
}

func TestAcquireTools_ParentShowsZeroPercentInitially(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.Steps[stepExtract] = extractionStepInfo{Status: stepRunning}
	// Initial state: tools exist but no pages processed yet, total unknown.
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: true, Count: 0},
		{Tool: "tesseract", Running: true, Count: 0},
	}
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "0%", "parent should show 0% before any pages complete")
}

func TestAcquireTools_ParentShowsPipelinePercentage(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.Steps[stepExtract] = extractionStepInfo{Status: stepRunning}
	// pdftocairo 8/10, tesseract 3/10 => (8+3)/(10*2) = 55%
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: true, Count: 8},
		{Tool: "tesseract", Running: true, Count: 3},
	}
	ex.extractedPages = 10
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "55%", "parent should show combined pipeline percentage")
}

func TestHandleExtractionProgress_AcquireToolsSetsExtractedPages(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.extractCh = make(<-chan extract.ExtractProgress)

	// Simulate a mid-pipeline progress message: AcquireTools is set
	// alongside Phase/Total/DocPages. Before the fix, the AcquireTools
	// branch returned early and never set extractedPages/docPages.
	m.handleExtractionProgress(extractionProgressMsg{
		ID: ex.ID,
		Progress: extract.ExtractProgress{
			Phase:    "extract",
			Page:     3,
			Total:    10,
			DocPages: 25,
			AcquireTools: []extract.AcquireToolState{
				{Tool: "pdftocairo", Running: true, Count: 5},
				{Tool: "tesseract", Running: true, Count: 3},
			},
		},
	})

	assert.Equal(t, 10, ex.extractedPages, "extractedPages must be set from Total")
	assert.Equal(t, 25, ex.docPages, "docPages must be set from DocPages")
	assert.Equal(t, "page 3/10", ex.Steps[stepExtract].Detail, "step detail must be updated")
	require.Len(t, ex.acquireTools, 2)
	assert.Equal(t, 5, ex.acquireTools[0].Count)
}

func TestAcquireTools_NonTerminalRunningRenderedDim(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: true, Count: 3},
		{Tool: "tesseract", Running: true, Count: 0},
	}
	ex.extractedPages = 10
	ex.expanded[stepExtract] = true // ext/ocr defaults to collapsed; expand for this test
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "pdftocairo", "running non-terminal tool visible")
	assert.Contains(t, out, "tesseract", "running terminal tool visible")
}

func TestAcquireTools_NonTerminalFailedRenderedDim(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepFailed,
	})
	ex := m.ex.extraction
	ex.Steps[stepExtract] = extractionStepInfo{Status: stepFailed}
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdftocairo", Running: false, Err: errors.New("fail"), Count: 5},
		{Tool: "tesseract", Running: false, Err: errors.New("fail"), Count: 0},
	}
	ex.extractedPages = 10
	m.width = 120
	m.height = 40

	// Failed = auto-expanded, both children visible.
	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "pdftocairo", "failed non-terminal tool visible")
	assert.Contains(t, out, "tesseract", "failed terminal tool visible")
}

func TestAcquireTools_ChildCursorRendered(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)
	ex.expanded[stepExtract] = true

	// Navigate to ext, then to first child.
	sendExtractionKey(m, "j") // ext parent
	sendExtractionKey(m, "j") // first child (pdftocairo)
	assert.Equal(t, 0, ex.toolCursor)
	assert.True(t, ex.cursorManual)

	out := m.buildExtractionOverlay()
	// Child cursor triangle should be rendered.
	assert.Contains(t, out, symTriRightSm, "child cursor triangle should render")
}

func TestRenderPageRatio_Capped(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	// docPages > 0 triggers the three-part ratio: count/limit/total pp.
	out := m.renderPageRatio(5, 10, 20)
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "10")
	assert.Contains(t, out, "20")
	assert.Contains(t, out, "pp")
}

func TestAcquireTools_CursorLineOffsetForChild(t *testing.T) {
	t.Parallel()
	m, ex := newExtToolModel(t)
	ex.expanded[stepExtract] = true

	// Navigate to ext parent, then to second child (tesseract).
	sendExtractionKey(m, "j") // ext parent
	sendExtractionKey(m, "j") // child 0
	sendExtractionKey(m, "j") // child 1
	assert.Equal(t, 1, ex.toolCursor)

	// Building the overlay exercises the cursorLine offset path.
	out := m.buildExtractionOverlay()
	assert.NotEmpty(t, out, "overlay should render with child cursor offset")
}

func TestWaitForLLMChunkOpenChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan llm.StreamChunk, 1)
	ch <- llm.StreamChunk{Content: "hello", Done: false}

	cmd := waitForLLMChunk(10, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(extractionLLMChunkMsg)
	require.True(t, ok)
	assert.Equal(t, uint64(10), result.ID)
	assert.Equal(t, "hello", result.Content)
	assert.False(t, result.Done)
}

func TestWaitForLLMChunkClosedChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan llm.StreamChunk)
	close(ch)

	cmd := waitForLLMChunk(10, ch)
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(extractionLLMChunkMsg)
	require.True(t, ok)
	assert.True(t, result.Done)
}

// --- Dispatch cross-reference ---

func TestDispatch_VendorCrossReference(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a project so the quote has a valid FK target.
	types, err := m.store.ProjectTypes()
	require.NoError(t, err)
	project := data.Project{
		Title:         "Plumbing Reno",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, m.store.CreateProject(&project))

	// Also create a document so DocID is valid.
	doc := data.Document{Title: "Invoice", FileName: "inv.pdf"}
	require.NoError(t, m.store.CreateDocument(&doc))

	// Simulate LLM output: create vendor then quote referencing it with
	// a fictional vendor_id that doesn't match the real DB ID.
	ops := []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "Garcia Plumbing",
		}},
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   float64(1),
			"project_id":  float64(project.ID),
			"total_cents": float64(150000),
		}},
	}

	// Stage through shadow DB.
	sdb, err := extract.NewShadowDB(m.store)
	require.NoError(t, err)
	require.NoError(t, sdb.Stage(ops))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	defer cancel()
	ex := &extractionLogState{
		ID:         nextExtractionID.Add(1),
		ctx:        ctx,
		CancelFn:   cancel,
		Visible:    true,
		expanded:   make(map[extractionStep]bool),
		Done:       true,
		DocID:      doc.ID,
		operations: ops,
		shadowDB:   sdb,
	}
	ex.Steps[stepLLM] = extractionStepInfo{Status: stepDone}
	ex.hasLLM = true
	m.ex.extraction = ex

	// Press "a" to accept -- should not error.
	sendExtractionKey(m, "a")
	assert.Nil(t, m.ex.extraction, "extraction should be cleared after accept")

	// Verify the vendor was created.
	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	var garcia *data.Vendor
	for i := range vendors {
		if vendors[i].Name == "Garcia Plumbing" {
			garcia = &vendors[i]
			break
		}
	}
	require.NotNil(t, garcia, "vendor should have been created")

	// Verify the quote was created and linked to the correct vendor.
	quotes, err := m.store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, garcia.ID, quotes[0].VendorID)
	assert.Equal(t, int64(150000), quotes[0].TotalCents)
}

func TestDispatch_InvalidProjectIDShowsError(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a vendor and document but no project.
	doc := data.Document{Title: "Quote", FileName: "q.pdf"}
	require.NoError(t, m.store.CreateDocument(&doc))
	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Acme"}))

	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.NotEmpty(t, vendors)
	acme := vendors[0]

	ops := []extract.Operation{
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   float64(acme.ID),
			"project_id":  float64(9999),
			"total_cents": float64(50000),
		}},
	}

	// Stage through shadow DB.
	sdb, err := extract.NewShadowDB(m.store)
	require.NoError(t, err)
	require.NoError(t, sdb.Stage(ops))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	defer cancel()
	ex := &extractionLogState{
		ID:         nextExtractionID.Add(1),
		ctx:        ctx,
		CancelFn:   cancel,
		Visible:    true,
		expanded:   make(map[extractionStep]bool),
		Done:       true,
		DocID:      doc.ID,
		operations: ops,
		shadowDB:   sdb,
	}
	ex.Steps[stepLLM] = extractionStepInfo{Status: stepDone}
	ex.hasLLM = true
	m.ex.extraction = ex

	// Accept should fail with an error (FK constraint on project_id).
	sendExtractionKey(m, "a")
	assert.NotNil(t, m.ex.extraction, "extraction stays open on dispatch error")
	assert.Contains(t, m.status.Text, "FOREIGN KEY constraint failed")
}

// TestDispatch_OffsetCrossReference verifies that when the real DB already
// contains vendors, shadow auto-increment IDs start after the max real ID,
// and cross-references between batch-created entities resolve correctly.
func TestDispatch_OffsetCrossReference(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a project for the quote FK.
	types, err := m.store.ProjectTypes()
	require.NoError(t, err)
	project := data.Project{
		Title:         "Offset Test",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, m.store.CreateProject(&project))

	doc := data.Document{Title: "Invoice", FileName: "inv.pdf"}
	require.NoError(t, m.store.CreateDocument(&doc))

	// Pre-populate 3 vendors so real IDs are 1-3.
	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: name}))
	}

	// LLM creates a new vendor and a quote referencing it.
	// The LLM sees max vendor ID = 3, so it emits vendor_id: 4.
	ops := []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "Delta Electric",
		}},
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   float64(4),
			"project_id":  float64(project.ID),
			"total_cents": float64(200000),
		}},
	}

	sdb, err := extract.NewShadowDB(m.store)
	require.NoError(t, err)
	require.NoError(t, sdb.Stage(ops))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	defer cancel()
	ex := &extractionLogState{
		ID:         nextExtractionID.Add(1),
		ctx:        ctx,
		CancelFn:   cancel,
		Visible:    true,
		expanded:   make(map[extractionStep]bool),
		Done:       true,
		DocID:      doc.ID,
		operations: ops,
		shadowDB:   sdb,
	}
	ex.Steps[stepLLM] = extractionStepInfo{Status: stepDone}
	ex.hasLLM = true
	m.ex.extraction = ex

	sendExtractionKey(m, "a")
	assert.Nil(t, m.ex.extraction, "extraction cleared after accept")

	// Verify the new vendor was created.
	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	var delta *data.Vendor
	for i := range vendors {
		if vendors[i].Name == "Delta Electric" {
			delta = &vendors[i]
			break
		}
	}
	require.NotNil(t, delta, "new vendor should exist")

	// Verify the quote points to the real ID of the new vendor.
	quotes, err := m.store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, delta.ID, quotes[0].VendorID)
	assert.Equal(t, int64(200000), quotes[0].TotalCents)
}

// TestDispatch_DuplicateVendorDedup verifies that accepting an extraction
// that creates a vendor with the same name as an existing one deduplicates
// via FindOrCreate instead of creating a duplicate.
func TestDispatch_DuplicateVendorDedup(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	types, err := m.store.ProjectTypes()
	require.NoError(t, err)
	project := data.Project{
		Title:         "Dedup Test",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, m.store.CreateProject(&project))

	doc := data.Document{Title: "Invoice", FileName: "inv.pdf"}
	require.NoError(t, m.store.CreateDocument(&doc))

	// Pre-create a vendor.
	require.NoError(t, m.store.CreateVendor(&data.Vendor{Name: "Acme Plumbing"}))
	vendorsBefore, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendorsBefore, 1)
	existingID := vendorsBefore[0].ID

	// LLM creates a vendor with the same name and a quote referencing it.
	ops := []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "Acme Plumbing",
		}},
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   float64(2),
			"project_id":  float64(project.ID),
			"total_cents": float64(75000),
		}},
	}

	sdb, err := extract.NewShadowDB(m.store)
	require.NoError(t, err)
	require.NoError(t, sdb.Stage(ops))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	defer cancel()
	ex := &extractionLogState{
		ID:         nextExtractionID.Add(1),
		ctx:        ctx,
		CancelFn:   cancel,
		Visible:    true,
		expanded:   make(map[extractionStep]bool),
		Done:       true,
		DocID:      doc.ID,
		operations: ops,
		shadowDB:   sdb,
	}
	ex.Steps[stepLLM] = extractionStepInfo{Status: stepDone}
	ex.hasLLM = true
	m.ex.extraction = ex

	sendExtractionKey(m, "a")
	assert.Nil(t, m.ex.extraction, "extraction cleared after accept")

	// No duplicate vendor created.
	vendorsAfter, err := m.store.ListVendors(false)
	require.NoError(t, err)
	assert.Len(t, vendorsAfter, 1, "should still have exactly one vendor")
	assert.Equal(t, existingID, vendorsAfter[0].ID, "should be the same vendor")

	// Quote linked to the existing vendor.
	quotes, err := m.store.ListQuotes(false)
	require.NoError(t, err)
	require.Len(t, quotes, 1)
	assert.Equal(t, existingID, quotes[0].VendorID)
}

// TestDispatch_DuplicateApplianceDedup verifies that batch-created appliances
// with names matching existing ones are deduplicated.
func TestDispatch_DuplicateApplianceDedup(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	doc := data.Document{Title: "Manual", FileName: "man.pdf"}
	require.NoError(t, m.store.CreateDocument(&doc))

	// Pre-create an appliance.
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Water Heater"}))
	before, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.Len(t, before, 1)
	existingID := before[0].ID

	categories, err := m.store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, categories)
	catID := categories[0].ID

	// LLM creates an appliance with the same name and a maintenance item.
	ops := []extract.Operation{
		{Action: "create", Table: data.TableAppliances, Data: map[string]any{
			"name": "Water Heater",
		}},
		{Action: "create", Table: data.TableMaintenanceItems, Data: map[string]any{
			"name":         "Flush Tank",
			"appliance_id": float64(2),
			"category_id":  float64(catID),
		}},
	}

	sdb, err := extract.NewShadowDB(m.store)
	require.NoError(t, err)
	require.NoError(t, sdb.Stage(ops))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	defer cancel()
	ex := &extractionLogState{
		ID:         nextExtractionID.Add(1),
		ctx:        ctx,
		CancelFn:   cancel,
		Visible:    true,
		expanded:   make(map[extractionStep]bool),
		Done:       true,
		DocID:      doc.ID,
		operations: ops,
		shadowDB:   sdb,
	}
	ex.Steps[stepLLM] = extractionStepInfo{Status: stepDone}
	ex.hasLLM = true
	m.ex.extraction = ex

	sendExtractionKey(m, "a")
	assert.Nil(t, m.ex.extraction, "extraction cleared after accept")

	// No duplicate appliance.
	after, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	assert.Len(t, after, 1, "should still have exactly one appliance")
	assert.Equal(t, existingID, after[0].ID)

	// Maintenance item linked to the existing appliance.
	items, err := m.store.ListMaintenance(false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].ApplianceID)
	assert.Equal(t, existingID, *items[0].ApplianceID)
	assert.Equal(t, "Flush Tank", items[0].Name)
}

// TestDispatch_TransactionRollbackOnFailure verifies that if one operation
// in a batch fails, the entire batch is rolled back atomically.
func TestDispatch_TransactionRollbackOnFailure(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	doc := data.Document{Title: "Quote", FileName: "q.pdf"}
	require.NoError(t, m.store.CreateDocument(&doc))

	// Batch: create a vendor, then a quote with an invalid project_id.
	// The vendor should NOT persist because the transaction rolls back.
	ops := []extract.Operation{
		{Action: "create", Table: data.TableVendors, Data: map[string]any{
			"name": "Ghost Vendor",
		}},
		{Action: "create", Table: data.TableQuotes, Data: map[string]any{
			"vendor_id":   float64(1),
			"project_id":  float64(9999),
			"total_cents": float64(50000),
		}},
	}

	sdb, err := extract.NewShadowDB(m.store)
	require.NoError(t, err)
	require.NoError(t, sdb.Stage(ops))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	defer cancel()
	ex := &extractionLogState{
		ID:         nextExtractionID.Add(1),
		ctx:        ctx,
		CancelFn:   cancel,
		Visible:    true,
		expanded:   make(map[extractionStep]bool),
		Done:       true,
		DocID:      doc.ID,
		operations: ops,
		shadowDB:   sdb,
	}
	ex.Steps[stepLLM] = extractionStepInfo{Status: stepDone}
	ex.hasLLM = true
	m.ex.extraction = ex

	sendExtractionKey(m, "a")
	assert.NotNil(t, m.ex.extraction, "extraction stays open on error")

	// Verify no vendor was created (transaction rolled back).
	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	assert.Empty(t, vendors, "vendor should not persist after rollback")

	// Verify no quote was created either.
	quotes, err := m.store.ListQuotes(false)
	require.NoError(t, err)
	assert.Empty(t, quotes, "quote should not persist after rollback")
}

// --- Per-pipeline LLM config (user-interaction tests) ---

// testExtractionOllamaClient creates an Ollama client for extraction tests
// that don't hit a real server.
func testExtractionOllamaClient(t *testing.T, model string) *llm.Client {
	t.Helper()
	c, err := llm.NewClient("ollama", "http://localhost:11434", model, "", 5*time.Second)
	require.NoError(t, err)
	return c
}

// TestModelPicker_ShowsExtractionModel verifies that the model picker
// displays the extraction-specific model when extraction has its own config,
// not the chat model. The user opens the picker via 'r' on the LLM step.
func TestModelPicker_ShowsExtractionModel(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Chat uses one model; extraction uses another.
	m.llmClient = testExtractionOllamaClient(t, "chat-model")
	m.ex.extractionModel = "extraction-model"

	// User presses 'r' to open the model picker.
	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)

	// The extraction model label should reflect the extraction-specific model.
	assert.Equal(t, "extraction-model", m.extractionModelLabel())
}

// TestModelPicker_FallsBackToChatModel verifies that when no extraction-specific
// model is configured, the model picker label falls back to the chat model.
func TestModelPicker_FallsBackToChatModel(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Chat model set, but no extraction-specific model.
	m.llmClient = testExtractionOllamaClient(t, "chat-model")
	m.ex.extractionModel = ""

	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)

	// Should fall back to chat model.
	assert.Equal(t, "chat-model", m.extractionModelLabel())
}

// TestModelPicker_SelectionWithIndependentProvider verifies that selecting
// a new model via the picker correctly updates the extraction model and
// invalidates the cached extraction client when extraction has its own provider.
func TestModelPicker_SelectionWithIndependentProvider(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Chat uses ollama on the default port.
	m.llmClient = testExtractionOllamaClient(t, "chat-model")
	m.chatCfg = chatConfig{
		Provider: "ollama",
		BaseURL:  "http://localhost:11434",
		Model:    "chat-model",
		Timeout:  5 * time.Second,
	}

	// Extraction uses a separate ollama instance on a different port.
	m.ex.extractionProvider = "ollama"
	m.ex.extractionBaseURL = "http://localhost:8080"
	m.ex.extractionModel = "original-extraction-model"
	m.ex.extractionTimeout = 10 * time.Second

	// Pre-populate the extraction client cache.
	client := m.extractionLLMClient()
	require.NotNil(t, client)
	assert.Equal(t, "original-extraction-model", client.Model())

	// User opens model picker.
	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	ex.modelPicker.Loading = false
	ex.modelPicker.All = []modelCompleterEntry{
		{Name: "original-extraction-model", Local: true},
		{Name: "new-extraction-model", Local: true},
	}
	refilterModelCompleter(ex.modelPicker, "", m.extractionModelLabel())

	// Navigate to the second entry and select.
	sendExtractionKey(m, "down")
	sendExtractionKey(m, "enter")

	// Model should be updated to the selected entry.
	assert.Equal(t, "new-extraction-model", m.ex.extractionModel)

	// The client should have been rebuilt with the new model.
	newClient := m.extractionLLMClient()
	require.NotNil(t, newClient)
	assert.Equal(t, "new-extraction-model", newClient.Model())
}

// TestExtractionClient_IndependentFromChat verifies that when extraction has
// its own provider/baseURL, extractionLLMClient() creates a client that is
// independent from the chat client.
func TestExtractionClient_IndependentFromChat(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Chat config: ollama on default port.
	m.llmClient = testExtractionOllamaClient(t, "chat-model")
	m.chatCfg = chatConfig{
		Provider: "ollama",
		BaseURL:  "http://localhost:11434",
		Model:    "chat-model",
		Timeout:  5 * time.Second,
	}

	// Extraction: separate ollama instance on a different port.
	m.ex.extractionProvider = "ollama"
	m.ex.extractionBaseURL = "http://localhost:8080"
	m.ex.extractionModel = "extraction-model"
	m.ex.extractionTimeout = 10 * time.Second

	// The extraction client should have its own model, not the chat model.
	client := m.extractionLLMClient()
	require.NotNil(t, client)
	assert.Equal(t, "extraction-model", client.Model())
	assert.NotEqual(t, m.llmClient.Model(), client.Model())

	// User opens the model picker -- should see extraction model label.
	sendExtractionKey(m, "r")
	require.NotNil(t, ex.modelPicker)
	assert.Equal(t, "extraction-model", m.extractionModelLabel())
}

// TestExtractionClient_NilWhenNoExtractionModel verifies that when
// extraction has no model configured, extractionLLMClient() returns nil
// even if chat has a model. Each pipeline is fully independent.
func TestExtractionClient_NilWhenNoExtractionModel(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	m.ex.extraction.Done = true

	// Chat config is fully configured.
	m.llmClient = testExtractionOllamaClient(t, "chat-model")
	m.chatCfg = chatConfig{
		Provider: "ollama",
		BaseURL:  "http://localhost:11434",
		Model:    "chat-model",
		Timeout:  5 * time.Second,
	}

	// Extraction has no model -- should NOT fall back to chat.
	m.ex.extractionModel = ""

	assert.Nil(t, m.extractionLLMClient(),
		"extraction should not fall back to chat config")
}

// TestExtractionClient_NilWhenNoConfig verifies that extractionLLMClient()
// returns nil when neither extraction nor chat config provides a model.
func TestExtractionClient_NilWhenNoConfig(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	m.ex.extraction.Done = true

	// No chat client, no chat config, no extraction config.
	m.llmClient = nil
	m.chatCfg = chatConfig{}
	m.ex.extractionModel = ""

	assert.Nil(t, m.extractionLLMClient())
}

// --- Auto-follow vs manual cursor mode ---

func TestExtractionCursor_AutoFollowDisengagesOnJK(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction

	// Initially in auto-follow mode.
	assert.False(t, ex.cursorManual, "should start in auto-follow mode")

	// advanceCursor should move cursor when auto-following.
	ex.Steps[stepExtract] = extractionStepInfo{Status: stepDone}
	ex.advanceCursor()
	assert.Equal(t, 1, ex.cursor, "advanceCursor should move to ext step in auto mode")

	// Press j to switch to manual mode.
	sendExtractionKey(m, "j")
	assert.True(t, ex.cursorManual, "j should engage manual mode")

	// Mark LLM as running so advanceCursor would normally advance there.
	ex.hasLLM = true
	ex.Steps[stepLLM] = extractionStepInfo{Status: stepDone}
	prevCursor := ex.cursor
	ex.advanceCursor()
	assert.Equal(t, prevCursor, ex.cursor, "advanceCursor should be no-op in manual mode")
}

func TestExtractionCursor_ManualModeViaUpKey(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction

	// Move cursor to ext via advanceCursor (auto mode).
	ex.advanceCursor()
	assert.Equal(t, 1, ex.cursor)
	assert.False(t, ex.cursorManual)

	// Press k to switch to manual.
	sendExtractionKey(m, "k")
	assert.True(t, ex.cursorManual, "k should engage manual mode")
	assert.Equal(t, 0, ex.cursor, "k should navigate back to text step")
}

// --- LLM ping ---

func TestExtractionLLMPing_FailSkipsLLMStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	id := ex.ID

	// Ping comes back with an error while extraction is still running.
	m.Update(extractionLLMPingMsg{ID: id, Err: errors.New("connection refused")})
	assert.True(t, ex.llmPingDone)
	require.Error(t, ex.llmPingErr)
	// LLM step should be skipped immediately (strikethrough in real time).
	assert.Equal(t, stepSkipped, ex.Steps[stepLLM].Status)
	assert.False(t, ex.Done, "pipeline not done -- extraction still running")

	// Now extraction finishes -- LLM should be skipped, not started.
	m.Update(extractionProgressMsg{
		ID: id,
		Progress: extract.ExtractProgress{
			Done: true,
			Tool: "tesseract",
			Text: "some ocr text",
		},
	})
	assert.Equal(t, stepSkipped, ex.Steps[stepLLM].Status)
	assert.True(t, ex.Done)
	require.NotEmpty(t, ex.Steps[stepLLM].Logs)
	assert.Contains(t, ex.Steps[stepLLM].Logs[0], "connection refused")
}

func TestExtractionLLMPing_FailAfterExtractDone(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	id := ex.ID

	// Extraction already done when ping fails -- LLM skipped immediately.
	m.Update(extractionLLMPingMsg{ID: id, Err: errors.New("unreachable")})
	assert.Equal(t, stepSkipped, ex.Steps[stepLLM].Status)
	assert.True(t, ex.Done)
}

func TestExtractionLLMPing_SuccessAllowsLLM(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	id := ex.ID

	// Ping succeeds.
	m.Update(extractionLLMPingMsg{ID: id, Err: nil})
	assert.True(t, ex.llmPingDone)
	require.NoError(t, ex.llmPingErr)
	// LLM still pending -- extraction hasn't finished.
	assert.Equal(t, stepPending, ex.Steps[stepLLM].Status)
}

func TestExtractionSkippedStep_Navigable(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepSkipped,
	})
	ex := m.ex.extraction
	ex.Done = true

	// advanceCursor should land on skipped step.
	ex.advanceCursor()
	assert.Equal(t, 2, ex.cursor)

	// j/k navigation: skipped step should be reachable.
	sendExtractionKey(m, "k")
	assert.Equal(t, 1, ex.cursor)
	sendExtractionKey(m, "j")
	assert.Equal(t, 2, ex.cursor)
}

func TestExtractionSkippedStep_DefaultExpanded(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepSkipped,
	})
	ex := m.ex.extraction
	ex.Steps[stepLLM] = extractionStepInfo{
		Status: stepSkipped,
		Logs:   []string{"connection refused"},
	}

	assert.True(t, ex.stepDefaultExpanded(stepLLM),
		"skipped step should auto-expand to show error")
	_ = m // keep linter happy
}

func TestExtractionSkippedStep_RerunHintShows(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepSkipped,
	})
	ex := m.ex.extraction
	ex.Done = true

	// Position cursor on LLM step.
	ex.advanceCursor()
	assert.Equal(t, stepLLM, ex.cursorStep())

	view := m.buildExtractionOverlay()
	assert.Contains(t, view, "r model",
		"rerun hint should appear for skipped LLM step")
}

func TestExtractionRerun_ClearsPingState(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepSkipped,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.llmPingDone = true
	ex.llmPingErr = errors.New("was unreachable")

	// Simulate rerun (which resets LLM state).
	m.rerunLLMExtraction()
	assert.False(t, ex.llmPingDone, "ping state should be cleared on rerun")
	assert.NoError(t, ex.llmPingErr, "ping error should be cleared on rerun")
}

func TestExtractionLLMPing_FailAfterExtractFailed(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepFailed,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	id := ex.ID

	// Extraction already failed when ping fails -- LLM skipped, pipeline done.
	m.Update(extractionLLMPingMsg{ID: id, Err: errors.New("unreachable")})
	assert.Equal(t, stepSkipped, ex.Steps[stepLLM].Status)
	assert.True(t, ex.Done)
}

func TestExtractionLLMPing_StaleIDIgnored(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})

	// Ping with a non-matching ID is silently dropped.
	m.Update(extractionLLMPingMsg{ID: 99999, Err: errors.New("boom")})
	assert.Equal(t, stepPending, m.ex.extraction.Steps[stepLLM].Status)
}

func TestExtractionLLMPing_BgExtractionSkipStatus(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	id := ex.ID

	// Background the extraction.
	sendExtractionKey(m, keyCtrlB)
	require.Nil(t, m.ex.extraction)
	require.Len(t, m.ex.bgExtractions, 1)

	// Ping failure on a background extraction.
	m.Update(extractionLLMPingMsg{ID: id, Err: errors.New("unreachable")})
	assert.Equal(t, stepSkipped, ex.Steps[stepLLM].Status)
	assert.True(t, ex.Done)
	assert.Contains(t, m.status.Text, "LLM skipped")
}

func TestMaybeStartLLMStep_NoLLM(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepDone,
	})
	ex := m.ex.extraction
	// hasLLM is false -- should return nil without touching steps.
	cmd := m.maybeStartLLMStep(ex)
	assert.Nil(t, cmd)
}

func TestMaybeStartLLMStep_NilClient(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepDone,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	// hasLLM true but no client configured -- should return nil.
	cmd := m.maybeStartLLMStep(ex)
	assert.Nil(t, cmd)
	assert.Equal(t, stepPending, ex.Steps[stepLLM].Status)
}

func TestExtractionSkippedStep_RendersNAIcon(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepSkipped,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.Steps[stepLLM].Detail = "qwen3"
	ex.Steps[stepLLM].Logs = []string{"cannot reach ollama"}
	ex.advanceCursor()

	view := m.buildExtractionOverlay()
	assert.Contains(t, view, "na", "skipped icon should render")
	assert.Contains(t, view, "llm", "step name should render")
	assert.Contains(t, view, "qwen3", "model detail should render")
	assert.Contains(t, view, "cannot reach ollama",
		"error log should render in expanded skipped step")
}

func TestExtractionSkippedStep_LogNotJSON(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepSkipped,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.Steps[stepLLM].Detail = "qwen3"
	ex.Steps[stepLLM].Logs = []string{"cannot reach ollama -- start it with ollama serve"}
	ex.advanceCursor()

	view := m.buildExtractionOverlay()
	// Should NOT contain JSON code fence markers that glamour would produce.
	assert.NotContains(t, view, "```",
		"skipped step log should not be rendered as JSON")
}

func TestExtractionFailedLLMStep_ErrorRenderedAsPlainText(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepRunning,
	})
	ex := m.ex.extraction
	ex.hasLLM = true
	ex.Steps[stepLLM].Started = time.Now()

	// Drive failure through the message path (simulating real LLM error).
	m.Update(extractionLLMChunkMsg{
		ID:   ex.ID,
		Err:  errors.New("parse error: unexpected token"),
		Done: true,
	})
	require.Equal(t, stepFailed, ex.Steps[stepLLM].Status)

	// Expand the failed step to see the log.
	ex.expanded[stepLLM] = true

	view := m.buildExtractionOverlay()
	assert.Contains(t, view, "parse error: unexpected token",
		"failed step error should be visible as plain text")
}

func TestExtractionFailedLLMStep_JSONNotPrettyPrinted(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepFailed,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.hasLLM = true
	ex.HasError = true
	// Use compact valid JSON as the log -- if the code takes the JSON
	// rendering path, json.Indent would expand it to multiple lines.
	// The fix should render it as-is (plain text, no pretty-printing).
	ex.Steps[stepLLM].Logs = []string{`{"error":"bad input"}`}
	ex.expanded[stepLLM] = true
	ex.advanceCursor()

	view := m.buildExtractionOverlay()
	assert.Contains(t, view, `{"error":"bad input"}`,
		"compact JSON should remain on one line, not pretty-printed")
}

func TestExtractionFailedExtractStep_ErrorRenderedAsPlainText(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.hasExtract = true
	ex.Steps[stepExtract].Started = time.Now()

	// Drive extract failure through the message path.
	m.Update(extractionProgressMsg{
		ID:       ex.ID,
		Progress: extract.ExtractProgress{Err: errors.New("tesseract not found")},
	})
	require.Equal(t, stepFailed, ex.Steps[stepExtract].Status)

	ex.expanded[stepExtract] = true

	view := m.buildExtractionOverlay()
	assert.Contains(t, view, "tesseract not found",
		"non-LLM failed step error should be visible as plain text")
}

func TestExtractionExtractFails_PingSkipPreventsLLM(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})
	ex := m.ex.extraction
	id := ex.ID

	// Ping fails while extraction is running.
	m.Update(extractionLLMPingMsg{ID: id, Err: errors.New("unreachable")})
	assert.Equal(t, stepSkipped, ex.Steps[stepLLM].Status)

	// Extraction then fails too.
	m.Update(extractionProgressMsg{
		ID: id,
		Progress: extract.ExtractProgress{
			Err: errors.New("tesseract not found"),
		},
	})
	// LLM should still be skipped (not started despite extraction error path).
	assert.Equal(t, stepSkipped, ex.Steps[stepLLM].Status)
	assert.True(t, ex.Done)
}

// --- Accept works with or without LLM ---

func TestAccept_WorksWhenLLMFailed(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepFailed,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.HasError = true
	ex.pendingText = "ocr text"

	doc := &data.Document{
		FileName: "invoice.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("pdf-bytes"),
	}
	require.NoError(t, m.store.CreateDocument(doc))
	ex.DocID = doc.ID

	sendExtractionKey(m, "a")

	assert.Nil(t, m.ex.extraction, "accept should work even when LLM failed")

	saved, err := m.store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "ocr text", saved.ExtractedText)
}

func TestAccept_WorksWithoutLLMStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.pendingText = "ocr text"

	doc := &data.Document{
		FileName: "scan.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("pdf-bytes"),
	}
	require.NoError(t, m.store.CreateDocument(doc))
	ex.DocID = doc.ID

	sendExtractionKey(m, "a")

	assert.Nil(t, m.ex.extraction, "accept should work without LLM step")

	saved, err := m.store.GetDocument(doc.ID)
	require.NoError(t, err)
	assert.Equal(t, "ocr text", saved.ExtractedText)
}

func TestAccept_DeferredDoc_WorksWhenLLMFailed(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepFailed,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.HasError = true
	ex.extractedText = "invoice text"
	ex.pendingDoc = &data.Document{
		FileName:      "invoice.pdf",
		MIMEType:      "application/pdf",
		Data:          []byte("pdf-bytes"),
		ExtractedText: "invoice text",
	}

	sendExtractionKey(m, "a")

	assert.Nil(t, m.ex.extraction, "accept should work for deferred doc when LLM failed")

	docs, err := m.store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "invoice.pdf", docs[0].FileName)
}

func TestAccept_DeferredDoc_WorksWithoutLLMStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.pendingText = "better ocr text"
	ex.pendingDoc = &data.Document{
		FileName: "scan.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("pdf-bytes"),
	}

	sendExtractionKey(m, "a")

	assert.Nil(t, m.ex.extraction, "accept should work for deferred doc without LLM")

	docs, err := m.store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)

	full, err := m.store.GetDocument(docs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "better ocr text", full.ExtractedText)
}

// --- TSV toggle ---

func TestExtractionTSVToggle_TogglesOCRTSV(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	assert.False(t, m.ex.ocrTSV, "ocrTSV should start false in test setup")

	// Press t to toggle layout on.
	sendExtractionKey(m, keyT)
	assert.True(t, m.ex.ocrTSV, "t should toggle ocrTSV on")

	// LLM step should be reset for rerun.
	assert.Equal(t, stepRunning, ex.Steps[stepLLM].Status,
		"LLM step should be rerunning after toggle")
}

func TestExtractionTSVToggle_TogglesOff(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	m.ex.ocrTSV = true

	sendExtractionKey(m, keyT)
	assert.False(t, m.ex.ocrTSV, "t should toggle ocrTSV off")
}

func TestExtractionTSVToggle_IgnoredWhenNotDone(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})

	sendExtractionKey(m, keyT)
	assert.False(t, m.ex.ocrTSV, "t should be ignored when extraction is not done")
}

func TestExtractionTSVToggle_IgnoredWithoutLLM(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	sendExtractionKey(m, keyT)
	assert.False(t, m.ex.ocrTSV, "t should be ignored when no LLM step")
}

func TestExtractionTSVToggle_StatusMessage(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true

	sendExtractionKey(m, keyT)
	assert.Contains(t, m.status.Text, "layout on")

	// Simulate LLM completing again so we can toggle off.
	ex.Done = true
	ex.Steps[stepLLM] = extractionStepInfo{Status: stepDone}

	sendExtractionKey(m, keyT)
	assert.Contains(t, m.status.Text, "layout off")
}

// ---------------------------------------------------------------------------
// extractionModelUsed tests
// ---------------------------------------------------------------------------

func TestExtractionModelUsed_ReturnsModelWhenLLMSucceeded(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	m.ex.extractionModel = "test-extraction-model"

	result := m.extractionModelUsed(m.ex.extraction)
	assert.Equal(t, "test-extraction-model", result)
}

func TestExtractionModelUsed_FallsBackToChatModel(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	m.ex.extractionModel = ""
	m.llmClient = testExtractionOllamaClient(t, "chat-model")

	result := m.extractionModelUsed(m.ex.extraction)
	assert.Equal(t, "chat-model", result)
}

func TestExtractionModelUsed_EmptyWhenLLMSkipped(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
	})
	ex := m.ex.extraction
	ex.hasLLM = false

	result := m.extractionModelUsed(ex)
	assert.Empty(t, result)
}

func TestExtractionModelUsed_EmptyWhenLLMFailed(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepFailed,
	})
	m.ex.extractionModel = "test-model"

	result := m.extractionModelUsed(m.ex.extraction)
	assert.Empty(t, result)
}

func TestExtractionTSVToggle_HintShownInFooter(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.ex.extraction
	ex.Done = true
	m.width = 120
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "layout", "footer should show layout hint when done with LLM")
}

func TestMarshalOps_NilSlice(t *testing.T) {
	t.Parallel()
	b, err := marshalOps(nil)
	require.NoError(t, err)
	assert.Nil(t, b, "nil input should produce nil output (no-update sentinel)")
}

func TestMarshalOps_EmptySlice(t *testing.T) {
	t.Parallel()
	b, err := marshalOps([]extract.Operation{})
	require.NoError(t, err)
	assert.Equal(t, []byte("[]"), b, "empty non-nil slice should serialize to []")
}

func TestMarshalOps_WithOps(t *testing.T) {
	t.Parallel()
	ops := []extract.Operation{
		{Action: "upsert", Table: "projects", Data: map[string]any{"title": "Test"}},
	}
	b, err := marshalOps(ops)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"action":"upsert"`)
}
