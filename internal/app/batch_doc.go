// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
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
	batchPhaseEntity     batchDocPhase = iota // top-level: pick entity first
	batchPhaseStaging                         // pick files, build staged list
	batchPhaseUploading                       // saving documents to DB
	batchPhaseExtracting                      // running extraction pipeline
	batchPhaseDone                            // summary view
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

	// Entity selection phase (batchPhaseEntity).
	entitySelect *huh.Select[entityRef]

	// Staging phase.
	picker *huh.FilePicker
	files  []stagedFile
	cursor int
	dirty  bool // true when files have been staged; guards cancel confirmation (Task 16)

	// Extraction phase.
	extractIdx int
	ctx        context.Context    // non-nil while extraction is running
	cancelFn   context.CancelFunc // non-nil while extraction is running
}

// batchExtractionDoneMsg is sent when an async extraction completes.
type batchExtractionDoneMsg struct {
	fileIdx int
	result  *extract.Result
	err     error // non-nil if the pipeline itself errored (not result.Err)
}

// batchDocOverlay implements the overlay interface for the batch doc staging overlay.
type batchDocOverlay struct{ m *Model }

func (o batchDocOverlay) isVisible() bool                       { return o.m.batchDoc != nil }
func (o batchDocOverlay) handleKey(key tea.KeyPressMsg) tea.Cmd { return o.m.handleBatchDocKey(key) }
func (o batchDocOverlay) hidesMainKeys() bool                   { return true }

// closeBatchDocOverlay cancels any running extraction and nils the overlay.
func (m *Model) closeBatchDocOverlay() {
	if bd := m.batchDoc; bd != nil && bd.cancelFn != nil {
		bd.cancelFn()
	}
	m.batchDoc = nil
}

// startBatchDocOverlay opens the multi-document staging overlay. When entity
// has a non-empty Kind the overlay starts in staging phase with the entity
// pre-locked; otherwise it starts in entity-selection phase (batchPhaseEntity)
// so the user picks a target entity before staging files.
func (m *Model) startBatchDocOverlay(entity entityRef) tea.Cmd {
	fp := m.newDocumentFilePicker("Select files")

	if entity.Kind == "" {
		// Build entity select for the first page.
		entityOpts, err := m.documentEntityOptions()
		if err != nil {
			m.setStatusError("load entities: " + err.Error())
			return nil
		}
		// Remove the "(none)" option -- a target entity is required here.
		filtered := make([]huh.Option[entityRef], 0, len(entityOpts))
		for _, o := range entityOpts {
			if o.Value.Kind != "" {
				filtered = append(filtered, o)
			}
		}
		if len(filtered) == 0 {
			m.setStatusError("no entities available for document upload")
			return nil
		}
		sel := huh.NewSelect[entityRef]().
			Title("Select entity").
			Height(12).
			Options(filtered...)
		initCmd := sel.Init()
		m.batchDoc = &batchDocState{
			phase:        batchPhaseEntity,
			focus:        batchFocusPicker,
			entity:       entityRef{},
			entitySelect: sel,
			picker:       fp,
		}
		return initCmd
	}

	initCmd := fp.Init()
	m.batchDoc = &batchDocState{
		phase:  batchPhaseStaging,
		focus:  batchFocusPicker,
		entity: entity,
		picker: fp,
	}
	return initCmd
}

