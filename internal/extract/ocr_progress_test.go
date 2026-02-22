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

// TestOCRWithProgress_EmptyData verifies that passing empty data produces
// a single Done message with no text -- the same path hit when a user
// somehow saves a zero-byte document.
func TestOCRWithProgress_EmptyData(t *testing.T) {
	ch := OCRWithProgress(context.Background(), nil, "application/pdf", 20)
	msg := <-ch
	assert.True(t, msg.Done)
	assert.Empty(t, msg.Text)
	assert.NoError(t, msg.Err)

	// Channel should be closed.
	_, open := <-ch
	assert.False(t, open)
}

// TestOCRWithProgress_EmptyImage verifies the image path with empty data.
func TestOCRWithProgress_EmptyImage(t *testing.T) {
	ch := OCRWithProgress(context.Background(), nil, "image/png", 20)
	msg := <-ch
	assert.True(t, msg.Done)
	assert.Empty(t, msg.Text)
	assert.NoError(t, msg.Err)
}

// TestOCRWithProgress_ContextCancelled verifies that cancelling the
// context during OCR sends an error and closes the channel. This is
// the path hit when the user quits the app mid-extraction.
func TestOCRWithProgress_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ch := OCRWithProgress(ctx, []byte("fake image data"), "image/png", 20)

	var gotErr bool
	for msg := range ch {
		if msg.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr, "should receive a context cancellation error")
}

// TestOCRWithProgress_Image_Integration exercises the real path a user
// hits when uploading a PNG: tesseract runs on the image and the channel
// delivers progress updates then the final text.
func TestOCRWithProgress_Image_Integration(t *testing.T) {
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	imgPath := filepath.Join("testdata", "sample-text.png")
	data, err := os.ReadFile(imgPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+imgPath)
	}

	ch := OCRWithProgress(context.Background(), data, "image/png", 20)

	var progressCount int
	var finalText string
	for msg := range ch {
		require.NoError(t, msg.Err)
		if !msg.Done {
			progressCount++
			assert.Equal(t, "ocr", msg.Phase)
			assert.Equal(t, 1, msg.Page)
			assert.Equal(t, 1, msg.Total)
		} else {
			finalText = msg.Text
		}
	}

	assert.Equal(t, 1, progressCount, "should get one progress update for a single image")
	assert.NotEmpty(t, finalText, "tesseract should extract text from the image")
}

// TestOCRWithProgress_PDF_Integration exercises the real path a user
// hits when uploading a scanned PDF: pdftoppm rasterizes pages, then
// tesseract OCRs each page image, with progress on both phases.
func TestOCRWithProgress_PDF_Integration(t *testing.T) {
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	pdfPath := filepath.Join("testdata", "scanned-invoice.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}

	ch := OCRWithProgress(context.Background(), data, "application/pdf", 5)

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

	// Should see at least a rasterize phase and an OCR phase.
	assert.Contains(t, phases, "rasterize")
	assert.Contains(t, phases, "ocr")
	assert.NotEmpty(t, finalText, "OCR should extract text from the scanned PDF")
}
