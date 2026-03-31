// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectProgress runs fn in a goroutine, closing ch when fn returns,
// and collects all messages. This mirrors how ExtractWithProgress wraps
// the lower-level progress functions.
func collectProgress(fn func(ch chan<- ExtractProgress)) []ExtractProgress {
	ch := make(chan ExtractProgress, 16)
	go func() {
		defer close(ch)
		fn(ch)
	}()
	var msgs []ExtractProgress
	for msg := range ch {
		msgs = append(msgs, msg)
	}
	return msgs
}

// ---------------------------------------------------------------------------
// ocrPDF -- direct tests
// ---------------------------------------------------------------------------

func TestOcrPDF_ValidPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	text, tsv, err := ocrPDF(t.Context(), data, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
	assert.Contains(t, text, "Invoice")
}

func TestOcrPDF_ScannedPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(
		filepath.Join("testdata", "scanned-invoice.pdf"),
	)
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/scanned-invoice.pdf")
	}

	text, tsv, err := ocrPDF(t.Context(), data, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrPDF_InvalidData(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	_, _, err := ocrPDF(t.Context(), []byte("not a pdf at all"), 5)
	require.Error(t, err)
}

func TestOcrPDF_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err = ocrPDF(ctx, data, 5)
	assert.Error(t, err)
}

func TestOcrPDF_MixedPDF_MultiPageTSV(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(
		filepath.Join("testdata", "mixed-inspection.pdf"),
	)
	if err != nil {
		t.Skipf("test fixture not found (pdfunite unavailable?): testdata/mixed-inspection.pdf")
	}

	text, tsv, err := ocrPDF(t.Context(), data, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrPDF_SinglePage(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	text, _, err := ocrPDF(t.Context(), data, 1)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
}

// ---------------------------------------------------------------------------
// ocrImage -- direct tests
// ---------------------------------------------------------------------------

func TestOcrImage_ValidImage(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample-text.png"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample-text.png")
	}

	text, tsv, err := ocrImage(t.Context(), data)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrImage_InvoicePNG(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "invoice.png"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/invoice.png")
	}

	text, tsv, err := ocrImage(t.Context(), data)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrImage_InvalidData(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	_, _, err := ocrImage(t.Context(), []byte("not an image"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tesseract")
}

func TestOcrImage_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample-text.png"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample-text.png")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err = ocrImage(ctx, data)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ocrImageFile -- direct tests
// ---------------------------------------------------------------------------

func TestOcrImageFile_ValidFile(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	imgPath := filepath.Join("testdata", "sample-text.png")
	if _, err := os.Stat(imgPath); err != nil {
		skipOrFatalCI(t, "test fixture not found: "+imgPath)
	}

	text, tsv, err := ocrImageFile(t.Context(), imgPath)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrImageFile_NonExistentFile(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	_, _, err := ocrImageFile(t.Context(), "/nonexistent/image.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tesseract")
}

func TestOcrImageFile_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	imgPath := filepath.Join("testdata", "sample-text.png")
	if _, err := os.Stat(imgPath); err != nil {
		skipOrFatalCI(t, "test fixture not found: "+imgPath)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err := ocrImageFile(ctx, imgPath)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// pdfPageCount -- direct tests
// ---------------------------------------------------------------------------

func TestPdfPageCount_ValidPDF(t *testing.T) {
	t.Parallel()
	if !HasPDFInfo() {
		skipOrFatalCI(t, "pdfinfo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	//nolint:gosec // path is tmpDir + constant filename
	require.NoError(
		t,
		os.WriteFile(pdfPath, data, 0o600),
	)

	count, err := pdfPageCount(t.Context(), pdfPath)
	require.NoError(t, err)
	assert.Positive(t, count, "page count should be positive")
}

func TestPdfPageCount_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !HasPDFInfo() {
		skipOrFatalCI(t, "pdfinfo not available")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "corrupt.pdf")
	require.NoError(
		t,
		os.WriteFile(pdfPath, []byte("corrupt data"), 0o600),
	)

	_, err := pdfPageCount(t.Context(), pdfPath)
	assert.Error(t, err)
}

func TestPdfPageCount_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !HasPDFInfo() {
		skipOrFatalCI(t, "pdfinfo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	//nolint:gosec // path is tmpDir + constant filename
	require.NoError(
		t,
		os.WriteFile(pdfPath, data, 0o600),
	)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = pdfPageCount(ctx, pdfPath)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ocrPage -- direct tests
// ---------------------------------------------------------------------------

func TestOcrPage_ValidPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	//nolint:gosec // path is tmpDir + constant filename
	require.NoError(
		t,
		os.WriteFile(pdfPath, data, 0o600),
	)

	result := ocrPage(t.Context(), pdfPath, 1, nil)
	require.NoError(t, result.err)
	assert.NotEmpty(t, result.text)
	assert.NotEmpty(t, result.tsv)
}

func TestOcrPage_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "corrupt.pdf")
	require.NoError(
		t,
		os.WriteFile(pdfPath, []byte("corrupt data"), 0o600),
	)

	result := ocrPage(t.Context(), pdfPath, 1, nil)
	assert.Error(t, result.err)
}

func TestOcrPage_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	//nolint:gosec // path is tmpDir + constant filename
	require.NoError(
		t,
		os.WriteFile(pdfPath, data, 0o600),
	)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result := ocrPage(ctx, pdfPath, 1, nil)
	assert.Error(t, result.err)
}

// ---------------------------------------------------------------------------
// extractPDF -- edge cases
// ---------------------------------------------------------------------------

func TestExtractPDF_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = extractPDF(ctx, data)
	assert.Error(t, err)
}

func TestExtractPDF_CorruptData(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	_, err := extractPDF(t.Context(), []byte("definitely not a PDF"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdftotext")
}

// ---------------------------------------------------------------------------
// ocrPDFWithProgress -- additional coverage
// ---------------------------------------------------------------------------

func TestOcrPDFWithProgress_ZeroMaxPages(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrPDFWithProgress(t.Context(), data, 0, ch)
	})

	var finalMsg ExtractProgress
	for _, msg := range msgs {
		if msg.Done {
			finalMsg = msg
		}
	}

	require.NoError(t, finalMsg.Err)
	assert.NotEmpty(t, finalMsg.Text)
}

func TestOcrPDFWithProgress_NegativeMaxPages(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrPDFWithProgress(t.Context(), data, -1, ch)
	})

	var finalMsg ExtractProgress
	for _, msg := range msgs {
		if msg.Done {
			finalMsg = msg
		}
	}

	require.NoError(t, finalMsg.Err)
	assert.NotEmpty(t, finalMsg.Text)
}