// handleBatchDocKey routes key events to the appropriate handler based on the
// current phase and focus within the staging overlay.
func (m *Model) handleBatchDocKey(msg tea.KeyPressMsg) tea.Cmd {
	bd := m.batchDoc
	key := msg.String()

	// Confirmation dialog (batch discard): only y/n/esc are active.
	if m.confirm == confirmBatchDiscard {
		switch key {
		case keyY:
			m.confirm = confirmNone
			m.closeBatchDocOverlay()
		case keyN, keyEsc:
			m.confirm = confirmNone
		}
		return nil
	}

	// Done phase: enter or esc dismisses.
	if bd.phase == batchPhaseDone {
		switch key {
		case keyEnter, keyEsc:
			m.closeBatchDocOverlay()
		}
		return nil
	}

	// Entity selection phase: forward keys to the select field.
	if bd.phase == batchPhaseEntity {
		return m.handleBatchDocEntityKey(msg)
	}

	switch key {
	case keyCtrlS:
		if bd.phase == batchPhaseStaging && m.batchDocHasValidFiles() {
			m.submitBatchDocuments()
			return m.startBatchExtraction()
		}
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

// handleBatchDocEntityKey forwards key events to the entity select field.
// When the select completes (user presses enter on a selection), the selected
// entity is locked and the overlay transitions to staging phase.
func (m *Model) handleBatchDocEntityKey(msg tea.KeyPressMsg) tea.Cmd {
	bd := m.batchDoc
	key := msg.String()

	if key == keyEsc {
		m.closeBatchDocOverlay()
		return nil
	}

	_, cmd := bd.entitySelect.Update(msg)

	// Check whether the select has a committed value.
	if ref, ok := bd.entitySelect.GetValue().(entityRef); ok && ref.Kind != "" {
		bd.entity = ref
		bd.phase = batchPhaseStaging
		return tea.Batch(cmd, bd.picker.Init())
	}

	return cmd
}

func (m *Model) handleBatchDocPickerKey(msg tea.KeyPressMsg) tea.Cmd {
	bd := m.batchDoc

	// Toggle hidden files with Shift+H.
	if msg.String() == keyShiftH {
		current := filePickerShowHidden(bd.picker)
		bd.picker.ShowHidden(!current)
		syncPickerDescription(bd.picker)
		return nil
	}

	// Pass key to embedded file picker.
	_, cmd := bd.picker.Update(msg)

	// Check whether the picker just selected a file.
	if path, ok := bd.picker.GetValue().(string); ok && path != "" {
		// Stage the file; errors are captured inside sf.Err.
		_ = m.stageFile(path) //nolint:errcheck // stageFile only returns errors for system failures

		// Re-create the picker rooted at the same directory so the user
		// can immediately pick another file.
		dir := filePickerCurrentDir(bd.picker)
		fp := m.newDocumentFilePicker("Select files")
		if dir != "" {
			fp.CurrentDirectory(dir)
		}
		initCmd := fp.Init()
		bd.picker = fp
		return tea.Batch(cmd, initCmd)
	}

	return cmd
}

// stageFile reads path eagerly (metadata, MIME, SHA256) and appends it to the
// staged list. Errors related to the file itself (unreadable, too large,
// duplicate checksum) are captured in stagedFile.Err rather than returned; the
// file is still appended so the user sees feedback. Only system-level errors
// (e.g. filepath.Abs failure) are returned.
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

	info, err := os.Stat(absPath)
	if err != nil {
		sf.Err = fmt.Errorf("cannot read: %w", err)
		bd.files = append(bd.files, sf)
		bd.dirty = true
		return nil
	}
	sf.Size = info.Size()

	// Check size limit.
	if sf.Size > 0 &&
		uint64(
			sf.Size,
		) > m.store.MaxDocumentSize() { //nolint:gosec // Size is non-negative (checked above)
		sf.Err = fmt.Errorf(
			"file too large (%s)",
			humanize.IBytes(uint64(sf.Size)),
		) //nolint:gosec // non-negative
		bd.files = append(bd.files, sf)
		bd.dirty = true
		return nil
	}

	data, err := os.ReadFile(absPath) //nolint:gosec // user's file picker selection
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
	if bd.entity.Kind != "" {
		exists, err := m.store.DocumentExistsByChecksum(
			bd.entity.Kind, bd.entity.ID, sf.Checksum,
		)
		if err != nil {
			sf.Err = fmt.Errorf("checksum check: %w", err)
		} else if exists {
			sf.Err = fmt.Errorf("duplicate: same file already attached")
		}
	}

	bd.files = append(bd.files, sf)
	bd.dirty = true
	return nil
}

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
		if bd.cursor < len(bd.files) {
			sf := bd.files[bd.cursor]
			detail := fmt.Sprintf("%s  %s  %s", sf.Name,
				humanize.IBytes(uint64(sf.Size)), sf.MIME) //nolint:gosec // Size is non-negative
			if sf.Err != nil {
				detail += "  " + sf.Err.Error()
			}
			m.setStatusInfo(detail)
		}
	}
	return nil
}

// submitBatchDocuments creates a Document record for each staged file without
// errors. Sets DocID on success or SubmitErr on failure. Does NOT release
// sf.Data -- extraction still needs it.
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
	}
	bd.phase = batchPhaseExtracting
}

// batchDocHasValidFiles reports whether any staged file has no error.
func (m *Model) batchDocHasValidFiles() bool {
	for _, sf := range m.batchDoc.files {
		if sf.Err == nil {
			return true
		}
	}
	return false
}

