// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractWithProgress_EmptyData verifies that passing empty data produces
// a single Done message with no text -- the same path hit when a user
// somehow saves a zero-byte document.
func TestExtractWithProgress_EmptyData(t *testing.T) {
	t.Parallel()
	ch := ExtractWithProgress(
		t.Context(),
		nil,
		"application/pdf",
		DefaultExtractors(20, 0, true),
	)
	msg := <-ch
	assert.True(t, msg.Done)
	assert.Empty(t, msg.Text)
	require.NoError(t, msg.Err)

	// Channel should be closed.
	_, open := <-ch
	assert.False(t, open)
}

// TestExtractWithProgress_EmptyImage verifies the image path with empty data.
func TestExtractWithProgress_EmptyImage(t *testing.T) {
	t.Parallel()
	ch := ExtractWithProgress(
		t.Context(),
		nil,
		"image/png",
		DefaultExtractors(20, 0, true),
	)
	msg := <-ch
	assert.True(t, msg.Done)
	assert.Empty(t, msg.Text)
	assert.NoError(t, msg.Err)
}

// TestExtractWithProgress_ContextCancelled verifies that cancelling the
// context during extraction sends an error and closes the channel. This
// is the path hit when the user quits the app mid-extraction.
func TestExtractWithProgress_ContextCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	ch := ExtractWithProgress(
		ctx,
		[]byte("fake image data"),
		"image/png",
		DefaultExtractors(20, 0, true),
	)

	var gotErr bool
	for msg := range ch {
		if msg.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr, "should receive a context cancellation error")
}

// TestExtractWithProgress_Image_Integration exercises the real path a user
// hits when uploading a PNG: tesseract runs on the image and the channel
// delivers progress updates then the final text.
func TestExtractWithProgress_Image_Integration(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	imgPath := filepath.Join("testdata", "sample-text.png")
	data, err := os.ReadFile(imgPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+imgPath)
	}

	ch := ExtractWithProgress(
		t.Context(),
		data,
		"image/png",
		DefaultExtractors(20, 0, true),
	)

	var progressCount int
	var finalText string
	for msg := range ch {
		require.NoError(t, msg.Err)
		if !msg.Done {
			progressCount++
			assert.Equal(t, "extract", msg.Phase)
			assert.Equal(t, 1, msg.Page)
			assert.Equal(t, 1, msg.Total)
		} else {
			finalText = msg.Text
		}
	}

	assert.Equal(t, 1, progressCount, "should get one progress update for a single image")
	assert.NotEmpty(t, finalText, "tesseract should extract text from the image")
}

// TestExtractWithProgress_PDF_Integration exercises the real path a user
// hits when uploading a scanned PDF: all poppler tools run in parallel to
// extract images, then tesseract OCRs them.
func TestExtractWithProgress_PDF_Integration(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or image extraction tools not available")
	}

	pdfPath := filepath.Join("testdata", "scanned-invoice.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}

	ch := ExtractWithProgress(
		t.Context(),
		data,
		"application/pdf",
		DefaultExtractors(5, 0, true),
	)

	var phases []string
	var hasAcquireTools bool
	var finalText string
	for msg := range ch {
		require.NoError(t, msg.Err)
		if msg.Done {
			finalText = msg.Text
			continue
		}
		if len(msg.AcquireTools) > 0 {
			hasAcquireTools = true
		}
		if msg.Phase != "" {
			phases = append(phases, msg.Phase)
		}
	}

	// Should see per-tool acquisition state and OCR page progress.
	assert.True(t, hasAcquireTools, "should see AcquireTools progress messages")
	assert.Contains(t, phases, "extract")
	assert.NotEmpty(t, finalText, "should extract text from the scanned PDF")
}

