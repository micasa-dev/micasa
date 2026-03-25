<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Multi-Document Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow selecting and uploading multiple documents at once through a staging overlay triggered by `Shift+A` on the Docs tab.

**Architecture:** New overlay state struct (`batchDocState`) following the `extractionLogState` pattern -- self-contained with its own Update/View, embedding a bare `huh.FilePicker` (not in a `huh.Form`). The overlay is gated by `m.batchDoc != nil` (same pointer-nil pattern as `m.ex.extraction`), not a new `Mode` constant -- Mode controls normal/edit/form transitions, while overlays like extraction and batch-doc compose on top of any mode. Two-phase flow: staging (pick files, build list) then batch submission + async sequential extraction via `tea.Cmd` message passing.

**Tech Stack:** Go, Bubble Tea, lipgloss, huh (FilePicker only), GORM/SQLite

**Spec:** `plans/multi-document-upload.md`

---

### Task 1: Store method -- DocumentExistsByChecksum

**Files:**
- Modify: `internal/data/store.go` (add method near line 1276, after ListDocumentsByEntity)
- Test: `internal/data/store_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestDocumentExistsByChecksum(t *testing.T) {
	s := newTestStore(t)
	house := data.HouseProfile{Nickname: "Test"}
	require.NoError(t, s.CreateHouseProfile(&house))
	proj := data.Project{Title: "Roof", HouseProfileID: house.ID}
	require.NoError(t, s.CreateProject(&proj))

	doc := data.Document{
		Title:          "invoice",
		FileName:       "invoice.pdf",
		EntityKind:     "project",
		EntityID:       proj.ID,
		MIMEType:       "application/pdf",
		SizeBytes:      100,
		ChecksumSHA256: "abc123deadbeef",
		Data:           []byte("fake"),
	}
	require.NoError(t, s.CreateDocument(&doc))

	// Same entity, same checksum → true
	assert.True(t, s.DocumentExistsByChecksum("project", proj.ID, "abc123deadbeef"))
	// Same entity, different checksum → false
	assert.False(t, s.DocumentExistsByChecksum("project", proj.ID, "different"))
	// Different entity, same checksum → false
	assert.False(t, s.DocumentExistsByChecksum("appliance", "nonexistent", "abc123deadbeef"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestDocumentExistsByChecksum ./internal/data/`
Expected: compilation error -- method not defined

- [ ] **Step 3: Implement DocumentExistsByChecksum**

Add to `internal/data/store.go` after `ListDocumentsByEntity`:

```go
// DocumentExistsByChecksum reports whether a document with the given SHA256
// checksum already exists for the specified entity.
func (s *Store) DocumentExistsByChecksum(entityKind, entityID, checksum string) bool {
	var count int64
	s.db.Model(&Document{}).
		Where("entity_kind = ? AND entity_id = ? AND sha256 = ?", entityKind, entityID, checksum).
		Count(&count)
	return count > 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestDocumentExistsByChecksum ./internal/data/`
Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 2: Refactor file picker helpers to accept bare *huh.FilePicker

**Files:**
- Modify: `internal/app/form_filepicker.go` (refactor helpers)
- Modify: `internal/app/forms.go` (update call sites)
- Test: `internal/app/form_filepicker_test.go` (new)

Currently `syncFilePickerDescription`, `syncFilePickerTitle`, `filePickerCurrentDir`, and `filePickerShowHidden` operate on `huh.Field` or `*huh.Form`. They need to also work with a bare `*huh.FilePicker` so the batch overlay can reuse them.

- [ ] **Step 1: Write a test for the refactored helpers**

Create `internal/app/form_filepicker_test.go`:

```go
func TestFilePickerCurrentDirFromPicker(t *testing.T) {
	fp := huh.NewFilePicker().CurrentDirectory("/tmp").Picking(true)
	fp.Init()
	dir := filePickerCurrentDirFromPicker(fp)
	assert.Equal(t, "/tmp", dir)
}

func TestFilePickerShowHiddenFromPicker(t *testing.T) {
	fp := huh.NewFilePicker().ShowHidden(false).Picking(true)
	fp.Init()
	assert.False(t, filePickerShowHiddenFromPicker(fp))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestFilePicker.*FromPicker ./internal/app/`
Expected: compilation error -- functions not defined

- [ ] **Step 3: Refactor the helpers**

In `form_filepicker.go`, extract the core logic into functions that accept `*huh.FilePicker` directly. Keep the existing `huh.Field`/`*huh.Form` wrappers calling the new functions:

```go
// filePickerShowHiddenFromPicker reads the unexported showHidden field from a
// bare *huh.FilePicker via reflection.
func filePickerShowHiddenFromPicker(fp *huh.FilePicker) bool {
	// ... reflection on fp directly ...
}

// filePickerShowHidden reads showHidden from a huh.Field (form-embedded picker).
func filePickerShowHidden(field huh.Field) bool {
	fp := extractFilePicker(field)
	if fp == nil {
		return false
	}
	return filePickerShowHiddenFromPicker(fp)
}

// filePickerCurrentDirFromPicker reads the current directory from a bare picker.
func filePickerCurrentDirFromPicker(fp *huh.FilePicker) string {
	// ... reflection on fp directly ...
}
```