// startBatchExtraction kicks off extraction for the first valid document.
func (m *Model) startBatchExtraction() tea.Cmd {
	bd := m.batchDoc
	//nolint:gosec // cancel stored in bd.cancelFn, called on esc or overlay close
	ctx, cancel := context.WithCancel(context.Background())
	bd.ctx = ctx
	bd.cancelFn = cancel

	for i := range bd.files {
		if bd.files[i].DocID != "" {
			bd.extractIdx = i
			return m.extractBatchFileCmd(i)
		}
	}
	// No valid documents to extract.
	bd.phase = batchPhaseDone
	return nil
}

// extractBatchFileCmd returns a tea.Cmd that runs extraction for the file at index i.
func (m *Model) extractBatchFileCmd(i int) tea.Cmd {
	bd := m.batchDoc
	sf := bd.files[i]
	ctx := bd.ctx

	pipeline := &extract.Pipeline{
		LLMClient:  m.extractionLLMClient(),
		Extractors: extract.DefaultExtractors(0, 0, true),
		Schema:     m.buildSchemaContext(),
		DocID:      sf.DocID,
	}
	fileData := sf.Data
	name := sf.Name
	mime := sf.MIME

	return func() tea.Msg {
		result := pipeline.Run(ctx, fileData, name, mime)
		return batchExtractionDoneMsg{fileIdx: i, result: result}
	}
}

// handleBatchExtractionDone processes a completed extraction and starts the next.
func (m *Model) handleBatchExtractionDone(msg batchExtractionDoneMsg) tea.Cmd {
	bd := m.batchDoc
	if bd == nil || bd.phase != batchPhaseExtracting {
		return nil
	}

	sf := &bd.files[msg.fileIdx]

	if msg.result != nil {
		if msg.result.Err != nil {
			sf.ExtractErr = msg.result.Err
		}

		text := msg.result.Text()
		var extractData []byte
		if src := msg.result.SourceByTool("tesseract"); src != nil {
			extractData = src.Data
		}
		model := ""
		if msg.result.LLMUsed {
			model = m.extractionModelLabel()
		}

		if text != "" || len(extractData) > 0 {
			if err := m.store.UpdateDocumentExtraction(
				sf.DocID, text, extractData, model, nil,
			); err != nil {
				sf.ExtractErr = err
			}
		}
	}
	if msg.err != nil {
		sf.ExtractErr = msg.err
	}

	// Release file data.
	sf.Data = nil

	// Find next document to extract.
	for i := msg.fileIdx + 1; i < len(bd.files); i++ {
		if bd.files[i].DocID != "" {
			bd.extractIdx = i
			return m.extractBatchFileCmd(i)
		}
	}

	// All done. Advance extractIdx past the last file so the icon logic
	// (i < bd.extractIdx) marks all processed files as complete.
	bd.extractIdx = len(bd.files)
	bd.phase = batchPhaseDone
	if bd.cancelFn != nil {
		bd.cancelFn()
	}
	m.reloadAfterMutation()
	return nil
}

// runBatchExtraction is a synchronous helper for tests that don't run the event loop.
func (m *Model) runBatchExtraction() {
	bd := m.batchDoc
	for i := range bd.files {
		sf := &bd.files[i]
		if sf.DocID == "" {
			continue
		}
		bd.extractIdx = i

		pipeline := &extract.Pipeline{
			Extractors: extract.DefaultExtractors(0, 0, true),
			Schema:     m.buildSchemaContext(),
			DocID:      sf.DocID,
		}

		result := pipeline.Run(context.Background(), sf.Data, sf.Name, sf.MIME)
		sf.Data = nil

		if result.Err != nil {
			sf.ExtractErr = result.Err
		}

		text := result.Text()
		var extractData []byte
		if src := result.SourceByTool("tesseract"); src != nil {
			extractData = src.Data
		}

		if text != "" || len(extractData) > 0 {
			if err := m.store.UpdateDocumentExtraction(
				sf.DocID,
				text,
				extractData,
				"",
				nil,
			); err != nil {
				sf.ExtractErr = err
			}
		}
	}
	bd.extractIdx = len(bd.files) // match handleBatchExtractionDone
	bd.phase = batchPhaseDone
	m.reloadAfterMutation()
}

func (m *Model) handleBatchDocEsc() tea.Cmd {
	bd := m.batchDoc
	switch bd.phase {
	case batchPhaseExtracting:
		if bd.cancelFn != nil {
			bd.cancelFn()
		}
		bd.phase = batchPhaseDone
		return nil
	case batchPhaseStaging:
		if len(bd.files) > 0 {
			m.confirm = confirmBatchDiscard
			return nil
		}
	case batchPhaseEntity, batchPhaseUploading, batchPhaseDone:
		// Entity/uploading/done: close immediately.
	}
	m.closeBatchDocOverlay()
	return nil
}