func TestOcrPDFWithProgress_EmptyData(t *testing.T) {
	t.Parallel()
	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrPDFWithProgress(t.Context(), nil, 5, ch)
	})

	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].Done)
	assert.Empty(t, msgs[0].Text)
	assert.NoError(t, msgs[0].Err)
}

func TestOcrPDFWithProgress_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrPDFWithProgress(t.Context(), []byte("not a pdf"), 5, ch)
	})

	var gotErr bool
	for _, msg := range msgs {
		if msg.Err != nil {
			gotErr = true
			assert.True(t, msg.Done)
		}
	}
	assert.True(t, gotErr, "should get an error for invalid PDF data")
}

func TestOcrPDFWithProgress_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrPDFWithProgress(ctx, data, 5, ch)
	})

	var gotErr bool
	for _, msg := range msgs {
		if msg.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr, "should get an error when context is cancelled")
}

// ---------------------------------------------------------------------------
// ocrImageWithProgress -- additional coverage
// ---------------------------------------------------------------------------

func TestOcrImageWithProgress_ValidImage(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample-text.png"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample-text.png")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrImageWithProgress(t.Context(), data, ch)
	})

	var progressCount int
	var finalMsg ExtractProgress
	for _, msg := range msgs {
		if msg.Done {
			finalMsg = msg
		} else {
			progressCount++
			assert.Equal(t, "extract", msg.Phase)
			assert.Equal(t, 1, msg.Page)
			assert.Equal(t, 1, msg.Total)
		}
	}

	require.NoError(t, finalMsg.Err)
	assert.Equal(t, 1, progressCount)
	assert.NotEmpty(t, finalMsg.Text)
	assert.NotEmpty(t, finalMsg.Data)
	assert.Equal(t, "tesseract", finalMsg.Tool)
}

