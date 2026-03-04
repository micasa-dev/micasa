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
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	text, tsv, err := ocrPDF(context.Background(), data, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
	assert.Contains(t, text, "Invoice")
}

func TestOcrPDF_ScannedPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(
		filepath.Join("testdata", "scanned-invoice.pdf"),
	) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/scanned-invoice.pdf")
	}

	text, tsv, err := ocrPDF(context.Background(), data, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrPDF_InvalidData(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	_, _, err := ocrPDF(context.Background(), []byte("not a pdf at all"), 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all image extraction tools failed")
}

func TestOcrPDF_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err = ocrPDF(ctx, data, 5)
	assert.Error(t, err)
}

func TestOcrPDF_MixedPDF_MultiPageTSV(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(
		filepath.Join("testdata", "mixed-inspection.pdf"),
	) //nolint:gosec // test fixture
	if err != nil {
		t.Skipf("test fixture not found (pdfunite unavailable?): testdata/mixed-inspection.pdf")
	}

	text, tsv, err := ocrPDF(context.Background(), data, 5)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrPDF_SinglePage(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	text, _, err := ocrPDF(context.Background(), data, 1)
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

	data, err := os.ReadFile(
		filepath.Join("testdata", "sample-text.png"),
	) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample-text.png")
	}

	text, tsv, err := ocrImage(context.Background(), data)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrImage_InvoicePNG(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(
		filepath.Join("testdata", "invoice.png"),
	) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/invoice.png")
	}

	text, tsv, err := ocrImage(context.Background(), data)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrImage_InvalidData(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	_, _, err := ocrImage(context.Background(), []byte("not an image"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tesseract")
}

func TestOcrImage_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(
		filepath.Join("testdata", "sample-text.png"),
	) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample-text.png")
	}

	ctx, cancel := context.WithCancel(context.Background())
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

	text, tsv, err := ocrImageFile(context.Background(), imgPath)
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	assert.NotEmpty(t, tsv)
}

func TestOcrImageFile_NonExistentFile(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	_, _, err := ocrImageFile(context.Background(), "/nonexistent/image.png")
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := ocrImageFile(ctx, imgPath)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// rasterize -- direct tests
// ---------------------------------------------------------------------------

func TestRasterize_ValidPDF(t *testing.T) {
	t.Parallel()
	if !HasPDFToPPM() {
		skipOrFatalCI(t, "pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	require.NoError(t, os.WriteFile(pdfPath, data, 0o600))

	outputPrefix := filepath.Join(tmpDir, "page")
	err = rasterize(context.Background(), pdfPath, outputPrefix, 5)
	require.NoError(t, err)

	images, err := filepath.Glob(outputPrefix + "*.png")
	require.NoError(t, err)
	assert.NotEmpty(t, images, "rasterize should produce at least one page image")
}

func TestRasterize_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !HasPDFToPPM() {
		skipOrFatalCI(t, "pdftoppm not available")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "corrupt.pdf")
	require.NoError(t, os.WriteFile(pdfPath, []byte("corrupt data"), 0o600))

	outputPrefix := filepath.Join(tmpDir, "page")
	err := rasterize(context.Background(), pdfPath, outputPrefix, 5)
	assert.Error(t, err)
}

func TestRasterize_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !HasPDFToPPM() {
		skipOrFatalCI(t, "pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	require.NoError(t, os.WriteFile(pdfPath, data, 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outputPrefix := filepath.Join(tmpDir, "page")
	err = rasterize(ctx, pdfPath, outputPrefix, 5)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// extractPDF -- edge cases
// ---------------------------------------------------------------------------

func TestExtractPDF_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = extractPDF(ctx, data)
	assert.Error(t, err)
}

func TestExtractPDF_CorruptData(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	_, err := extractPDF(context.Background(), []byte("definitely not a PDF"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdftotext")
}

// ---------------------------------------------------------------------------
// ocrPDFWithProgress -- additional coverage
// ---------------------------------------------------------------------------

func TestOcrPDFWithProgress_ZeroMaxPages(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrPDFWithProgress(context.Background(), data, 0, ch)
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
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrPDFWithProgress(context.Background(), data, -1, ch)
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
		ocrPDFWithProgress(context.Background(), nil, 5, ch)
	})

	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].Done)
	assert.Empty(t, msgs[0].Text)
	assert.NoError(t, msgs[0].Err)
}

func TestOcrPDFWithProgress_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrPDFWithProgress(context.Background(), []byte("not a pdf"), 5, ch)
	})

	var gotErr bool
	for _, msg := range msgs {
		if msg.Err != nil {
			gotErr = true
			assert.True(t, msg.Done)
			assert.Contains(t, msg.Err.Error(), "all image extraction tools failed")
		}
	}
	assert.True(t, gotErr, "should get an error for invalid PDF data")
}

