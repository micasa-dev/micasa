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

func TestPipeline_EmptyData(t *testing.T) {
	t.Parallel()
	p := &Pipeline{}
	r := p.Run(context.Background(), nil, "empty.pdf", "application/pdf")
	assert.Empty(t, r.Text())
	assert.Empty(t, r.Operations)
	assert.False(t, r.HasSource("tesseract"))
	assert.False(t, r.LLMUsed)
	assert.NoError(t, r.Err)
}

func TestPipeline_PlainText(t *testing.T) {
	t.Parallel()
	p := &Pipeline{}
	r := p.Run(context.Background(), []byte("Hello, world!"), "readme.txt", "text/plain")
	assert.Equal(t, "Hello, world!", r.Text())
	assert.True(t, r.HasSource("plaintext"))
	assert.Empty(t, r.Operations)
	assert.False(t, r.HasSource("tesseract"))
	assert.False(t, r.LLMUsed)
	assert.NoError(t, r.Err)
}

func TestPipeline_UnsupportedMIME(t *testing.T) {
	t.Parallel()
	p := &Pipeline{}
	// application/octet-stream: no text extraction, no OCR, no LLM.
	r := p.Run(context.Background(), []byte{0xFF, 0xD8}, "blob.bin", "application/octet-stream")
	assert.Empty(t, r.Text())
	assert.NoError(t, r.Err)
}

func TestPipeline_ImageOCR(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	imgPath := filepath.Join("testdata", "invoice.png")
	data, err := os.ReadFile(imgPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+imgPath)
	}

	p := &Pipeline{}
	r := p.Run(context.Background(), data, "invoice.png", "image/png")
	require.NoError(t, r.Err)
	assert.True(t, r.HasSource("tesseract"), "image should trigger OCR")
	assert.NotEmpty(t, r.Text())
}

func TestPipeline_PDFTextExtraction(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}

	p := &Pipeline{}
	r := p.Run(context.Background(), data, "sample.pdf", "application/pdf")
	require.NoError(t, r.Err)
	pdfSrc := r.SourceByTool("pdftotext")
	require.NotNil(t, pdfSrc, "pdftotext should extract text")
	assert.Contains(t, pdfSrc.Text, "Invoice")
	assert.Contains(t, r.Text(), "Invoice")
	assert.False(t, r.LLMUsed, "no LLM client configured")
	assert.Empty(t, r.Operations)
}

func TestPipeline_NoLLMClient(t *testing.T) {
	t.Parallel()
	p := &Pipeline{LLMClient: nil}
	r := p.Run(context.Background(), []byte("some extracted text"), "doc.txt", "text/plain")
	assert.Equal(t, "some extracted text", r.Text())
	assert.False(t, r.LLMUsed)
	assert.Empty(t, r.Operations)
}

func TestPipeline_OCRIntegration(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}

	// Both pdftotext and OCR should run for PDFs.
	p := &Pipeline{Extractors: DefaultExtractors(5, 0, true)}
	r := p.Run(context.Background(), data, "sample.pdf", "application/pdf")
	require.NoError(t, r.Err)
	assert.True(t, r.HasSource("tesseract"), "OCR always runs for PDFs")
	pdfSrc := r.SourceByTool("pdftotext")
	require.NotNil(t, pdfSrc, "pdftotext should extract text")
	assert.NotEmpty(t, pdfSrc.Text)
	ocrSrc := r.SourceByTool("tesseract")
	require.NotNil(t, ocrSrc, "OCR should also extract text")
	assert.NotEmpty(t, ocrSrc.Text)
	assert.Contains(t, r.Text(), "Invoice")
}

func TestPipeline_MixedPDF(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	// mixed-inspection.pdf requires pdfunite which is unavailable on Windows.
	pdfPath := filepath.Join("testdata", "mixed-inspection.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Skipf("test fixture not found (pdfunite unavailable?): %s", pdfPath)
	}

	p := &Pipeline{Extractors: DefaultExtractors(5, 0, true)}
	r := p.Run(context.Background(), data, "mixed-inspection.pdf", "application/pdf")
	require.NoError(t, r.Err)

	// Page 1 is digital text -- pdftotext should extract it.
	pdfSrc := r.SourceByTool("pdftotext")
	require.NotNil(t, pdfSrc, "pdftotext should extract digital text pages")
	assert.Contains(t, pdfSrc.Text, "Invoice")

	// Pages 2-3 are scanned -- OCR should run and find text.
	assert.True(t, r.HasSource("tesseract"), "OCR should run for mixed PDFs")
	ocrSrc := r.SourceByTool("tesseract")
	require.NotNil(t, ocrSrc, "OCR should extract text from scanned pages")
	assert.NotEmpty(t, ocrSrc.Text)

	// Text() should return the pdftotext content (first in priority order).
	assert.Contains(t, r.Text(), "Invoice")
}

func TestPipeline_NilExtractorsDefault(t *testing.T) {
	t.Parallel()
	p := &Pipeline{}
	// Nil extractors falls back to DefaultExtractors(0, 0).
	r := p.Run(context.Background(), []byte("text"), "doc.txt", "text/plain")
	require.NoError(t, r.Err)
	assert.True(t, r.HasSource("plaintext"))
}

func TestPipeline_SchemaContext(t *testing.T) {
	t.Parallel()
	p := &Pipeline{
		Schema: SchemaContext{
			Vendors:    []EntityRow{{ID: "1", Name: "Garcia Plumbing"}},
			Projects:   []EntityRow{{ID: "1", Name: "Kitchen Remodel"}},
			Appliances: []EntityRow{{ID: "1", Name: "HVAC Unit"}},
		},
	}
	// Without LLM client, schema context is loaded but not used.
	r := p.Run(context.Background(), []byte("invoice text"), "inv.txt", "text/plain")
	assert.Equal(t, "invoice text", r.Text())
	assert.Empty(t, r.Operations)
}

func TestResult_Text_EmptySources(t *testing.T) {
	t.Parallel()
	r := &Result{}
	assert.Empty(t, r.Text())
}

func TestResult_Text_FirstNonEmpty(t *testing.T) {
	t.Parallel()
	r := &Result{
		Sources: []TextSource{
			{Tool: "pdftotext", Text: "pdf text"},
			{Tool: "tesseract", Text: "ocr text"},
		},
	}
	assert.Equal(t, "pdf text", r.Text())
}

func TestResult_Text_SkipsWhitespace(t *testing.T) {
	t.Parallel()
	r := &Result{
		Sources: []TextSource{
			{Tool: "pdftotext", Text: "   "},
			{Tool: "tesseract", Text: "ocr text"},
		},
	}
	assert.Equal(t, "ocr text", r.Text())
}

func TestResult_SourceByTool(t *testing.T) {
	t.Parallel()
	r := &Result{
		Sources: []TextSource{
			{Tool: "pdftotext", Text: "pdf"},
			{Tool: "tesseract", Text: "ocr"},
		},
	}
	src := r.SourceByTool("tesseract")
	require.NotNil(t, src)
	assert.Equal(t, "ocr", src.Text)
	assert.Nil(t, r.SourceByTool("nonexistent"))
}

func TestResult_HasSource(t *testing.T) {
	t.Parallel()
	r := &Result{
		Sources: []TextSource{{Tool: "plaintext", Text: "hello"}},
	}
	assert.True(t, r.HasSource("plaintext"))
	assert.False(t, r.HasSource("tesseract"))
}
