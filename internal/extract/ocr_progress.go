// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// AcquireToolState tracks a single image extraction tool during acquisition.
type AcquireToolState struct {
	Tool    string
	Running bool // true while the tool is executing
	Count   int  // pages completed (valid when !Running, or incremental while Running)
	Err     error
}

// ExtractProgress reports incremental progress from ExtractWithProgress.
type ExtractProgress struct {
	Tool     string // extractor tool name (set on Done)
	Desc     string // human description (set on Done)
	Phase    string // e.g. "extract"
	Page     int    // current page (1-indexed)
	Total    int    // total pages (0 until known)
	DocPages int    // total pages in the PDF (0 when uncapped)
	Done     bool   // all phases finished
	Text     string // accumulated text (set on Done)
	Data     []byte // structured data (set on Done)
	Err      error  // set on failure

	// AcquireTools carries per-tool state during the rasterization+OCR
	// phase. Non-nil while pages are being processed.
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
	if err := os.WriteFile(imgPath, data, 0o600); err != nil { //nolint:gosec // path is tmpDir + constant filename
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
	tmpDir, err := os.MkdirTemp("", "micasa-ocr-*")
	if err != nil {
		ch <- ExtractProgress{Err: fmt.Errorf("create temp dir: %w", err), Done: true}
		return
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil { //nolint:gosec // path is tmpDir + constant filename
		ch <- ExtractProgress{Err: fmt.Errorf("write temp pdf: %w", err), Done: true}
		return
	}

	// Get page count.
	pageCount, err := pdfPageCount(ctx, pdfPath)
	if err != nil {
		ch <- ExtractProgress{
			Err:  fmt.Errorf("pdfinfo: %w", err),
			Done: true,
		}
		return
	}

	// Track total document pages when a cap is active.
	docPages := pageCount
	if maxPages > 0 && pageCount > maxPages {
		pageCount = maxPages
	}
	// Only surface docPages when the cap actually trimmed something.
	if pageCount == docPages {
		docPages = 0
	}
	if pageCount == 0 {
		ch <- ExtractProgress{Done: true}
		return
	}

	// Send initial pipeline state: both stages running.
	cairoState := &AcquireToolState{Tool: "pdftocairo", Running: true}
	tessState := &AcquireToolState{Tool: "tesseract", Running: true}
	snapshot := func() []AcquireToolState {
		return []AcquireToolState{*cairoState, *tessState}
	}
	select {
	case ch <- ExtractProgress{AcquireTools: snapshot()}:
	case <-ctx.Done():
		ch <- ExtractProgress{Err: ctx.Err(), Done: true}
		return
	}

	// Run fused pdftocairo|tesseract pipeline with per-stage progress.
	total := pageCount
	rasterDone := make(chan struct{}, total)
	pageDone := make(chan struct{}, total)
	var ocrResults []ocrPageResult
	done := make(chan struct{})
	go func() {
		ocrResults = ocrPDFPages(ctx, pdfPath, total, rasterDone, pageDone)
		close(done)
	}()

	cancelled := ocrProgressLoop(
		ctx, total, docPages,
		cairoState, tessState,
		rasterDone, pageDone, ch,
	)
	<-done
	if cancelled {
		ch <- ExtractProgress{Err: ctx.Err(), Done: true}
		return
	}

	cairoState.Running = false
	cairoState.Count = total
	tessState.Running = false
	tessState.Count = total

	text, tsv := collectOCRResults(ocrResults)
	ch <- ExtractProgress{
		Tool:         "tesseract",
		Desc:         "Text recognized from rasterized page images.",
		Done:         true,
		Total:        total,
		DocPages:     docPages,
		Text:         text,
		Data:         tsv,
		AcquireTools: snapshot(),
	}
}

// ocrProgressLoop consumes rasterDone and pageDone events from the
// per-page OCR producer (ocrPDFPages) and forwards per-stage progress
// messages to ch. It returns false when completed reaches total
// normally, true when ctx is cancelled. The caller is responsible for
// waiting on the producer goroutine before draining the result and for
// emitting the final cancellation message when this returns true.
//
// We rely on pageDone alone for completion tracking because ocrPage
// can return without invoking its onRasterDone callback (e.g. when
// cairoCmd.Start() fails before the subprocess is launched), in which
// case rasterDone is never signalled for that page even though
// ocrPDFPages still signals pageDone. Counting rasterDone for
// completion would deadlock the loop in that scenario.
func ocrProgressLoop(
	ctx context.Context,
	total, docPages int,
	cairoState, tessState *AcquireToolState,
	rasterDone, pageDone <-chan struct{},
	ch chan<- ExtractProgress,
) (cancelled bool) {
	snapshot := func() []AcquireToolState {
		return []AcquireToolState{*cairoState, *tessState}
	}
	rasterized := 0
	completed := 0
	for completed < total {
		select {
		case <-rasterDone:
			rasterized++
			cairoState.Count = rasterized
			if rasterized == total {
				cairoState.Running = false
			}
			select {
			case ch <- ExtractProgress{
				Phase:        "extract",
				Page:         completed,
				Total:        total,
				DocPages:     docPages,
				AcquireTools: snapshot(),
			}:
			case <-ctx.Done():
				return true
			}
		case <-pageDone:
			completed++
			tessState.Count = completed
			if completed == total {
				tessState.Running = false
			}
			select {
			case ch <- ExtractProgress{
				Phase:        "extract",
				Page:         completed,
				Total:        total,
				DocPages:     docPages,
				AcquireTools: snapshot(),
			}:
			case <-ctx.Done():
				return true
			}
		case <-ctx.Done():
			return true
		}
	}
	return false
}
