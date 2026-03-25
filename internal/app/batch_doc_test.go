// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchDocOverlayRenders(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))

	view := m.buildBatchDocOverlay()
	assert.Contains(t, view, "a.txt")
	assert.Contains(t, view, "Staged")
}

func TestShiftAOnDocsTabOpensBatchOverlay(t *testing.T) {
	m := newTestModelWithStore(t)
	createTestProject(t, m) // need at least one entity for the selector
	m.active = tabIndex(tabDocuments)
	sendKey(m, "i") // enter edit mode
	sendKey(m, "A") // Shift+A

	require.NotNil(t, m.batchDoc, "batch doc overlay should be active")
	// Top-level Docs tab has no scoped entity, so Task 13 starts in entity-selection phase.
	assert.Equal(t, batchPhaseEntity, m.batchDoc.phase)
	assert.NotNil(t, m.batchDoc.entitySelect)
	assert.NotNil(t, m.batchDoc.picker)
}

func TestBatchDocEscWithNoFilesCloses(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NotNil(t, m.batchDoc)

	sendKey(m, "esc")
	assert.Nil(t, m.batchDoc, "overlay should close on esc with no staged files")
}

func TestBatchDocTabTogglesFocus(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NotNil(t, m.batchDoc)
	assert.Equal(t, batchFocusPicker, m.batchDoc.focus)

	sendKey(m, "tab")
	assert.Equal(t, batchFocusList, m.batchDoc.focus)

	sendKey(m, "tab")
	assert.Equal(t, batchFocusPicker, m.batchDoc.focus)
}

func TestBatchDocStageFile(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.pdf")
	require.NoError(t, os.WriteFile(path, []byte("%PDF-fake"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NotNil(t, m.batchDoc)

	err := m.stageFile(path)
	require.NoError(t, err)

	require.Len(t, m.batchDoc.files, 1)
	sf := m.batchDoc.files[0]
	assert.Equal(t, "test.pdf", sf.Name)
	assert.Equal(t, int64(9), sf.Size)
	assert.NotEmpty(t, sf.Checksum)
	assert.NotEmpty(t, sf.MIME)
	assert.NoError(t, sf.Err)
}

func TestBatchDocStageDuplicatePathIgnored(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(path))
	require.NoError(t, m.stageFile(path))

	assert.Len(t, m.batchDoc.files, 1, "duplicate path should be ignored")
}

func TestBatchDocStageOversizedFileHasError(t *testing.T) {
	m := newTestModelWithStore(t)
	require.NoError(t, m.store.SetMaxDocumentSize(uint64(5)))

	tmp := t.TempDir()
	path := filepath.Join(tmp, "big.txt")
	require.NoError(t, os.WriteFile(path, []byte("too large file"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(path))

	require.Len(t, m.batchDoc.files, 1)
	assert.Error(t, m.batchDoc.files[0].Err, "oversized file should have error")
}

func TestBatchDocListNavigation(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0o600))
	}

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, m.stageFile(filepath.Join(tmp, name)))
	}

	bd := m.batchDoc
	bd.focus = batchFocusList
	assert.Equal(t, 0, bd.cursor)

	sendKey(m, "j")
	assert.Equal(t, 1, bd.cursor)

	sendKey(m, "j")
	assert.Equal(t, 2, bd.cursor)

	sendKey(m, "j") // at end, should not wrap
	assert.Equal(t, 2, bd.cursor)

	sendKey(m, "k")
	assert.Equal(t, 1, bd.cursor)
}

func TestBatchDocListRemove(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0o600))
	}

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	for _, name := range []string{"a.txt", "b.txt"} {
		require.NoError(t, m.stageFile(filepath.Join(tmp, name)))
	}

	bd := m.batchDoc
	bd.focus = batchFocusList
	bd.cursor = 0

	sendKey(m, "x") // remove first file
	require.Len(t, bd.files, 1)
	assert.Equal(t, "b.txt", bd.files[0].Name)
	assert.Equal(t, 0, bd.cursor) // cursor clamped
}

func TestBatchDocListEnterShowsFileDetails(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))

	bd := m.batchDoc
	bd.focus = batchFocusList

	sendKey(m, "enter")
	assert.Contains(t, m.statusView(), "a.txt")
}

func createTestProject(t *testing.T, m *Model) string {
	t.Helper()
	types, err := m.store.ProjectTypes()
	require.NoError(t, err)
	require.NotEmpty(t, types, "store should have seeded project types")
	p := data.Project{
		Title:         "Test Project",
		ProjectTypeID: types[0].ID,
		Status:        data.ProjectStatusPlanned,
	}
	require.NoError(t, m.store.CreateProject(&p))
	return p.ID
}

