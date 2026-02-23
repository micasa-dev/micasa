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
	ch := ExtractWithProgress(
		context.Background(),
		nil,
		"application/pdf",
		DefaultExtractors(20, 0),
	)
	msg := <-ch
	assert.True(t, msg.Done)
	assert.Empty(t, msg.Text)
	assert.NoError(t, msg.Err)

	// Channel should be closed.
	_, open := <-ch
	assert.False(t, open)
}

// TestExtractWithProgress_EmptyImage verifies the image path with empty data.
func TestExtractWithProgress_EmptyImage(t *testing.T) {
	ch := ExtractWithProgress(context.Background(), nil, "image/png", DefaultExtractors(20, 0))
	msg := <-ch
	assert.True(t, msg.Done)
	assert.Empty(t, msg.Text)
	assert.NoError(t, msg.Err)
}

// TestExtractWithProgress_ContextCancelled verifies that cancelling the
// context during extraction sends an error and closes the channel. This
// is the path hit when the user quits the app mid-extraction.
func TestExtractWithProgress_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ch := ExtractWithProgress(ctx, []byte("fake image data"), "image/png", DefaultExtractors(20, 0))

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
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	imgPath := filepath.Join("testdata", "sample-text.png")
	data, err := os.ReadFile(imgPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+imgPath)
	}

	ch := ExtractWithProgress(context.Background(), data, "image/png", DefaultExtractors(20, 0))

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
// hits when uploading a scanned PDF: pdfimages extracts embedded images (or
// pdftoppm rasterizes as fallback), then tesseract OCRs them in parallel.
func TestExtractWithProgress_PDF_Integration(t *testing.T) {
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or image extraction tools not available")
	}

	pdfPath := filepath.Join("testdata", "scanned-invoice.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}

	ch := ExtractWithProgress(
		context.Background(),
		data,
		"application/pdf",
		DefaultExtractors(5, 0),
	)

	var phases []string
	var finalText string
	for msg := range ch {
		require.NoError(t, msg.Err)
		if !msg.Done {
			phases = append(phases, msg.Phase)
		} else {
			finalText = msg.Text
		}
	}

	// Should see an image acquisition phase and an extract phase.
	// The acquisition phase is "images" (pdfimages) or "rasterize" (pdftoppm fallback).
	hasAcquire := false
	for _, p := range phases {
		if p == "images" || p == "rasterize" {
			hasAcquire = true
			break
		}
	}
	assert.True(t, hasAcquire, "should see an images or rasterize phase, got: %v", phases)
	assert.Contains(t, phases, "extract")
	assert.NotEmpty(t, finalText, "should extract text from the scanned PDF")
}