Similarly refactor `syncFilePickerDescription` and `syncFilePickerTitle` to have
`*huh.FilePicker`-accepting variants that the batch overlay can call.

- [ ] **Step 4: Run all tests to verify nothing broke**

Run: `go test -run TestFilePicker ./internal/app/`
Expected: PASS (both new and existing tests)

- [ ] **Step 5: Verify existing form tests still pass**

Run: `go test ./internal/app/ -count=1 -shuffle=on`
Expected: PASS

- [ ] **Step 6: Commit**

`/commit`

---

### Task 3: Define batchDocState and stagedFile types

**Files:**
- Create: `internal/app/batch_doc.go`
- Modify: `internal/app/types.go` (add field to Model or a sub-struct)

- [ ] **Step 1: Create the state types**

Create `internal/app/batch_doc.go` with the core types:

```go
package app

import (
	"github.com/charmbracelet/huh"
)

// batchDocFocus tracks which zone has keyboard focus in the staging overlay.
type batchDocFocus int

const (
	batchFocusPicker batchDocFocus = iota
	batchFocusList
)

// batchDocPhase tracks the overlay's lifecycle phase.
type batchDocPhase int

const (
	batchPhaseEntity  batchDocPhase = iota // top-level: pick entity first
	batchPhaseStaging                      // pick files, build staged list
	batchPhaseUploading                    // saving documents to DB
	batchPhaseExtracting                   // running extraction pipeline
	batchPhaseDone                         // summary view
)

// stagedFile holds eagerly-read data for a file queued for upload.
type stagedFile struct {
	Path     string // absolute path, used for dedup
	Name     string // base filename
	Data     []byte // file contents (read at stage time)
	Size     int64
	MIME     string // detected at stage time
	Checksum string // SHA256 hex, computed at stage time
	Err      error  // non-nil if read/checksum/size-limit failed

	// Set after submission.
	DocID      string // assigned after CreateDocument succeeds
	SubmitErr  error  // non-nil if CreateDocument failed
	ExtractErr error  // non-nil if extraction failed
}

// batchDocState manages the multi-document staging overlay.
type batchDocState struct {
	phase  batchDocPhase
	focus  batchDocFocus
	entity entityRef // locked entity for all docs in the batch

	// Staging phase.
	picker *huh.FilePicker
	files  []stagedFile
	cursor int  // cursor position in staged files list
	dirty  bool // true if files are staged (used for cancel confirmation)

	// Extraction phase.
	extractIdx int // index into files currently being extracted
}
```

- [ ] **Step 2: Add batchDoc field to Model**

In `internal/app/types.go`, add to the Model struct (near the `ex extractState`
field):

```go
batchDoc *batchDocState // non-nil when batch upload overlay is active
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/app/`
Expected: success (no references to the new types yet beyond the field)

- [ ] **Step 4: Commit**

`/commit`

---

### Task 4: Staging overlay activation -- startBatchDocOverlay

**Files:**
- Modify: `internal/app/batch_doc.go` (add activation method)
- Modify: `internal/app/forms.go` (redirect Shift+A)
- Test: `internal/app/batch_doc_test.go` (new)

- [ ] **Step 1: Write the failing test for overlay activation**

Create `internal/app/batch_doc_test.go`:

```go
func TestShiftAOnDocsTabOpensBatchOverlay(t *testing.T) {
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabDocuments)
	sendKey(m, "i") // enter edit mode
	sendKey(m, "A") // Shift+A

	require.NotNil(t, m.batchDoc, "batch doc overlay should be active")
	assert.Equal(t, batchPhaseStaging, m.batchDoc.phase)
	assert.NotNil(t, m.batchDoc.picker)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestShiftAOnDocsTabOpensBatchOverlay ./internal/app/`
Expected: FAIL -- batchDoc is nil

- [ ] **Step 3: Implement startBatchDocOverlay**

In `internal/app/batch_doc.go`:

```go
// startBatchDocOverlay initializes the multi-document staging overlay.
// If entity is zero, the overlay starts in the entity-selection phase.
// Returns a tea.Cmd from the file picker's Init (must not be dropped).
func (m *Model) startBatchDocOverlay(entity entityRef) tea.Cmd {
	phase := batchPhaseStaging
	if entity.Kind == "" {
		phase = batchPhaseEntity
	}
	fp := m.newDocumentFilePicker("Select files")
	initCmd := fp.Init()
	m.batchDoc = &batchDocState{
		phase:  phase,
		focus:  batchFocusPicker,
		entity: entity,
		picker: fp,
	}
	return initCmd
}
```

The caller (Shift+A dispatch) must return the `tea.Cmd` so the picker's
startup I/O fires. Test helpers that call `startBatchDocOverlay` directly
can ignore the returned cmd since tests don't run the Bubble Tea event loop.

- [ ] **Step 4: Wire Shift+A to the new overlay**