// TestOcrProgressLoop_NoDeadlockOnPhantomRasterSignals verifies that
// the consumer loop completes when pageDone is signalled for every
// page but rasterDone is never signalled. This is the exact pattern
// ocrPDFPages produces when every per-page goroutine hits ocrPage's
// early-return path before invoking onRasterDone -- e.g. when
// cairoCmd.Start() fails because pdftocairo is missing from PATH or
// the context is cancelled before the subprocess is launched.
//
// Drives ocrProgressLoop directly with synthetic channels so the test
// has zero subprocess dependencies.
func TestOcrProgressLoop_NoDeadlockOnPhantomRasterSignals(t *testing.T) {
	t.Parallel()

	const total = 3
	rasterDone := make(chan struct{}, total)
	pageDone := make(chan struct{}, total)
	// Buffer ch so the loop's send-or-cancel selects never block;
	// the test does not need to drain progress messages.
	ch := make(chan ExtractProgress, 2*total)

	cairoState := &AcquireToolState{Tool: "pdftocairo", Running: true}
	tessState := &AcquireToolState{Tool: "tesseract", Running: true}

	// Simulate ocrPDFPages with a producer that fails to call
	// onRasterDone (cairoCmd.Start() failure path) but still signals
	// pageDone for every page.
	for range total {
		pageDone <- struct{}{}
	}

	result := make(chan bool, 1)
	go func() {
		result <- ocrProgressLoop(
			t.Context(), total, 0,
			cairoState, tessState,
			rasterDone, pageDone, ch,
		)
	}()

	select {
	case cancelled := <-result:
		assert.False(t, cancelled, "loop should complete normally")
		assert.Equal(t, total, tessState.Count,
			"tesseract count should reach total even without rasterDone signals")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "ocrProgressLoop deadlocked waiting for "+
			"rasterDone signals that ocrPDFPages never sent")
	}
}

// TestOcrProgressLoop_CatchesUpCairoOnExit verifies that when the main
// loop exits via pageDone before fully consuming rasterDone (which
// happens non-deterministically because Go's select is uniformly random
// when multiple cases are ready), the last progress message still shows
// cairoState.Count == total. Without the post-loop drain, the last
// message can show cairo < tess -- a visible one-frame stutter in the UI.
func TestOcrProgressLoop_CatchesUpCairoOnExit(t *testing.T) {
	t.Parallel()

	const total = 20 // large enough that select virtually never fully drains rasterDone first
	rasterDone := make(chan struct{}, total)
	pageDone := make(chan struct{}, total)
	ch := make(chan ExtractProgress, 4*total) // plenty of room

	cairoState := &AcquireToolState{Tool: "pdftocairo", Running: true}
	tessState := &AcquireToolState{Tool: "tesseract", Running: true}

	// Pre-fill both channels -- simulates the happy path where every
	// page rasterizes and OCRs successfully.
	for range total {
		rasterDone <- struct{}{}
		pageDone <- struct{}{}
	}

	cancelled := ocrProgressLoop(
		t.Context(), total, 0,
		cairoState, tessState,
		rasterDone, pageDone, ch,
	)
	require.False(t, cancelled, "loop should complete normally")

	// Deterministic assertion: the drain must have consumed every
	// buffered rasterDone signal regardless of select ordering.
	assert.Equal(t, 0, len(rasterDone),
		"drain should have consumed all buffered rasterDone signals")
	assert.Equal(t, total, cairoState.Count,
		"cairoState pointer should reflect total after drain")

	// Drain ch and find the last progress message.
	close(ch)
	var last ExtractProgress
	for msg := range ch {
		last = msg
	}

	// The last progress message must show cairo caught up to total.
	require.NotEmpty(t, last.AcquireTools, "should have AcquireTools in last message")
	cairoSnap := last.AcquireTools[0]
	assert.Equal(t, total, cairoSnap.Count,
		"last progress message should show cairo count == total (no stutter)")
	assert.Equal(t, total, last.AcquireTools[1].Count,
		"last progress message should show tess count == total")
}

// TestDrainBuffered verifies the drain helper deterministically:
// it reads exactly the signals present in the buffer and returns.
func TestDrainBuffered(t *testing.T) {
	t.Parallel()

	ch := make(chan struct{}, 10)
	assert.Equal(t, 0, drainBuffered(ch), "empty channel should drain 0")

	for range 7 {
		ch <- struct{}{}
	}
	assert.Equal(t, 7, drainBuffered(ch), "should drain all 7 buffered signals")
	assert.Equal(t, 0, len(ch), "channel should be empty after drain")

	// Draining again finds nothing.
	assert.Equal(t, 0, drainBuffered(ch), "second drain should find nothing")
}