func TestBatchDocSubmitCreatesDocuments(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("world"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))
	require.NoError(t, m.stageFile(filepath.Join(tmp, "b.txt")))

	m.submitBatchDocuments()

	docs, err := m.store.ListDocumentsByEntity(data.DocumentEntityProject, projectID, false)
	require.NoError(t, err)
	assert.Len(t, docs, 2)
}

func TestBatchDocSubmitSkipsErroredFiles(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "good.txt"), []byte("content"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "good.txt")))

	// Manually add an errored file.
	m.batchDoc.files = append(m.batchDoc.files, stagedFile{
		Path: "/nonexistent",
		Name: "bad.txt",
		Err:  fmt.Errorf("unreadable"),
	})

	m.submitBatchDocuments()

	docs, err := m.store.ListDocumentsByEntity(data.DocumentEntityProject, projectID, false)
	require.NoError(t, err)
	assert.Len(t, docs, 1, "only the good file should be saved")
}

func TestBatchDocHasValidFilesTrue(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "ok.txt"), []byte("x"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "ok.txt")))

	assert.True(t, m.batchDocHasValidFiles())
}

func TestBatchDocHasValidFilesFalseAllErrored(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	m.batchDoc.files = append(m.batchDoc.files, stagedFile{
		Path: "/bad",
		Name: "bad.txt",
		Err:  fmt.Errorf("error"),
	})

	assert.False(t, m.batchDocHasValidFiles())
}

func TestBatchDocHasValidFilesFalseEmpty(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})

	assert.False(t, m.batchDocHasValidFiles())
}

func TestBatchDocCtrlSSubmitsAndAdvancesPhase(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("data"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "file.txt")))

	sendKey(m, "ctrl+s")

	require.NotNil(t, m.batchDoc)
	assert.Equal(
		t,
		batchPhaseExtracting,
		m.batchDoc.phase,
		"ctrl+s should advance phase to extracting",
	)

	docs, err := m.store.ListDocumentsByEntity(data.DocumentEntityProject, projectID, false)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

func TestBatchDocCtrlSNoOpWhenNoValidFiles(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	// No files staged.

	sendKey(m, "ctrl+s")

	// Phase should remain staging -- no submission happened.
	require.NotNil(t, m.batchDoc)
	assert.Equal(t, batchPhaseStaging, m.batchDoc.phase)
}

func TestBatchDocSubmitSetsDocID(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "report.pdf"), []byte("%PDF-x"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "report.pdf")))

	m.submitBatchDocuments()

	require.Len(t, m.batchDoc.files, 1)
	sf := m.batchDoc.files[0]
	require.NoError(t, sf.SubmitErr)
	assert.NotEmpty(t, sf.DocID, "DocID should be set after successful submission")
	// Data must NOT be released yet (extraction still needs it).
	assert.NotNil(t, sf.Data)
}

func TestBatchDocSubmitSetsPhaseExtracting(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "x.txt"), []byte("x"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "x.txt")))

	m.submitBatchDocuments()

	assert.Equal(t, batchPhaseExtracting, m.batchDoc.phase)
}

func TestBatchDocExtractionPersistsResults(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmp, "notes.txt"), []byte("plumbing inspection notes"), 0o600),
	)

	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "notes.txt")))

	m.submitBatchDocuments()

	require.NotNil(t, m.batchDoc)
	require.Equal(t, batchPhaseExtracting, m.batchDoc.phase)

	// Run the synchronous extraction helper (no event loop needed).
	m.runBatchExtraction()

	require.NotNil(t, m.batchDoc)
	assert.Equal(t, batchPhaseDone, m.batchDoc.phase)

	// The file data should be released.
	assert.Nil(t, m.batchDoc.files[0].Data)

	// Verify the document now has extracted text persisted.
	docID := m.batchDoc.files[0].DocID
	require.NotEmpty(t, docID)

	doc, err := m.store.GetDocument(docID)
	require.NoError(t, err)
	assert.NotEmpty(t, doc.ExtractedText, "extracted text should be persisted")
}

func TestBatchDocExtractionCtrlSStartsExtraction(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmp, "doc.txt"), []byte("some text content"), 0o600),
	)

	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "doc.txt")))

	// ctrl+s should submit and transition to extracting phase.
	sendKey(m, "ctrl+s")

	require.NotNil(t, m.batchDoc)
	assert.Equal(
		t,
		batchPhaseExtracting,
		m.batchDoc.phase,
		"phase should be extracting after ctrl+s",
	)
	// Context should be set (indicates startBatchExtraction ran).
	assert.NotNil(t, m.batchDoc.ctx, "extraction context should be set")
	assert.NotNil(t, m.batchDoc.cancelFn, "extraction cancel func should be set")
}

