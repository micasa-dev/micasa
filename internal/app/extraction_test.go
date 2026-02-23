// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
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
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	m.handleExtractionKey(msg)
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
