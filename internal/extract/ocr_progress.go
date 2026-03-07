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
	Tool    string // extractor tool name (set on Done)
	Desc    string // human description (set on Done)
	Phase   string // e.g. "extract"
	Page    int    // current page (1-indexed)
	Total   int    // total pages (0 until known)
	Skipped int    // pages beyond maxPages cap (0 when no limit)
	Done    bool   // all phases finished
	Text    string // accumulated text (set on Done)
	Data    []byte // structured data (set on Done)
	Err     error  // set on failure

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

	// Get page count.
	pageCount, err := pdfPageCount(ctx, pdfPath)
	if err != nil {
		ch <- ExtractProgress{
			Err:  fmt.Errorf("pdfinfo: %w", err),
			Done: true,
		}
		return
	}

	// Determine how many pages were skipped by the maxPages cap.
	var skipped int
	if maxPages > 0 && pageCount > maxPages {
		skipped = pageCount - maxPages
		pageCount = maxPages
	}
	if pageCount == 0 {
		ch <- ExtractProgress{Done: true}
		return
	}

	// Send initial tool state.
	toolState := &AcquireToolState{Tool: "pdftocairo", Running: true}
	select {
	case ch <- ExtractProgress{AcquireTools: []AcquireToolState{*toolState}}:
	case <-ctx.Done():
		ch <- ExtractProgress{Err: ctx.Err(), Done: true}
		return
	}

	// Run fused pdftocairo+tesseract pipeline with per-page progress.
	total := pageCount
	pageDone := make(chan struct{}, total)
	var ocrResults []ocrPageResult
	done := make(chan struct{})
	go func() {
		ocrResults = ocrPDFPages(ctx, pdfPath, total, pageDone)
		close(done)
	}()

	completed := 0
	for completed < total {
		select {
		case <-pageDone:
			completed++
			toolState.Count = completed
			if completed == total {
				toolState.Running = false
			}
			select {
			case ch <- ExtractProgress{
				Phase:        "extract",
				Page:         completed,
				Total:        total,
				Skipped:      skipped,
				AcquireTools: []AcquireToolState{*toolState},
			}:
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
		Tool:    "tesseract",
		Desc:    "Text recognized from rasterized page images.",
		Done:    true,
		Text:    text,
		Data:    tsv,
		Skipped: skipped,
	}
}