// ---------------------------------------------------------------------------
// Task 11: Extraction progress View rendering
// ---------------------------------------------------------------------------

func TestBatchDocExtractionProgressRenders(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	m.batchDoc.phase = batchPhaseExtracting
	m.batchDoc.files = []stagedFile{
		{Name: "a.pdf", DocID: "doc1"},
		{Name: "b.pdf", DocID: "doc2"},
		{Name: "c.pdf", DocID: "doc3", ExtractErr: fmt.Errorf("OCR failed")},
	}
	m.batchDoc.extractIdx = 1

	view := m.buildBatchDocOverlay()
	assert.Contains(t, view, "a.pdf")
	assert.Contains(t, view, "b.pdf")
	assert.Contains(t, view, "c.pdf")
}

func TestBatchDocDonePhaseRenders(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	m.batchDoc.phase = batchPhaseDone
	m.batchDoc.files = []stagedFile{
		{Name: "a.pdf", DocID: "doc1"},
		{Name: "b.pdf", DocID: "doc2", ExtractErr: fmt.Errorf("OCR failed")},
	}

	view := m.buildBatchDocOverlay()
	assert.Contains(t, view, "Upload Complete")
	assert.Contains(t, view, "a.pdf")
	assert.Contains(t, view, "b.pdf")
}

// ---------------------------------------------------------------------------
// Task 12: Done phase -- summary and dismiss
// ---------------------------------------------------------------------------

func TestBatchDocDonePhaseEnterDismisses(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	m.batchDoc.phase = batchPhaseDone

	sendKey(m, "enter")
	assert.Nil(t, m.batchDoc, "overlay should dismiss on enter in done phase")
}

func TestBatchDocDonePhaseEscDismisses(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	m.batchDoc.phase = batchPhaseDone

	sendKey(m, "esc")
	assert.Nil(t, m.batchDoc)
}

// ---------------------------------------------------------------------------
// Task 14: Cancel during extraction
// ---------------------------------------------------------------------------

func TestBatchDocEscDuringExtractionStopsAndShowsSummary(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	m.batchDoc.phase = batchPhaseExtracting
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m.batchDoc.ctx = ctx
	m.batchDoc.cancelFn = cancel
	m.batchDoc.files = []stagedFile{
		{Name: "a.txt", DocID: "doc1"},
		{Name: "b.txt", DocID: "doc2"},
	}
	m.batchDoc.extractIdx = 0

	sendKey(m, "esc")
	assert.Equal(t, batchPhaseDone, m.batchDoc.phase)
	// Context should be cancelled.
	assert.Error(t, ctx.Err())
}

// ---------------------------------------------------------------------------
// Task 13: Entity selector first page (top-level Docs tab)
// ---------------------------------------------------------------------------

func TestBatchDocTopLevelShowsEntitySelector(t *testing.T) {
	m := newTestModelWithStore(t)
	createTestProject(t, m) // need at least one entity for the selector
	m.startBatchDocOverlay(entityRef{})
	require.NotNil(t, m.batchDoc)
	assert.Equal(t, batchPhaseEntity, m.batchDoc.phase)
}

// ---------------------------------------------------------------------------
// Task 16: Confirmation dialog for cancel with staged files
// ---------------------------------------------------------------------------

func TestBatchDocEscWithStagedFilesShowsConfirmation(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))

	sendKey(m, "esc")
	// Should show confirmation, not close immediately.
	assert.NotNil(t, m.batchDoc, "overlay should NOT close yet")
}

func TestBatchDocConfirmDiscardYClosesOverlay(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))

	sendKey(m, "esc")
	require.NotNil(t, m.batchDoc)
	assert.Equal(t, confirmBatchDiscard, m.confirm)

	sendKey(m, "y")
	assert.Nil(t, m.batchDoc, "overlay should close after confirming discard")
	assert.Equal(t, confirmNone, m.confirm)
}

func TestBatchDocConfirmDiscardNKeepsOverlay(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))

	sendKey(m, "esc")
	require.NotNil(t, m.batchDoc)

	sendKey(m, "n")
	assert.NotNil(t, m.batchDoc, "overlay should stay after cancelling discard")
	assert.Equal(t, confirmNone, m.confirm)
}

// ---------------------------------------------------------------------------
// Task 17: Shift+H hidden file toggle in batch overlay
// ---------------------------------------------------------------------------

func TestBatchDocShiftHTogglesHiddenFiles(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NotNil(t, m.batchDoc)

	assert.False(t, filePickerShowHidden(m.batchDoc.picker))

	sendKey(m, "H") // Shift+H
	assert.True(t, filePickerShowHidden(m.batchDoc.picker))
}