func TestOcrPDFWithProgress_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(context.Background())
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

	data, err := os.ReadFile(
		filepath.Join("testdata", "sample-text.png"),
	) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample-text.png")
	}

	msgs := collectProgress(func(ch chan<- ExtractProgress) {
		ocrImageWithProgress(context.Background(), data, ch)
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
		ocrImageWithProgress(context.Background(), nil, ch)
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
		ocrImageWithProgress(context.Background(), []byte("not an image"), ch)
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

	data, err := os.ReadFile(
		filepath.Join("testdata", "sample-text.png"),
	) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample-text.png")
	}

	ctx, cancel := context.WithCancel(context.Background())
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
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ext := &PDFOCRExtractor{MaxPages: 0}
	src, err := ext.Extract(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, "tesseract", src.Tool)
	assert.NotEmpty(t, src.Text)
}

func TestPDFOCRExtractor_Extract_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	ext := &PDFOCRExtractor{MaxPages: 5}
	_, err := ext.Extract(context.Background(), []byte("not a valid pdf"))
	assert.Error(t, err)
}

func TestPDFOCRExtractor_Extract_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(context.Background())
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
	_, err := ext.Extract(context.Background(), []byte("not an image"))
	assert.Error(t, err)
}

func TestImageOCRExtractor_Extract_ContextCancelled(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	data, err := os.ReadFile(
		filepath.Join("testdata", "invoice.png"),
	) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/invoice.png")
	}

	ctx, cancel := context.WithCancel(context.Background())
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

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ctx, cancel := context.WithCancel(context.Background())
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
	_, err := ext.Extract(context.Background(), []byte("not a pdf"))
	assert.Error(t, err)
}

func TestPDFTextExtractor_Extract_DefaultTimeout(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf")) //nolint:gosec // test fixture
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: testdata/sample.pdf")
	}

	ext := &PDFTextExtractor{Timeout: 0}
	src, err := ext.Extract(context.Background(), data)
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
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	ch := ExtractWithProgress(
		context.Background(),
		[]byte("not a pdf"),
		"application/pdf",
		DefaultExtractors(5, 0),
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
// mergeAcquiredImages -- deduplication logic
// ---------------------------------------------------------------------------

func TestMergeAcquiredImages_PrefersPdftoppm(t *testing.T) {
	t.Parallel()
	results := []acquireResult{
		{tool: "pdfimages", images: []string{"a.png", "b.png"}},
		{tool: "pdftohtml", images: []string{"c.png"}},
		{tool: "pdftoppm", images: []string{"page-01.png", "page-02.png", "page-03.png"}},
	}
	got := mergeAcquiredImages(results)
	assert.Equal(t, []string{"page-01.png", "page-02.png", "page-03.png"}, got,
		"should use pdftoppm when available")
}

func TestMergeAcquiredImages_FallsBackToTargeted(t *testing.T) {
	t.Parallel()
	results := []acquireResult{
		{tool: "pdfimages", images: []string{"a.png", "b.png"}},
		{tool: "pdftohtml", images: []string{"c.png"}},
	}
	got := mergeAcquiredImages(results)
	assert.Equal(t, []string{"a.png", "b.png", "c.png"}, got,
		"should use targeted tools when pdftoppm absent")
}

func TestMergeAcquiredImages_PdftoppmOnly(t *testing.T) {
	t.Parallel()
	results := []acquireResult{
		{tool: "pdftoppm", images: []string{"page-01.png"}},
	}
	got := mergeAcquiredImages(results)
	assert.Equal(t, []string{"page-01.png"}, got)
}

func TestMergeAcquiredImages_Empty(t *testing.T) {
	t.Parallel()
	assert.Nil(t, mergeAcquiredImages(nil))
	assert.Nil(t, mergeAcquiredImages([]acquireResult{}))
}

func TestExtractWithProgress_Image_InvalidData(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	ch := ExtractWithProgress(
		context.Background(),
		[]byte("not an image"),
		"image/png",
		DefaultExtractors(5, 0),
	)

	var gotErr bool
	for msg := range ch {
		if msg.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr, "should report error for invalid image data")
}
