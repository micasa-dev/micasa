// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
