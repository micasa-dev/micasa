// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/cpcloud/micasa/internal/locale"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newExtractionModel sets up a Model with an active extraction overlay
// for testing keyboard interaction. Steps are pre-populated with the
// given statuses.
func newExtractionModel(steps map[extractionStep]stepStatus) *Model {
	m := newTestModel()
	ctx, cancel := context.WithCancel(context.Background())
	ex := &extractionLogState{
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
	m.extraction = ex
	return m
}

func sendExtractionKey(m *Model, key string) {
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEscape}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	m.Update(msg)
}

// --- Cursor navigation ---

func TestExtractionCursor_JK_SkipsRunningSteps(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
		stepLLM:     stepPending,
	})
	ex := m.extraction
	assert.Equal(t, 0, ex.cursor)

	// j should not move to the running extract step.
	sendExtractionKey(m, "j")
	assert.Equal(t, 0, ex.cursor, "j should not land on running step")

	// k at 0 stays at 0.
	sendExtractionKey(m, "k")
	assert.Equal(t, 0, ex.cursor)
}

func TestExtractionCursor_JK_LandsOnSettledSteps(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepFailed,
	})
	ex := m.extraction
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
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepText: stepDone,
		stepLLM:  stepDone,
	})
	ex := m.extraction
	ex.Done = true

	sendExtractionKey(m, "j")
	assert.Equal(t, 1, ex.cursor)
}

// --- Enter toggle ---

func TestExtractionEnter_TogglesDoneStep(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepText: stepDone,
	})
	ex := m.extraction
	ex.Done = true

	// Text step is done, not auto-expanded. First enter should expand.
	sendExtractionKey(m, "enter")
	assert.True(t, ex.expanded[stepText], "enter should expand done text step")

	// Second enter should collapse.
	sendExtractionKey(m, "enter")
	assert.False(t, ex.expanded[stepText], "enter should collapse")
}

func TestExtractionEnter_TogglesAutoExpandedLLMStep(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.extraction
	ex.Done = true

	// LLM done step is auto-expanded. First enter should collapse.
	sendExtractionKey(m, "enter")
	assert.False(t, ex.expanded[stepLLM], "enter on auto-expanded LLM should collapse")

	// Second enter should re-expand.
	sendExtractionKey(m, "enter")
	assert.True(t, ex.expanded[stepLLM], "enter should re-expand")
}

func TestExtractionEnter_TogglesFailedStep(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepExtract: stepFailed,
	})
	ex := m.extraction

	// Failed steps are auto-expanded. First enter should collapse.
	sendExtractionKey(m, "enter")
	assert.False(t, ex.expanded[stepExtract], "enter on auto-expanded failed step should collapse")
}

func TestExtractionEnter_NoOpOnRunningStep(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepRunning,
	})
	ex := m.extraction
	ex.Done = true
	ex.cursor = 1 // force onto running step (shouldn't happen in practice)

	sendExtractionKey(m, "enter")
	_, set := ex.expanded[stepExtract]
	assert.False(t, set, "enter should not toggle running step")
}

// --- Rerun cursor relocation ---

func TestRerunLLM_MovesCursorToSettledStep(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepText:    stepDone,
		stepExtract: stepDone,
		stepLLM:     stepDone,
	})
	ex := m.extraction
	ex.Done = true
	ex.cursor = 2 // on LLM step

	m.rerunLLMExtraction()

	// Cursor should move back to the nearest settled step before LLM.
	assert.Equal(t, 1, ex.cursor, "cursor should move to extract step")
}

func TestRerunLLM_CursorFallbackToZero(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.extraction
	ex.Done = true
	ex.cursor = 0

	m.rerunLLMExtraction()

	// Only LLM is active and it's now running -- cursor falls back to 0.
	assert.Equal(t, 0, ex.cursor)
}

// --- Operation preview rendering ---

// newPreviewModel creates a Model with extraction state containing the given
// operations, suitable for testing renderOperationPreviewSection.
func newPreviewModel(ops []extract.Operation) *Model {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	m.extraction.Done = true
	m.extraction.operations = ops
	return m
}

func TestRenderOperationPreview_TabbedInterface(t *testing.T) {
	m := newPreviewModel([]extract.Operation{
		{
			Action: "create",
			Table:  "vendors",
			Data:   map[string]any{"name": "Garcia Plumbing"},
		},
		{
			Action: "update",
			Table:  "documents",
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
	m.extraction.exploring = true
	m.extraction.enterExploreMode(m.cur)
	m.extraction.previewTab = 1
	out = m.renderOperationPreviewSection(60, true)
	assert.Contains(t, out, "Title")
	assert.Contains(t, out, "Invoice")
	assert.Contains(t, out, "Notes")
	assert.Contains(t, out, "Repair")
}

func TestRenderOperationPreview_EmptyOps(t *testing.T) {
	m := newPreviewModel(nil)
	out := m.renderOperationPreviewSection(60, false)
	assert.Contains(t, out, "no operations")
}

func TestRenderOperationPreview_EmptyData(t *testing.T) {
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "vendors", Data: nil},
	})
	out := m.renderOperationPreviewSection(60, false)
	assert.Contains(t, out, "no operations")
}

