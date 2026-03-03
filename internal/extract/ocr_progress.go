// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AcquireToolState tracks a single image extraction tool during acquisition.
type AcquireToolState struct {
	Tool    string
	Running bool // true while the tool is executing
	Count   int  // images produced (valid when !Running)
	Err     error
}

// ExtractProgress reports incremental progress from ExtractWithProgress.
type ExtractProgress struct {
	Tool  string // extractor tool name (set on Done)
	Desc  string // human description (set on Done)
	Phase string // e.g. "pdfimages", "extract"
	Page  int    // current page (1-indexed)
	Total int    // total pages (0 until known)
	Done  bool   // all phases finished
	Text  string // accumulated text (set on Done)
	Data  []byte // structured data (set on Done)
	Err   error  // set on failure

	// AcquireTools carries per-tool state during the image acquisition
	// phase. Non-nil only while poppler tools are running/completing.
	AcquireTools []AcquireToolState
}

// ExtractWithProgress runs async extraction with per-page progress updates
// sent on the returned channel. The channel closes when processing completes.
// The extractors list is consulted to determine whether to run image or PDF
// OCR. Unsupported types produce a single Done message with empty text.
func ExtractWithProgress(
	ctx context.Context,
	data []byte,
	mime string,
	extractors []Extractor,
) <-chan ExtractProgress {
	ch := make(chan ExtractProgress, 8)
	go func() {
		defer close(ch)
		if HasMatchingExtractor(extractors, "tesseract", "image/png") && IsImageMIME(mime) {
			ocrImageWithProgress(ctx, data, ch)
		} else {
			ocrPDFWithProgress(ctx, data, ExtractorMaxPages(extractors), ch)
		}
	}()
	return ch
}