// ---------------------------------------------------------------------------
// Task 15: Zone marking and mouse clickability
// ---------------------------------------------------------------------------

func TestBatchDocClickOnStagedFileSelectsIt(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0o600))
	}
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, m.stageFile(filepath.Join(tmp, name)))
	}

	z := requireZone(t, m, "batch-file-1")
	sendClick(m, z.StartX, z.StartY)
	assert.Equal(t, 1, m.batchDoc.cursor)
	assert.Equal(t, batchFocusList, m.batchDoc.focus)
}

// ---------------------------------------------------------------------------
// Task 18: Cumulative size warning and ctrl+s disabled hint
// ---------------------------------------------------------------------------

func TestBatchDocCumulativeSizeWarning(t *testing.T) {
	m := newTestModelWithStore(t)
	require.NoError(t, m.store.SetMaxDocumentSize(uint64(100)))

	tmp := t.TempDir()
	for i := range 12 {
		name := fmt.Sprintf("file%d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), make([]byte, 90), 0o600))
	}

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	for i := range 12 {
		require.NoError(t, m.stageFile(
			filepath.Join(tmp, fmt.Sprintf("file%d.txt", i)),
		))
	}

	view := m.buildBatchDocOverlay()
	assert.Contains(t, view, "total")
}

func TestBatchDocNoWarningBelowThreshold(t *testing.T) {
	m := newTestModelWithStore(t)
	require.NoError(t, m.store.SetMaxDocumentSize(uint64(1000)))

	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "small.txt"), []byte("x"), 0o600))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "small.txt")))

	view := m.buildBatchDocOverlay()
	assert.NotContains(t, view, "total")
}

// ---------------------------------------------------------------------------
// Task 19: Full integration test -- end-to-end flow
// ---------------------------------------------------------------------------

func TestBatchDocEndToEnd(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	for _, name := range []string{"invoice.txt", "receipt.txt"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(tmp, name),
			[]byte("content of "+name),
			0o600,
		))
	}

	// Open overlay from scoped view (known entity).
	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.Equal(t, batchPhaseStaging, m.batchDoc.phase)

	// Stage files.
	for _, name := range []string{"invoice.txt", "receipt.txt"} {
		require.NoError(t, m.stageFile(filepath.Join(tmp, name)))
	}
	require.Len(t, m.batchDoc.files, 2)

	// Submit.
	m.submitBatchDocuments()
	require.Equal(t, batchPhaseExtracting, m.batchDoc.phase)

	// Verify documents created in DB.
	docs, err := m.store.ListDocumentsByEntity(data.DocumentEntityProject, projectID, false)
	require.NoError(t, err)
	assert.Len(t, docs, 2)

	// Run extraction (synchronous test helper).
	m.runBatchExtraction()
	assert.Equal(t, batchPhaseDone, m.batchDoc.phase)

	// Verify extraction results persisted.
	for _, sf := range m.batchDoc.files {
		if sf.DocID != "" {
			doc, err := m.store.GetDocument(sf.DocID)
			require.NoError(t, err)
			assert.NotEmpty(t, doc.ExtractedText, "doc %s should have extracted text", sf.Name)
		}
	}

	// Dismiss overlay.
	sendKey(m, "enter")
	assert.Nil(t, m.batchDoc, "overlay should be dismissed")
}

func TestBatchDocExtractionDoneMsg(t *testing.T) {
	m := newTestModelWithStore(t)
	projectID := createTestProject(t, m)

	tmp := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmp, "readme.txt"), []byte("hello world"), 0o600),
	)

	m.startBatchDocOverlay(entityRef{Kind: data.DocumentEntityProject, ID: projectID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "readme.txt")))
	m.submitBatchDocuments()

	require.Len(t, m.batchDoc.files, 1)
	docID := m.batchDoc.files[0].DocID
	require.NotEmpty(t, docID)

	// Inject a synthetic batchExtractionDoneMsg with a result carrying plain text.
	result := &extract.Result{
		Sources: []extract.TextSource{
			{Tool: "plaintext", Text: "hello world"},
		},
	}
	sendMsg(m, batchExtractionDoneMsg{fileIdx: 0, result: result})

	require.NotNil(t, m.batchDoc)
	assert.Equal(t, batchPhaseDone, m.batchDoc.phase, "phase should advance to done")
	assert.Nil(t, m.batchDoc.files[0].Data, "file data should be released")

	// Text should be persisted to the store.
	doc, err := m.store.GetDocument(docID)
	require.NoError(t, err)
	assert.Equal(t, "hello world", doc.ExtractedText)
}