In `internal/app/forms.go`, modify `startQuickDocumentForm` (or wherever
`keyShiftA` dispatches on the Documents tab) to call `startBatchDocOverlay`
instead. Pass the scoped entity if in a detail view, or zero `entityRef{}` if
on the top-level Docs tab.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -run TestShiftAOnDocsTabOpensBatchOverlay ./internal/app/`
Expected: PASS

- [ ] **Step 6: Commit**

`/commit`

---

### Task 5: Staging overlay key handling -- Update dispatch

**Files:**
- Modify: `internal/app/batch_doc.go` (add Update handler)
- Modify: `internal/app/update.go` or wherever key dispatch happens (route keys to batch overlay)
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing tests for key handling**

```go
func TestBatchDocEscWithNoFilesCloses(t *testing.T) {
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabDocuments)
	sendKey(m, "i")
	sendKey(m, "A")
	require.NotNil(t, m.batchDoc)

	sendKey(m, "esc")
	assert.Nil(t, m.batchDoc, "overlay should close on esc with no staged files")
}

func TestBatchDocTabTogglesFocus(t *testing.T) {
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabDocuments)
	sendKey(m, "i")
	sendKey(m, "A")
	require.NotNil(t, m.batchDoc)
	assert.Equal(t, batchFocusPicker, m.batchDoc.focus)

	sendKey(m, "tab")
	assert.Equal(t, batchFocusList, m.batchDoc.focus)

	sendKey(m, "tab")
	assert.Equal(t, batchFocusPicker, m.batchDoc.focus)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestBatchDoc ./internal/app/`
Expected: FAIL

- [ ] **Step 3: Implement handleBatchDocKey**

In `internal/app/batch_doc.go`:

```go
func (m *Model) handleBatchDocKey(msg tea.KeyPressMsg) tea.Cmd {
	bd := m.batchDoc
	key := msg.String()

	switch key {
	case keyEsc:
		return m.handleBatchDocEsc()
	case keyTab:
		if bd.focus == batchFocusPicker {
			bd.focus = batchFocusList
		} else {
			bd.focus = batchFocusPicker
		}
		return nil
	}

	switch bd.focus {
	case batchFocusPicker:
		return m.handleBatchDocPickerKey(msg)
	case batchFocusList:
		return m.handleBatchDocListKey(msg)
	}
	return nil
}
```

- [ ] **Step 4: Wire into main Update dispatch**

In the main key dispatch (likely `internal/app/update.go` or `model.go`), add
a check: if `m.batchDoc != nil`, route to `handleBatchDocKey` before other
handlers.

- [ ] **Step 5: Implement handleBatchDocEsc**

```go
func (m *Model) handleBatchDocEsc() tea.Cmd {
	if len(m.batchDoc.files) > 0 {
		// Show confirmation -- reuse existing confirm pattern
		m.confirm = confirmBatchDiscard
		return nil
	}
	m.batchDoc = nil
	return nil
}
```

Add `confirmBatchDiscard` constant to the confirmation enum.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test -run TestBatchDoc ./internal/app/`
Expected: PASS

- [ ] **Step 7: Commit**

`/commit`

---

### Task 6: File staging -- enter key stages a file

**Files:**
- Modify: `internal/app/batch_doc.go` (add stageFile logic)
- Test: `internal/app/batch_doc_test.go`

This is the core staging logic: when the picker has a selected file and the
user presses `enter`, read the file eagerly and append to the staged list.

- [ ] **Step 1: Write failing tests for file staging**

```go
func TestBatchDocStageFile(t *testing.T) {
	m := newTestModelWithStore(t)

	// Create a temp file to stage
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.pdf")
	require.NoError(t, os.WriteFile(path, []byte("%PDF-fake"), 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NotNil(t, m.batchDoc)

	// Simulate staging a file
	err := m.stageFile(path)
	require.NoError(t, err)

	require.Len(t, m.batchDoc.files, 1)
	sf := m.batchDoc.files[0]
	assert.Equal(t, path, sf.Path)
	assert.Equal(t, "test.pdf", sf.Name)
	assert.Equal(t, int64(9), sf.Size) // len("%PDF-fake")
	assert.NotEmpty(t, sf.Checksum)
	assert.NotEmpty(t, sf.MIME)
	assert.Nil(t, sf.Err)
}

func TestBatchDocStageDuplicatePathIgnored(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(path))
	require.NoError(t, m.stageFile(path)) // duplicate

	assert.Len(t, m.batchDoc.files, 1, "duplicate path should be ignored")
}

func TestBatchDocStageOversizedFileHasError(t *testing.T) {
	m := newTestModelWithStore(t)
	require.NoError(t, m.store.SetMaxDocumentSize(uint64(5))) // 5 bytes max

	tmp := t.TempDir()
	path := filepath.Join(tmp, "big.txt")
	require.NoError(t, os.WriteFile(path, []byte("too large file"), 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(path))

	require.Len(t, m.batchDoc.files, 1)
	assert.NotNil(t, m.batchDoc.files[0].Err, "oversized file should have error")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestBatchDocStage ./internal/app/`
Expected: compilation error -- stageFile not defined

- [ ] **Step 3: Implement stageFile**