// buildBatchDocOverlay renders the staging overlay content.
func (m *Model) buildBatchDocOverlay() string {
	bd := m.batchDoc
	if bd == nil {
		return ""
	}

	var sections []string

	switch bd.phase {
	case batchPhaseEntity:
		sections = append(sections,
			appStyles.HeaderSection().Render(" Add Documents "),
			bd.entitySelect.View(),
		)
		hints := m.helpItem(keyEnter, "select") + "  " + m.helpItem(keyEsc, "cancel")
		sections = append(sections, appStyles.TextDim().Render(hints))

	case batchPhaseStaging:
		// Title.
		title := "Add Documents"
		if bd.entity.Kind != "" {
			title = fmt.Sprintf("Add Documents to %s", bd.entity.Kind)
		}
		sections = append(sections, appStyles.HeaderSection().Render(" "+title+" "))

		// File picker.
		pickerView := bd.picker.View()
		if bd.focus != batchFocusPicker {
			pickerView = appStyles.TextDim().Render(pickerView)
		}
		sections = append(sections, pickerView)

		// Staged files list.
		if len(bd.files) > 0 {
			header := fmt.Sprintf("Staged (%d)", len(bd.files))
			var rows []string
			rows = append(rows, appStyles.TextDim().Render(header))
			var totalBytes uint64
			for i, sf := range bd.files {
				cursor := "  "
				if bd.focus == batchFocusList && i == bd.cursor {
					cursor = "> "
				}
				line := cursor + sf.Name
				//nolint:gosec // Size is non-negative
				line += "  " + humanize.IBytes(uint64(sf.Size))
				if sf.Err != nil {
					line += "  " + sf.Err.Error()
				}
				//nolint:gosec // Size is non-negative
				totalBytes += uint64(sf.Size)
				row := m.zones.Mark(fmt.Sprintf("%s%d", zoneBatchFile, i), line)
				rows = append(rows, row)
			}
			sections = append(sections, strings.Join(rows, "\n"))

			// Warn when cumulative size exceeds 10x the per-file limit.
			if totalBytes > m.store.MaxDocumentSize()*10 {
				warn := fmt.Sprintf(
					"total %s exceeds recommended batch size",
					humanize.IBytes(totalBytes),
				)
				sections = append(sections, appStyles.TextDim().Render(warn))
			}
		}

		// Hint line.
		hints := m.helpItem(keyEnter, "add") + "  " +
			m.helpItem(keyCtrlS, "upload") + "  " +
			m.helpItem(keyX, "remove") + "  " +
			m.helpItem(keyEsc, "cancel")
		sections = append(sections, appStyles.TextDim().Render(hints))

	case batchPhaseUploading:
		// Uploading is synchronous; this phase is transient.

	case batchPhaseExtracting, batchPhaseDone:
		title := "Extracting Documents"
		if bd.phase == batchPhaseDone {
			title = "Upload Complete"
		}
		sections = append(sections, appStyles.HeaderSection().Render(" "+title+" "))

		var completed, total int
		var rows []string
		for i, sf := range bd.files {
			if sf.DocID == "" {
				continue
			}
			total++
			var icon string
			switch {
			case sf.ExtractErr != nil:
				icon = symX
				completed++
			case i < bd.extractIdx:
				icon = symCheck
				completed++
			case i == bd.extractIdx && bd.phase == batchPhaseExtracting:
				icon = symEllipsis
			default:
				icon = symMiddleDot
			}
			rows = append(rows, fmt.Sprintf(" %s %s", icon, sf.Name))
		}
		sections = append(sections,
			strings.Join(rows, "\n"),
			appStyles.TextDim().Render(
				fmt.Sprintf("\n%d/%d complete", completed, total),
			),
		)

		if bd.phase == batchPhaseExtracting {
			sections = append(sections, appStyles.TextDim().Render(
				m.helpItem(keyEsc, "cancel"),
			))
		} else {
			sections = append(sections, appStyles.TextDim().Render(
				m.helpItem(keyEnter, "close"),
			))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return m.styles.OverlayBox().
		Width(m.overlayContentWidth()).
		MaxHeight(m.overlayMaxHeight()).
		Render(content)
}