func TestOcrImageWithProgress_EmptyData(t *testing.T) {
	t.Parallel()
	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrImageWithProgress(t.Context(), nil, ch)
	})

	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].Done)
	assert.Empty(t, msgs[0].Text)
	assert.NoError(t, msgs[0].Err)
}

func TestOcrImageWithProgress_InvalidImage(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrImageWithProgress(t.Context(), []byte("not an image"), ch)
	})

	var gotErr bool
	for _, msg := range msgs {
		if msg.Err != nil {
			gotErr = true
			assert.True(t, msg.Done)
		}
	}
	assert.True(t, gotErr, "should get a tesseract error for invalid image data")
}

func TestOcrImageWithProgress_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample-text.png"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample-text.png")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrImageWithProgress(ctx, data, ch)
	})

	var gotErr bool
	for _, msg := range msgs {
		if msg.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr, "should get an error when context is cancelled")
}

// ---------------------------------------------------------------------------
// PDFOCRExtractor.Extract -- error paths and defaults
// ---------------------------------------------------------------------------

func TestPDFOCRExtractor_Extract_MaxPagesDefault(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ext := &PDFOCRExtractor{MaxPages: 0}
	src, err := ext.Extract(t.Context(), data)
	require.NoError(t, err)
	assert.Equal(t, "tesseract", src.Tool)
	assert.NotEmpty(t, src.Text)
}

func TestPDFOCRExtractor_Extract_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	ext := &PDFOCRExtractor{MaxPages: 5}
	_, err := ext.Extract(t.Context(), []byte("not a valid pdf"))
	assert.Error(t, err)
}

func TestPDFOCRExtractor_Extract_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ext := &PDFOCRExtractor{MaxPages: 5}
	_, err = ext.Extract(ctx, data)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ImageOCRExtractor.Extract -- error paths
// ---------------------------------------------------------------------------

func TestImageOCRExtractor_Extract_InvalidImage(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	ext := &ImageOCRExtractor{}
	_, err := ext.Extract(t.Context(), []byte("not an image"))
	assert.Error(t, err)
}

func TestImageOCRExtractor_Extract_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "invoice.png"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/invoice.png")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ext := &ImageOCRExtractor{}
	_, err = ext.Extract(ctx, data)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// PDFTextExtractor.Extract -- edge cases
// ---------------------------------------------------------------------------

func TestPDFTextExtractor_Extract_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	ext := &PDFTextExtractor{}
	_, err = ext.Extract(ctx, data)
	assert.Error(t, err)
}

func TestPDFTextExtractor_Extract_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	ext := &PDFTextExtractor{}
	_, err := ext.Extract(t.Context(), []byte("not a pdf"))
	assert.Error(t, err)
}

func TestPDFTextExtractor_Extract_DefaultTimeout(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ext := &PDFTextExtractor{Timeout: 0}
	src, err := ext.Extract(t.Context(), data)
	require.NoError(t, err)
	assert.Equal(t, "pdftotext", src.Tool)
	assert.Contains(t, src.Text, "Invoice")
}

// ---------------------------------------------------------------------------
// ExtractWithProgress -- additional coverage
// ---------------------------------------------------------------------------

func TestExtractWithProgress_PDF_InvalidData(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	ch := ExtractWithProgress(
		t.Context(),
		[]byte("not a pdf"),
		"application/pdf",
		DefaultExtractors(5, 0, true),
	)

	var gotErr bool
	for msg := range ch {
		if msg.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr, "should report error for invalid PDF data")
}

// ---------------------------------------------------------------------------
// ocrPDFPages -- fused pipeline tests
// ---------------------------------------------------------------------------

func TestOcrPDFPages_ValidPDF(t *testing.T) {
	t.Parallel()
	if !HasPDFToCairo() || !HasPDFInfo() || !HasTesseract() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	//nolint:gosec // path is tmpDir + constant filename
	require.NoError(
		t,
		os.WriteFile(pdfPath, data, 0o600),
	)

	pageCount, err := pdfPageCount(t.Context(), pdfPath)
	require.NoError(t, err)
	require.Positive(t, pageCount)

	if pageCount > 2 {
		pageCount = 2
	}

	results := ocrPDFPages(t.Context(), pdfPath, pageCount, nil, nil)
	require.Len(t, results, pageCount)

	for i, r := range results {
		require.NoError(t, r.err, "page %d should succeed", i+1)
		assert.NotEmpty(t, r.text, "page %d should produce text", i+1)
	}
}