```go
// stageFile reads a file from disk, computes metadata, and appends it to the
// staging list. Returns nil even for invalid files (the error is captured in
// stagedFile.Err). Returns an error only for programming bugs.
func (m *Model) stageFile(path string) error {
	bd := m.batchDoc
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}

	// Deduplicate by absolute path.
	for _, f := range bd.files {
		if f.Path == absPath {
			return nil
		}
	}

	sf := stagedFile{
		Path: absPath,
		Name: filepath.Base(absPath),
	}

	// Read file eagerly.
	info, err := os.Stat(absPath)
	if err != nil {
		sf.Err = fmt.Errorf("cannot read: %w", err)
		bd.files = append(bd.files, sf)
		bd.dirty = true
		return nil
	}
	sf.Size = info.Size()

	// Check size limit.
	if sf.Size > 0 && uint64(sf.Size) > m.store.MaxDocumentSize() {
		sf.Err = fmt.Errorf("file too large (%s)", humanize.IBytes(uint64(sf.Size)))
		bd.files = append(bd.files, sf)
		bd.dirty = true
		return nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		sf.Err = fmt.Errorf("read failed: %w", err)
		bd.files = append(bd.files, sf)
		bd.dirty = true
		return nil
	}

	sf.Data = data
	sf.MIME = detectMIMEType(absPath, data)
	sf.Checksum = fmt.Sprintf("%x", sha256.Sum256(data))

	// Check for duplicate checksum on the target entity.
	if bd.entity.Kind != "" && m.store.DocumentExistsByChecksum(
		bd.entity.Kind, bd.entity.ID, sf.Checksum,
	) {
		sf.Err = fmt.Errorf("duplicate: same file already attached")
	}

	bd.files = append(bd.files, sf)
	bd.dirty = true
	return nil
}
```

- [ ] **Step 4: Wire enter key in picker to call stageFile**