// TestCatchUpRasterProgress_DrainAndUpdate verifies the catch-up helper
// deterministically: it drains buffered signals, updates cairoState,
// and emits a progress message.
func TestCatchUpRasterProgress_DrainAndUpdate(t *testing.T) {
	t.Parallel()

	rasterDone := make(chan struct{}, 5)
	for range 3 {
		rasterDone <- struct{}{}
	}
	ch := make(chan ExtractProgress, 1)
	cairoState := &AcquireToolState{Tool: "pdftocairo", Running: true, Count: 2}
	tessState := &AcquireToolState{Tool: "tesseract", Running: false, Count: 5}
	snapshot := func() []AcquireToolState {
		return []AcquireToolState{*cairoState, *tessState}
	}

	catchUpRasterProgress(t.Context(), rasterDone, 2, 5, 5, 0,
		cairoState, snapshot, ch)

	assert.Equal(t, 5, cairoState.Count, "should drain 3 signals and add to rasterized=2")
	assert.False(t, cairoState.Running, "should mark not running at total")
	assert.Equal(t, 0, len(rasterDone), "channel should be fully drained")

	require.Equal(t, 1, len(ch), "should have emitted one progress message")
	msg := <-ch
	assert.Equal(t, 5, msg.AcquireTools[0].Count)
}

// TestCatchUpRasterProgress_NothingToDrain verifies the helper is a
// no-op when the channel is empty (error path where ocrPage skipped
// onRasterDone).
func TestCatchUpRasterProgress_NothingToDrain(t *testing.T) {
	t.Parallel()

	rasterDone := make(chan struct{}, 5) // empty
	ch := make(chan ExtractProgress, 1)
	cairoState := &AcquireToolState{Tool: "pdftocairo", Running: true, Count: 0}
	tessState := &AcquireToolState{Tool: "tesseract", Running: false, Count: 3}
	snapshot := func() []AcquireToolState {
		return []AcquireToolState{*cairoState, *tessState}
	}

	catchUpRasterProgress(t.Context(), rasterDone, 0, 3, 3, 0,
		cairoState, snapshot, ch)

	assert.Equal(t, 0, cairoState.Count, "count should be unchanged")
	assert.True(t, cairoState.Running, "running should be unchanged")
	assert.Equal(t, 0, len(ch), "no progress message should be sent")
}

// TestCatchUpRasterProgress_LateCancelDoesNotBlock verifies that a
// cancelled ctx makes the catch-up send fall through without blocking,
// and that cairoState is still updated even though the message was
// dropped.
func TestCatchUpRasterProgress_LateCancelDoesNotBlock(t *testing.T) {
	t.Parallel()

	rasterDone := make(chan struct{}, 3)
	for range 3 {
		rasterDone <- struct{}{}
	}
	ch := make(chan ExtractProgress) // unbuffered, nobody receiving
	cairoState := &AcquireToolState{Tool: "pdftocairo", Running: true}
	tessState := &AcquireToolState{Tool: "tesseract", Running: false, Count: 3}
	snapshot := func() []AcquireToolState {
		return []AcquireToolState{*cairoState, *tessState}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Must return without blocking even though ch has no receiver.
	catchUpRasterProgress(ctx, rasterDone, 0, 3, 3, 0,
		cairoState, snapshot, ch)

	assert.Equal(t, 3, cairoState.Count, "state should be updated even if send was dropped")
	assert.False(t, cairoState.Running)
}

// TestOcrProgressLoop_CancelledContext verifies that cancelling ctx
// before any signals are sent makes the loop return cancelled=true
// without blocking.
func TestOcrProgressLoop_CancelledContext(t *testing.T) {
	t.Parallel()

	const total = 3
	rasterDone := make(chan struct{}, total)
	pageDone := make(chan struct{}, total)
	ch := make(chan ExtractProgress, 2*total)

	cairoState := &AcquireToolState{Tool: "pdftocairo", Running: true}
	tessState := &AcquireToolState{Tool: "tesseract", Running: true}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cancelled := ocrProgressLoop(
		ctx, total, 0,
		cairoState, tessState,
		rasterDone, pageDone, ch,
	)
	assert.True(t, cancelled)
}