func TestOcrPDFPages_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !HasPDFToCairo() || !HasPDFInfo() || !HasTesseract() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	//nolint:gosec // path is tmpDir + constant filename
	require.NoError(
		t,
		os.WriteFile(pdfPath, data, 0o600),
	)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	results := ocrPDFPages(ctx, pdfPath, 1, nil, nil)
	require.Len(t, results, 1)
	assert.Error(t, results[0].err)
}

func TestOcrPDFPages_ProgressReporting(t *testing.T) {
	t.Parallel()
	if !HasPDFToCairo() || !HasPDFInfo() || !HasTesseract() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	//nolint:gosec // path is tmpDir + constant filename
	require.NoError(
		t,
		os.WriteFile(pdfPath, data, 0o600),
	)

	pageDone := make(chan struct{}, 2)
	results := ocrPDFPages(t.Context(), pdfPath, 1, nil, pageDone)
	require.Len(t, results, 1)
	require.NoError(t, results[0].err)

	// Should have received exactly 1 done signal.
	select {
	case <-pageDone:
	default:
		t.Error("expected a page done signal")
	}
}

// ---------------------------------------------------------------------------
// collectOCRResults -- unit tests for result merging
// ---------------------------------------------------------------------------

func TestCollectOCRResults_MixedErrorsAndSuccess(t *testing.T) {
	t.Parallel()

	results := []ocrPageResult{
		{text: "page one", tsv: []byte("h1\th2\ndata1\n")},
		{err: errors.New("page 2 failed")},
		{text: "page three", tsv: []byte("h1\th2\ndata3\n")},
	}

	text, tsv := collectOCRResults(results)
	assert.Contains(t, text, "page one")
	assert.Contains(t, text, "page three")
	assert.NotContains(t, text, "page 2")
	assert.NotEmpty(t, tsv)
}

func TestCollectOCRResults_AllErrors(t *testing.T) {
	t.Parallel()

	results := []ocrPageResult{
		{err: errors.New("fail 1")},
		{err: errors.New("fail 2")},
	}

	text, tsv := collectOCRResults(results)
	assert.Empty(t, text)
	assert.Empty(t, tsv)
}

func TestCollectOCRResults_MultiPageTSVHeaderDedup(t *testing.T) {
	t.Parallel()

	header := "level\tpage_num\tblock_num\n"
	results := []ocrPageResult{
		{text: "one", tsv: []byte(header + "1\t1\t1\n")},
		{text: "two", tsv: []byte(header + "2\t1\t1\n")},
		{text: "three", tsv: []byte(header + "3\t1\t1\n")},
	}

	text, tsv := collectOCRResults(results)
	assert.Contains(t, text, "one")
	assert.Contains(t, text, "three")

	// Header should appear exactly once in merged TSV.
	tsvStr := string(tsv)
	assert.Equal(t, 1, strings.Count(tsvStr, "level\tpage_num\tblock_num"),
		"TSV header should appear exactly once")
	assert.Contains(t, tsvStr, "1\t1\t1")
	assert.Contains(t, tsvStr, "3\t1\t1")
}

func TestCollectOCRResults_Empty(t *testing.T) {
	t.Parallel()

	text, tsv := collectOCRResults(nil)
	assert.Empty(t, text)
	assert.Empty(t, tsv)
}

func TestCollectOCRResults_SinglePageHeaderOnly(t *testing.T) {
	t.Parallel()

	results := []ocrPageResult{
		{text: "words", tsv: []byte("header_only\n")},
	}

	text, tsv := collectOCRResults(results)
	assert.Contains(t, text, "words")
	assert.Contains(t, string(tsv), "header_only")
}

// ---------------------------------------------------------------------------
// ocrPage -- additional error paths
// ---------------------------------------------------------------------------

func TestOcrPage_NonExistentPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	result := ocrPage(t.Context(), "/nonexistent/file.pdf", 1, nil)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "pdftocairo")
}

func TestExtractWithProgress_Image_InvalidData(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	ch := ExtractWithProgress(
		t.Context(),
		[]byte("not an image"),
		"image/png",
		DefaultExtractors(5, 0, true),
	)

	var gotErr bool
	for msg := range ch {
		if msg.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr, "should report error for invalid image data")
}