In `handleBatchDocPickerKey`, intercept `enter` when the picker has a selected
file (not a directory). Read the selected path from the picker, call
`stageFile`, and re-create the picker at the same directory.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run TestBatchDocStage ./internal/app/`
Expected: PASS

- [ ] **Step 6: Commit**

`/commit`

---

### Task 7: Staged list navigation -- j/k/x/d keys

**Files:**
- Modify: `internal/app/batch_doc.go` (add list key handler)
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing tests for list navigation**

```go
func TestBatchDocListNavigation(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0644))
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
		require.NoError(t, os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0644))
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
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))

	bd := m.batchDoc
	bd.focus = batchFocusList

	sendKey(m, "enter")
	assert.Contains(t, m.statusView(), "a.txt")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestBatchDocList ./internal/app/`
Expected: FAIL

- [ ] **Step 3: Implement handleBatchDocListKey**

```go
func (m *Model) handleBatchDocListKey(msg tea.KeyPressMsg) tea.Cmd {
	bd := m.batchDoc
	key := msg.String()

	switch key {
	case keyJ, keyDown:
		if bd.cursor < len(bd.files)-1 {
			bd.cursor++
		}
	case keyK, keyUp:
		if bd.cursor > 0 {
			bd.cursor--
		}
	case keyX, keyD:
		if len(bd.files) > 0 {
			bd.files = append(bd.files[:bd.cursor], bd.files[bd.cursor+1:]...)
			if bd.cursor >= len(bd.files) && bd.cursor > 0 {
				bd.cursor--
			}
			bd.dirty = len(bd.files) > 0
		}
	case keyEnter:
		// Show file details in status bar (name, size, MIME, checksum, error).
		if bd.cursor < len(bd.files) {
			sf := bd.files[bd.cursor]
			detail := fmt.Sprintf("%s  %s  %s", sf.Name,
				humanize.IBytes(uint64(sf.Size)), sf.MIME)
			if sf.Err != nil {
				detail += "  " + sf.Err.Error()
			}
			m.setStatusInfo(detail)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestBatchDocList ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 8: Staging overlay View rendering

**Files:**
- Modify: `internal/app/batch_doc.go` (add View method)
- Modify: `internal/app/view.go` (compose batch overlay into main view)
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write a test that the overlay renders**

```go
func TestBatchDocOverlayRenders(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))

	view := m.buildBatchDocOverlay()
	assert.Contains(t, view, "a.txt")
	assert.Contains(t, view, "Staged")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestBatchDocOverlayRenders ./internal/app/`
Expected: compilation error

- [ ] **Step 3: Implement buildBatchDocOverlay**

In `internal/app/batch_doc.go`:

```go
func (m *Model) buildBatchDocOverlay() string {
	bd := m.batchDoc
	if bd == nil {
		return ""
	}

	var sections []string

	// Title.
	title := "Add Documents"
	if bd.entity.Kind != "" {
		title += " to " + bd.entity.Kind + " " + bd.entity.ID
	}

	// File picker (top zone).
	pickerView := bd.picker.View()
	if bd.focus != batchFocusPicker {
		pickerView = appStyles.TextDim().Render(pickerView)
	}
	sections = append(sections, pickerView)

	// Staged files list (bottom zone).
	if len(bd.files) > 0 {
		header := fmt.Sprintf("Staged (%d)", len(bd.files))
		// ... render each file row with cursor indicator, name, size, MIME ...
		// ... highlight current cursor position ...
		// ... show error badge for files with Err != nil ...
		sections = append(sections, listView)
	}

	// Hint line.
	hints := "[enter] add  [ctrl+s] upload all  [x] remove  [esc] cancel"
	sections = append(sections, appStyles.TextDim().Render(hints))

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return m.styles.OverlayBox().Render(content)
}
```

- [ ] **Step 4: Wire into main View**

In `internal/app/view.go`, add the batch doc overlay to the overlay compositing
logic (similar to how `buildExtractionOverlay` is composed):

```go
if m.batchDoc != nil {
	overlay := m.buildBatchDocOverlay()
	// ... compose with zone marking ...
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run TestBatchDocOverlay ./internal/app/`
Expected: PASS

- [ ] **Step 6: Commit**

`/commit`

---

### Task 9: Batch submission -- ctrl+s creates documents

**Files:**
- Modify: `internal/app/batch_doc.go` (add submission logic)
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing tests for batch submission**

```go
func TestBatchDocCtrlSCreatesDocuments(t *testing.T) {
	m := newTestModelWithStore(t)
	house := requireHouse(t, m)
	proj := data.Project{Title: "Test", HouseProfileID: house.ID}
	require.NoError(t, m.store.CreateProject(&proj))

	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("world"), 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: proj.ID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))
	require.NoError(t, m.stageFile(filepath.Join(tmp, "b.txt")))

	m.submitBatchDocuments()

	docs, err := m.store.ListDocumentsByEntity("project", proj.ID, false)
	require.NoError(t, err)
	assert.Len(t, docs, 2)

	// Check titles derived from filenames.
	titles := []string{docs[0].Title, docs[1].Title}
	assert.Contains(t, titles, "a")
	assert.Contains(t, titles, "b")
}

func TestBatchDocCtrlSSkipsErroredFiles(t *testing.T) {
	m := newTestModelWithStore(t)
	house := requireHouse(t, m)
	proj := data.Project{Title: "Test", HouseProfileID: house.ID}
	require.NoError(t, m.store.CreateProject(&proj))

	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "good.txt"), []byte("ok"), 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: proj.ID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "good.txt")))

	// Manually add an errored file.
	m.batchDoc.files = append(m.batchDoc.files, stagedFile{
		Path: "/nonexistent",
		Name: "bad.txt",
		Err:  fmt.Errorf("unreadable"),
	})

	m.submitBatchDocuments()

	docs, err := m.store.ListDocumentsByEntity("project", proj.ID, false)
	require.NoError(t, err)
	assert.Len(t, docs, 1, "only the good file should be saved")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestBatchDocCtrlS ./internal/app/`
Expected: compilation error

- [ ] **Step 3: Implement submitBatchDocuments**

```go
// submitBatchDocuments creates a Document record for each staged file without
// errors. Sets DocID on success or SubmitErr on failure for each file.
func (m *Model) submitBatchDocuments() {
	bd := m.batchDoc
	for i := range bd.files {
		sf := &bd.files[i]
		if sf.Err != nil {
			continue
		}

		doc := data.Document{
			Title:          data.TitleFromFilename(sf.Name),
			FileName:       sf.Name,
			EntityKind:     bd.entity.Kind,
			EntityID:       bd.entity.ID,
			MIMEType:       sf.MIME,
			SizeBytes:      sf.Size,
			ChecksumSHA256: sf.Checksum,
			Data:           sf.Data,
		}

		if err := m.store.CreateDocument(&doc); err != nil {
			sf.SubmitErr = err
			continue
		}
		sf.DocID = doc.ID
		// Note: do NOT release sf.Data here -- extraction still needs it.
		// Data is released after extraction completes for this file.
	}
	bd.phase = batchPhaseExtracting
}
```

- [ ] **Step 4: Wire ctrl+s in handleBatchDocKey**

```go
case keyCtrlS:
	if bd.phase == batchPhaseStaging && m.batchDocHasValidFiles() {
		m.submitBatchDocuments()
		return m.startBatchExtraction()
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run TestBatchDocCtrlS ./internal/app/`
Expected: PASS

- [ ] **Step 6: Commit**

`/commit`

---

### Task 10: Batch extraction -- sequential pipeline with progress

**Files:**
- Modify: `internal/app/batch_doc.go` (add extraction orchestration)
- Test: `internal/app/batch_doc_test.go`

This is the most complex task. Extraction runs sequentially, one doc at a time,
using the existing `extract.Pipeline`. Each completed extraction auto-calls
`UpdateDocumentExtraction` (text + OCR data only, no shadow ops).

- [ ] **Step 1: Write failing test for batch extraction flow**

```go
func TestBatchDocExtractionPersistsResults(t *testing.T) {
	m := newTestModelWithStore(t)
	house := requireHouse(t, m)
	proj := data.Project{Title: "Test", HouseProfileID: house.ID}
	require.NoError(t, m.store.CreateProject(&proj))

	tmp := t.TempDir()
	content := []byte("plain text content for extraction")
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "doc.txt"), content, 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: proj.ID})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "doc.txt")))
	m.submitBatchDocuments()

	require.Equal(t, batchPhaseExtracting, m.batchDoc.phase)

	// Run extraction for the single document (no LLM, just text).
	m.runBatchExtraction()

	// Verify extraction results persisted.
	doc, err := m.store.GetDocumentMetadata(m.batchDoc.files[0].DocID)
	require.NoError(t, err)
	assert.NotEmpty(t, doc.ExtractedText)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestBatchDocExtraction ./internal/app/`
Expected: compilation error

- [ ] **Step 3: Implement batch extraction**

```go
// runBatchExtraction runs the extraction pipeline sequentially for each
// successfully submitted document. Results are auto-persisted via
// UpdateDocumentExtraction after each document completes. LLM operations
// are NOT auto-committed.
func (m *Model) runBatchExtraction() {
	bd := m.batchDoc
	for i := range bd.files {
		sf := &bd.files[i]
		if sf.DocID == "" {
			continue // not submitted or failed
		}
		bd.extractIdx = i

		pipeline := &extract.Pipeline{
			LLMClient:  m.ex.llmClient,
			Extractors: extract.DefaultExtractors(0, 0, true),
			Schema:     m.buildSchemaContext(),
			DocID:      sf.DocID,
		}

		result := pipeline.Run(context.Background(), sf.Data, sf.Name, sf.MIME)
		// Release file data from memory after extraction.
		sf.Data = nil
		if result.Err != nil {
			sf.ExtractErr = result.Err
		}

		// Auto-persist text + OCR data (not LLM operations).
		text := result.Text()
		var extractData []byte
		if src := result.SourceByTool("tesseract"); src != nil {
			extractData = src.Data
		}
		model := ""
		if result.LLMUsed {
			model = m.extractionModelName()
		}

		if text != "" || len(extractData) > 0 {
			if err := m.store.UpdateDocumentExtraction(
				sf.DocID, text, extractData, model, nil,
			); err != nil {
				sf.ExtractErr = err
			}
		}
	}
	bd.phase = batchPhaseDone
	m.reloadAfterMutation()
}
```

**IMPORTANT: This must be async.** The synchronous `runBatchExtraction` shown
above is for test convenience only. The real implementation must use `tea.Cmd`
and message passing to avoid blocking the UI render loop:

1. Define a `batchExtractionDoneMsg` type carrying the file index and result.
2. `startBatchExtraction()` returns a `tea.Cmd` that runs extraction for the
   first document in a goroutine and sends `batchExtractionDoneMsg` on
   completion.
3. In `Update`, handle `batchExtractionDoneMsg`: persist results, advance
   `extractIdx`, and return a new `tea.Cmd` for the next document (or
   transition to `batchPhaseDone` if all documents are processed).
4. Cancellation: store a `context.Context` + `CancelFunc` on `batchDocState`.
   `esc` calls `cancel()`, and the running goroutine checks `ctx.Err()`.

Follow the same channel + `tea.Batch` pattern as `startExtractionOverlay` in
`extraction.go`. The synchronous `runBatchExtraction` helper can remain for
tests that don't run the Bubble Tea event loop.

- [ ] **Step 4a: Define batchExtractionDoneMsg and async dispatch**

- [ ] **Step 4b: Wire message handling in Update**

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestBatchDocExtraction ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 11: Extraction progress View rendering

**Files:**
- Modify: `internal/app/batch_doc.go` (extend buildBatchDocOverlay for extraction phase)
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write test for extraction progress rendering**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestBatchDocExtractionProgress ./internal/app/`
Expected: FAIL (view doesn't handle extraction phase yet)

- [ ] **Step 3: Extend buildBatchDocOverlay for extraction/done phases**

Add phase-specific rendering in `buildBatchDocOverlay`:

```go
switch bd.phase {
case batchPhaseStaging:
	// ... existing picker + list view ...
case batchPhaseExtracting, batchPhaseDone:
	// Render per-document status list with icons.
	for i, sf := range bd.files {
		if sf.DocID == "" {
			continue
		}
		var icon string
		switch {
		case sf.ExtractErr != nil:
			icon = "X"
		case i < bd.extractIdx:
			icon = "done"
		case i == bd.extractIdx:
			icon = "..."  // spinner in real impl
		default:
			icon = "wait"
		}
		rows = append(rows, fmt.Sprintf("[%s] %s", icon, sf.Name))
	}
	// Overall progress counter.
	footer = fmt.Sprintf("Overall: %d/%d complete", completed, total)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestBatchDocExtractionProgress ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 12: Done phase -- summary and dismiss

**Files:**
- Modify: `internal/app/batch_doc.go`
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestBatchDocDonePhaseEnterDismisses(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	m.batchDoc.phase = batchPhaseDone
	m.batchDoc.files = []stagedFile{
		{Name: "a.txt", DocID: "doc1"},
	}

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestBatchDocDonePhase ./internal/app/`
Expected: FAIL

- [ ] **Step 3: Add done phase key handling**

In `handleBatchDocKey`:

```go
if bd.phase == batchPhaseDone {
	switch key {
	case keyEnter, keyEsc:
		m.batchDoc = nil
		return nil
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestBatchDocDonePhase ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 13: Entity selector first page (top-level Docs tab)

**Files:**
- Modify: `internal/app/batch_doc.go` (add entity selection phase)
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestBatchDocTopLevelShowsEntitySelector(t *testing.T) {
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabDocuments) // top-level docs tab

	// Start with zero entity (top-level).
	m.startBatchDocOverlay(entityRef{})
	require.NotNil(t, m.batchDoc)
	assert.Equal(t, batchPhaseEntity, m.batchDoc.phase)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestBatchDocTopLevel ./internal/app/`
Expected: FAIL or PASS depending on earlier work -- verify phase is entity

- [ ] **Step 3: Implement entity selection phase**

Build a `huh.Select[entityRef]` with the same entity options as the full
document form. Render it in `buildBatchDocOverlay` when `phase == batchPhaseEntity`.
On `enter`, lock the entity and transition to `batchPhaseStaging`.

```go
func (m *Model) handleBatchDocEntityKey(msg tea.KeyPressMsg) tea.Cmd {
	// Pass keys to the huh.Select field.
	// On completion, set bd.entity and transition to staging.
	bd := m.batchDoc
	// ... huh.Select update logic ...
	if entitySelected {
		bd.entity = selectedEntity
		bd.phase = batchPhaseStaging
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestBatchDocTopLevel ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 14: Cancel during extraction

**Files:**
- Modify: `internal/app/batch_doc.go`
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestBatchDocEscDuringExtractionStopsAndShowsSummary(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	m.batchDoc.phase = batchPhaseExtracting
	m.batchDoc.files = []stagedFile{
		{Name: "a.txt", DocID: "doc1"},
		{Name: "b.txt", DocID: "doc2"},
	}
	m.batchDoc.extractIdx = 0

	sendKey(m, "esc")
	assert.Equal(t, batchPhaseDone, m.batchDoc.phase,
		"esc during extraction should transition to done/summary")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestBatchDocEscDuringExtraction ./internal/app/`
Expected: FAIL

- [ ] **Step 3: Implement cancel during extraction**

In `handleBatchDocEsc`, add extraction-phase handling:

```go
func (m *Model) handleBatchDocEsc() tea.Cmd {
	bd := m.batchDoc
	switch bd.phase {
	case batchPhaseExtracting:
		// Cancel remaining extractions, show summary.
		bd.phase = batchPhaseDone
		return nil
	case batchPhaseStaging:
		if len(bd.files) > 0 {
			m.confirm = confirmBatchDiscard
			return nil
		}
	}
	m.batchDoc = nil
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestBatchDocEscDuringExtraction ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 15: Zone marking and mouse clickability

**Files:**
- Modify: `internal/app/batch_doc.go` (add zone marks to staged list)
- Modify: `internal/app/mouse.go` (add zone constants, handle clicks)
- Test: `internal/app/mouse_test.go`

- [ ] **Step 1: Add zone constants**

In `internal/app/mouse.go`, add:

```go
const (
	zoneBatchFile = "batch-file-" // "batch-file-0", "batch-file-1", etc.
)
```

- [ ] **Step 2: Zone-mark staged file rows in buildBatchDocOverlay**

```go
row := m.zones.Mark(fmt.Sprintf("%s%d", zoneBatchFile, i), renderedRow)
```

- [ ] **Step 3: Write mouse click test**

```go
func TestBatchDocClickOnStagedFileSelectsIt(t *testing.T) {
	m := newTestModelWithStore(t)
	// ... setup with staged files ...
	z := requireZone(t, m, "batch-file-1")
	sendClick(m, z.StartX, z.StartY)
	assert.Equal(t, 1, m.batchDoc.cursor)
	assert.Equal(t, batchFocusList, m.batchDoc.focus)
}
```

- [ ] **Step 4: Handle click dispatch in mouse.go**

Parse `zoneBatchFile` prefix in click handler, set cursor and focus.

- [ ] **Step 5: Run tests**

Run: `go test -run TestBatchDocClick ./internal/app/`
Expected: PASS

- [ ] **Step 6: Commit**

`/commit`

---

### Task 16: Confirmation dialog for cancel with staged files

**Files:**
- Modify: `internal/app/types.go` (add `confirmBatchDiscard` to confirmKind enum)
- Modify: `internal/app/batch_doc.go` (confirmation resolution)
- Modify: `internal/app/confirm.go` or wherever confirmation dialogs are rendered/resolved
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestBatchDocEscWithStagedFilesShowsConfirmation(t *testing.T) {
	m := newTestModelWithStore(t)
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0644))

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NoError(t, m.stageFile(filepath.Join(tmp, "a.txt")))

	sendKey(m, "esc")
	assert.NotNil(t, m.batchDoc, "overlay should NOT close yet")
	assert.Equal(t, confirmBatchDiscard, m.confirm)

	// Confirm discard.
	sendKey(m, "enter") // or "y" depending on confirm pattern
	assert.Nil(t, m.batchDoc)
}
```

- [ ] **Step 2: Add confirmBatchDiscard constant**

In `internal/app/types.go`, add `confirmBatchDiscard` to the `confirmKind` iota.

- [ ] **Step 3: Wire confirmation resolution**

In the confirmation handler (where `m.confirm` is resolved), add a case for
`confirmBatchDiscard` that sets `m.batchDoc = nil` on accept.

- [ ] **Step 4: Run tests**

Run: `go test -run TestBatchDocEsc ./internal/app/`
Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 17: Shift+H hidden file toggle in batch overlay

**Files:**
- Modify: `internal/app/batch_doc.go`
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestBatchDocShiftHTogglesHiddenFiles(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	require.NotNil(t, m.batchDoc)

	// Initially hidden files are not shown.
	assert.False(t, filePickerShowHiddenFromPicker(m.batchDoc.picker))

	sendKey(m, "H") // Shift+H
	assert.True(t, filePickerShowHiddenFromPicker(m.batchDoc.picker))
}
```

- [ ] **Step 2: Implement Shift+H in handleBatchDocPickerKey**

Use the refactored `filePickerShowHiddenFromPicker` / setter to toggle and
re-create the picker with the new setting.

- [ ] **Step 3: Run test**

Run: `go test -run TestBatchDocShiftH ./internal/app/`
Expected: PASS

- [ ] **Step 4: Commit**

`/commit`

---

### Task 18: Cumulative size warning and ctrl+s disabled hint

**Files:**
- Modify: `internal/app/batch_doc.go`
- Test: `internal/app/batch_doc_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestBatchDocCumulativeSizeWarning(t *testing.T) {
	m := newTestModelWithStore(t)
	require.NoError(t, m.store.SetMaxDocumentSize(uint64(100)))

	tmp := t.TempDir()
	// Create files totaling > 10 * maxDocumentSize (1000 bytes).
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("file%d.txt", i)
		require.NoError(t, os.WriteFile(
			filepath.Join(tmp, name), make([]byte, 90), 0644,
		))
	}

	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})
	for i := 0; i < 12; i++ {
		m.stageFile(filepath.Join(tmp, fmt.Sprintf("file%d.txt", i)))
	}

	view := m.buildBatchDocOverlay()
	assert.Contains(t, view, "total size")
}

func TestBatchDocCtrlSDisabledWhenEmpty(t *testing.T) {
	m := newTestModelWithStore(t)
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: "test-id"})

	view := m.buildBatchDocOverlay()
	// ctrl+s hint should be dimmed/absent when no files staged.
	// The exact assertion depends on rendering implementation.
}
```

- [ ] **Step 2: Implement cumulative size warning in buildBatchDocOverlay**

Calculate total staged bytes. If exceeds `store.MaxDocumentSize() * 10`, render
a warning line in the status area.

- [ ] **Step 3: Implement ctrl+s hint dimming**

When `len(bd.files) == 0` or no valid files, render the `ctrl+s` hint with
`appStyles.TextDim()`.

- [ ] **Step 4: Run tests**

Expected: PASS

- [ ] **Step 5: Commit**

`/commit`

---

### Task 19: Full integration test -- end-to-end flow

**Files:**
- Test: `internal/app/batch_doc_test.go`

This test drives the complete flow through the public API (not through
`sendKey` for submission/extraction, since those are async in real usage).

- [ ] **Step 1: Write end-to-end integration test**

```go
func TestBatchDocEndToEnd(t *testing.T) {
	m := newTestModelWithStore(t)
	house := requireHouse(t, m)
	proj := data.Project{Title: "Renovations", HouseProfileID: house.ID}
	require.NoError(t, m.store.CreateProject(&proj))

	// Stage files.
	tmp := t.TempDir()
	for _, name := range []string{"invoice.txt", "receipt.txt"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(tmp, name),
			[]byte("content of "+name),
			0644,
		))
	}

	// Open overlay from scoped view.
	m.startBatchDocOverlay(entityRef{Kind: "project", ID: proj.ID})
	require.Equal(t, batchPhaseStaging, m.batchDoc.phase)

	// Stage files.
	for _, name := range []string{"invoice.txt", "receipt.txt"} {
		require.NoError(t, m.stageFile(filepath.Join(tmp, name)))
	}
	require.Len(t, m.batchDoc.files, 2)

	// Submit.
	m.submitBatchDocuments()
	require.Equal(t, batchPhaseExtracting, m.batchDoc.phase)

	// Verify documents created.
	docs, err := m.store.ListDocumentsByEntity("project", proj.ID, false)
	require.NoError(t, err)
	assert.Len(t, docs, 2)

	// Run extraction.
	m.runBatchExtraction()
	assert.Equal(t, batchPhaseDone, m.batchDoc.phase)

	// Verify extraction results.
	for _, sf := range m.batchDoc.files {
		if sf.DocID != "" {
			doc, err := m.store.GetDocumentMetadata(sf.DocID)
			require.NoError(t, err)
			assert.NotEmpty(t, doc.ExtractedText)
		}
	}

	// Dismiss.
	sendKey(m, "enter")
	assert.Nil(t, m.batchDoc)
}
```

- [ ] **Step 2: Run end-to-end test**

Run: `go test -run TestBatchDocEndToEnd ./internal/app/`
Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `go test -shuffle=on ./...`
Expected: PASS, no regressions

- [ ] **Step 4: Commit**

`/commit`

---

### Task 20: Polish and audit

- [ ] **Step 1: Run linters**

Run: `golangci-lint run ./...`
Fix all warnings.

- [ ] **Step 2: Verify styles use appStyles singleton**

Audit `batch_doc.go` for any inline `lipgloss.NewStyle()` calls. All styles
must go through `appStyles` or the `m.styles` accessor.

- [ ] **Step 3: Verify key constants**

All key string literals in `batch_doc.go` must use constants from `model.go`
(`keyJ`, `keyK`, `keyEsc`, `keyEnter`, `keyCtrlS`, `keyTab`, `keyX`, `keyD`,
`keyShiftA`, `keyShiftH`). No bare string literals.

- [ ] **Step 4: Run /audit-docs**

Check if any documentation needs updating.

- [ ] **Step 5: Record demo**

Run `/record-demo` to capture the new multi-document upload flow.

- [ ] **Step 6: Final commit**

`/commit`