// ocrImageWithProgress runs tesseract directly on an image file.
func ocrImageWithProgress(ctx context.Context, data []byte, ch chan<- ExtractProgress) {
	if len(data) == 0 {
		ch <- ExtractProgress{Done: true}
		return
	}

	tmpDir, err := os.MkdirTemp("", "micasa-ocr-*")
	if err != nil {
		ch <- ExtractProgress{Err: fmt.Errorf("create temp dir: %w", err), Done: true}
		return
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	imgPath := filepath.Join(tmpDir, "input.png")
	if err := os.WriteFile(imgPath, data, 0o600); err != nil {
		ch <- ExtractProgress{Err: fmt.Errorf("write temp image: %w", err), Done: true}
		return
	}

	select {
	case ch <- ExtractProgress{Phase: "extract", Page: 1, Total: 1}:
	case <-ctx.Done():
		ch <- ExtractProgress{Err: ctx.Err(), Done: true}
		return
	}

	text, tsv, err := ocrImageFile(ctx, imgPath)
	if err != nil {
		ch <- ExtractProgress{Err: fmt.Errorf("tesseract: %w", err), Done: true}
		return
	}

	ch <- ExtractProgress{
		Tool: "tesseract",
		Desc: "Text recognized from the image.",
		Done: true,
		Text: normalizeWhitespace(text),
		Data: tsv,
	}
}

func ocrPDFWithProgress(
	ctx context.Context,
	data []byte,
	maxPages int,
	ch chan<- ExtractProgress,
) {
	if len(data) == 0 {
		ch <- ExtractProgress{Done: true}
		return
	}
	if maxPages <= 0 {
		maxPages = DefaultMaxExtractPages
	}

	tmpDir, err := os.MkdirTemp("", "micasa-ocr-*")
	if err != nil {
		ch <- ExtractProgress{Err: fmt.Errorf("create temp dir: %w", err), Done: true}
		return
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		ch <- ExtractProgress{Err: fmt.Errorf("write temp pdf: %w", err), Done: true}
		return
	}

	// Determine which tools are available and build initial "all running" state.
	toolStates := make(map[string]*AcquireToolState, len(toolOrder))
	for _, tool := range toolOrder {
		switch tool {
		case "pdfimages":
			if HasPDFImages() {
				toolStates[tool] = &AcquireToolState{Tool: tool, Running: true}
			}
		case "pdftohtml":
			if HasPDFToHTML() {
				toolStates[tool] = &AcquireToolState{Tool: tool, Running: true}
			}
		case "pdftoppm":
			if HasPDFToPPM() {
				toolStates[tool] = &AcquireToolState{Tool: tool, Running: true}
			}
		}
	}

	// Send initial state showing all tools as running.
	if len(toolStates) > 0 {
		select {
		case ch <- ExtractProgress{AcquireTools: snapshotToolStates(toolStates)}:
		case <-ctx.Done():
			ch <- ExtractProgress{Err: ctx.Err(), Done: true}
			return
		}
	}

	// Per-tool completion channel: acquireImages notifies here as each tool
	// finishes, so we can update the UI while acquisition is still in flight.
	type toolDoneMsg struct {
		tool  string
		count int
		err   error
	}
	toolDoneCh := make(chan toolDoneMsg, len(toolStates))

	type acquireOut struct {
		results []acquireResult
		err     error
	}
	resultCh := make(chan acquireOut, 1)

	go func() {
		results, acqErr := acquireImages(ctx, pdfPath, tmpDir, maxPages,
			func(tool string, count int, toolErr error) {
				toolDoneCh <- toolDoneMsg{tool, count, toolErr}
			},
		)
		close(toolDoneCh)
		resultCh <- acquireOut{results, acqErr}
	}()

	// Forward per-tool completion to the ExtractProgress channel.
	for msg := range toolDoneCh {
		if ts, ok := toolStates[msg.tool]; ok {
			ts.Running = false
			ts.Count = msg.count
			ts.Err = msg.err
		}
		select {
		case ch <- ExtractProgress{AcquireTools: snapshotToolStates(toolStates)}:
		case <-ctx.Done():
			ch <- ExtractProgress{Err: ctx.Err(), Done: true}
			return
		}
	}

	result := <-resultCh
	if result.err != nil {
		ch <- ExtractProgress{Err: result.err, Done: true}
		return
	}

	// Merge images, preferring targeted tools over pdftoppm fallback.
	images := mergeAcquiredImages(result.results)

	if len(images) == 0 {
		ch <- ExtractProgress{Done: true}
		return
	}

	total := len(images)

	// OCR pages in parallel with per-page progress.
	pageDone := make(chan struct{}, total)
	var ocrResults []ocrPageResult
	done := make(chan struct{})
	go func() {
		ocrResults = ocrPagesParallel(ctx, images, pageDone)
		close(done)
	}()

	completed := 0
	for completed < total {
		select {
		case <-pageDone:
			completed++
			select {
			case ch <- ExtractProgress{Phase: "extract", Page: completed, Total: total}:
			case <-ctx.Done():
				<-done
				ch <- ExtractProgress{Err: ctx.Err(), Done: true}
				return
			}
		case <-ctx.Done():
			<-done
			ch <- ExtractProgress{Err: ctx.Err(), Done: true}
			return
		}
	}
	<-done

	text, tsv := collectOCRResults(ocrResults)
	ch <- ExtractProgress{
		Tool: "tesseract",
		Desc: "Text recognized from rasterized page images.",
		Done: true,
		Text: text,
		Data: tsv,
	}
}

// snapshotToolStates returns a slice of AcquireToolState in tool priority
// order, suitable for embedding in an ExtractProgress message.
func snapshotToolStates(states map[string]*AcquireToolState) []AcquireToolState {
	var out []AcquireToolState
	for _, tool := range toolOrder {
		if s, ok := states[tool]; ok {
			out = append(out, *s)
		}
	}
	return out
}

// rasterize calls pdftoppm to convert PDF pages to PNG images.
func rasterize(ctx context.Context, pdfPath, outputPrefix string, maxPages int) error {
	args := []string{
		"-png",
		"-r", "300",
		"-l", fmt.Sprintf("%d", maxPages),
		pdfPath,
		outputPrefix,
	}
	cmd := exec.CommandContext( //nolint:gosec // args are constructed internally
		ctx,
		"pdftoppm",
		args...,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}