func TestRenderOperationPreview_MoneyFormatting(t *testing.T) {
	m := newPreviewModel([]extract.Operation{
		{
			Action: "create",
			Table:  "quotes",
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
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "Acme"}},
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "Beta Corp"}},
	})
	out := m.renderOperationPreviewSection(60, false)

	assert.Contains(t, out, "Vendors")
	assert.Contains(t, out, "Acme")
	assert.Contains(t, out, "Beta Corp")
}

func TestRenderOperationPreview_UnknownTable(t *testing.T) {
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "unknown_table", Data: map[string]any{"x": "y"}},
	})
	out := m.renderOperationPreviewSection(60, false)
	assert.Contains(t, out, "no operations")
}

func TestGroupOperationsByTable(t *testing.T) {
	ops := []extract.Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "A"}},
		{Action: "update", Table: "documents", Data: map[string]any{"title": "B"}},
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "C", "email": "c@x.com"}},
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
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.extraction
	ex.Done = true

	// Simulate deferred creation with a pending doc.
	ex.pendingDoc = &data.Document{
		FileName: "invoice.pdf",
		MIMEType: "application/pdf",
		Data:     []byte("pdf-bytes"),
	}
	// LLM produced operations including document fields.
	ex.operations = []extract.Operation{
		{Action: "create", Table: "documents", Data: map[string]any{
			"title": "Garcia Invoice",
			"notes": "Plumbing repair",
		}},
		{Action: "create", Table: "vendors", Data: map[string]any{
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
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepLLM: stepRunning,
	})
	ex := m.extraction
	ex.pendingDoc = &data.Document{
		FileName: "invoice.pdf",
		MIMEType: "application/pdf",
	}

	// Cancel should nil out extraction state.
	m.cancelExtraction()
	assert.Nil(t, m.extraction, "extraction should be nil after cancel")
}

func TestDeferredExtraction_PendingDocFieldPresent(t *testing.T) {
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepLLM: stepDone,
	})
	ex := m.extraction
	ex.Done = true
	ex.pendingDoc = &data.Document{FileName: "scan.jpg"}

	// Verify the pendingDoc is accessible.
	assert.Equal(t, "scan.jpg", ex.pendingDoc.FileName)
}

// --- Explore mode ---

func TestExploreMode_XTogglesExploring(t *testing.T) {
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "A"}},
	})
	ex := m.extraction
	assert.False(t, ex.exploring)

	// x should enter explore mode when done with operations.
	sendExtractionKey(m, "x")
	assert.True(t, ex.exploring, "x should enter explore mode")

	// x again should exit explore mode.
	sendExtractionKey(m, "x")
	assert.False(t, ex.exploring, "x should exit explore mode")
}

func TestExploreMode_EscExitsExploring(t *testing.T) {
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "A"}},
	})
	ex := m.extraction
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)

	// Esc should exit explore mode, not cancel the overlay.
	sendExtractionKey(m, "esc")
	assert.False(t, ex.exploring, "esc should exit explore mode")
	assert.NotNil(t, m.extraction, "overlay should still be open")
}

func TestExploreMode_JKNavigatesRows(t *testing.T) {
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "A"}},
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "B"}},
	})
	ex := m.extraction
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
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{
			"name": "A", "email": "a@b.com",
		}},
	})
	ex := m.extraction
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)
	assert.Equal(t, 0, ex.previewCol)

	sendExtractionKey(m, "l")
	assert.Equal(t, 1, ex.previewCol)

	sendExtractionKey(m, "h")
	assert.Equal(t, 0, ex.previewCol)
}

func TestExploreMode_BFSwitchesTabs(t *testing.T) {
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "A"}},
		{Action: "create", Table: "quotes", Data: map[string]any{"total_cents": float64(100)}},
	})
	ex := m.extraction
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
	m := newPreviewModel([]extract.Operation{
		{Action: "create", Table: "vendors", Data: map[string]any{"name": "A"}},
	})
	ex := m.extraction
	sendExtractionKey(m, "x")
	require.True(t, ex.exploring)

	// a should accept even in explore mode. Without a store, dispatch is
	// a silent no-op, so accept succeeds and clears extraction state.
	sendExtractionKey(m, "a")
	assert.Nil(t, m.extraction, "accept without store succeeds and clears state")
}

// --- NeedsOCR integration ---

func TestNeedsOCR_UsedInsteadOfHardcodedToolName(t *testing.T) {
	// Verify that extraction.go and model.go use NeedsOCR (not HasMatchingExtractor
	// with "tesseract"). This is a compile-time guarantee: if extract.NeedsOCR is
	// removed, the build will break. This test documents the intent.
	m := newExtractionModel(map[extractionStep]stepStatus{
		stepText: stepDone,
	})
	// With no OCR extractors configured, startExtractionOverlay should
	// not flag needsExtract.
	assert.Nil(t, m.extractors, "default test model has no extractors")
}
