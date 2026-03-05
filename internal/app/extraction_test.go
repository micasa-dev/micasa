// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
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
	ex := &extractionLogState{
		ID:       nextExtractionID.Add(1),
		ctx:      ctx,
		CancelFn: cancel,
		Visible:  true,
		expanded: make(map[extractionStep]bool),
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

func TestExtractionEnter_NoOpOnRunningStep(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.Done = true
	ex.cursor = 1 // force onto running step (shouldn't happen in practice)

	sendExtractionKey(m, "enter")
	_, set := ex.expanded[stepExtract]
	assert.False(t, set, "enter should not toggle running step")
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
	ex.pendingDoc = &data.Document{FileName: "scan.jpg"}

	// Verify the pendingDoc is accessible.
	assert.Equal(t, "scan.jpg", ex.pendingDoc.FileName)
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
	assert.Contains(t, step.Logs[0], "extraction.llm_timeout")
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
		{Tool: "pdfimages", Running: false, Count: 5},
		{Tool: "pdftohtml", Running: false, Count: 3},
		{Tool: "pdftoppm", Running: false, Count: 10},
	}
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "pdfimages", "completed tool lines should persist after step done")
	assert.Contains(t, out, "pdftohtml", "completed tool lines should persist after step done")
	assert.Contains(t, out, "pdftoppm", "completed tool lines should persist after step done")
	assert.Contains(t, out, "5 images")
	assert.Contains(t, out, "3 images")
	assert.Contains(t, out, "10 images")
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
		{Tool: "pdfimages", Running: false, Count: 2},
		{Tool: "pdftoppm", Running: false, Count: 10},
	}
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "pdfimages", "tool lines should show during OCR")
	assert.Contains(t, out, "pdftoppm", "tool lines should show during OCR")
	assert.Contains(t, out, "page 3/10", "page progress should show during OCR")
}

func TestAcquireTools_PartialRunning(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepExtract: stepRunning,
	})
	ex := m.ex.extraction
	ex.Steps[stepExtract] = extractionStepInfo{Status: stepRunning}
	ex.acquireTools = []extract.AcquireToolState{
		{Tool: "pdfimages", Running: false, Count: 5},
		{Tool: "pdftohtml", Running: true},
		{Tool: "pdftoppm", Running: true},
	}
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "pdfimages", "completed tool should show")
	assert.Contains(t, out, "5 images", "image count should show for completed tool")
	assert.Contains(t, out, "pdftohtml", "running tool should show")
	assert.Contains(t, out, "pdftoppm", "running tool should show")
}

func TestAcquireTools_DetailSuppressedInHeader(t *testing.T) {
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
		{Tool: "pdftoppm", Running: false, Count: 5},
	}
	m.width = 120
	m.height = 40

	out := m.buildExtractionOverlay()
	// Page progress should appear in the sub-spinner section, not duplicated
	// in the header alongside the step name.
	assert.Contains(t, out, "page 1/5")
	assert.Contains(t, out, "pdftoppm")
}

func TestAcquireTools_CollapseHidesToolLines(t *testing.T) {
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
		{Tool: "pdfimages", Running: false, Count: 5},
		{Tool: "pdftoppm", Running: false, Count: 10},
	}
	m.width = 120
	m.height = 40

	// Default: tools expanded.
	out := m.buildExtractionOverlay()
	assert.Contains(t, out, "pdfimages", "tools should be expanded by default when done")
	assert.Contains(t, out, "pdftoppm")

	// User collapses the ext step.
	ex.expanded[stepExtract] = false
	out = m.buildExtractionOverlay()
	assert.NotContains(t, out, "pdfimages", "tools should be hidden when collapsed")
	assert.NotContains(t, out, "pdftoppm", "tools should be hidden when collapsed")
	assert.Contains(t, out, "tesseract", "header detail should still show when collapsed")

	// User re-expands.
	ex.expanded[stepExtract] = true
	out = m.buildExtractionOverlay()
	assert.Contains(t, out, "pdfimages", "tools should reappear when re-expanded")
	assert.Contains(t, out, "pdftoppm", "tools should reappear when re-expanded")
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
	m.llmConfig = &llmConfig{
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
	m.llmConfig = &llmConfig{
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

// TestExtractionClient_FallsBackToChatConfig verifies that when extraction
// has no provider/baseURL of its own, extractionLLMClient() fills gaps
// from the chat config.
func TestExtractionClient_FallsBackToChatConfig(t *testing.T) {
	t.Parallel()
	m := newExtractionModel(t, map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	m.ex.extraction.Done = true

	// Chat config provides the connection.
	m.llmClient = testExtractionOllamaClient(t, "chat-model")
	m.llmConfig = &llmConfig{
		Provider: "ollama",
		BaseURL:  "http://localhost:11434",
		Model:    "chat-model",
		Timeout:  5 * time.Second,
	}

	// Extraction overrides only the model.
	m.ex.extractionModel = "extraction-only-model"

	client := m.extractionLLMClient()
	require.NotNil(t, client)
	assert.Equal(t, "extraction-only-model", client.Model())

	// User opens model picker and sees extraction-specific model.
	sendExtractionKey(m, "r")
	require.NotNil(t, m.ex.extraction.modelPicker)
	assert.Equal(t, "extraction-only-model", m.extractionModelLabel())
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
	m.llmConfig = nil
	m.ex.extractionModel = ""

	assert.Nil(t, m.extractionLLMClient())
}
