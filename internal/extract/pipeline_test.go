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
	p := &Pipeline{}
	r := p.Run(context.Background(), nil, "empty.pdf", "application/pdf")
	assert.Empty(t, r.ExtractedText)
	assert.Nil(t, r.Hints)
	assert.False(t, r.OCRUsed)
	assert.False(t, r.LLMUsed)
	assert.NoError(t, r.Err)
}

func TestPipeline_PlainText(t *testing.T) {
	p := &Pipeline{}
	r := p.Run(context.Background(), []byte("Hello, world!"), "readme.txt", "text/plain")
	assert.Equal(t, "Hello, world!", r.ExtractedText)
	assert.Nil(t, r.Hints)
	assert.False(t, r.OCRUsed)
	assert.False(t, r.LLMUsed)
	assert.NoError(t, r.Err)
}

func TestPipeline_UnsupportedMIME(t *testing.T) {
	p := &Pipeline{}
	// application/octet-stream: no text extraction, no OCR, no LLM.
	r := p.Run(context.Background(), []byte{0xFF, 0xD8}, "blob.bin", "application/octet-stream")
	assert.Empty(t, r.ExtractedText)
	assert.NoError(t, r.Err)
}

func TestPipeline_ImageOCR(t *testing.T) {
	if !ImageOCRAvailable() {
		t.Skip("tesseract not available")
	}

	imgPath := filepath.Join("testdata", "invoice.png")
	data, err := os.ReadFile(imgPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Skipf("test fixture not found: %s", imgPath)
	}

	p := &Pipeline{}
	r := p.Run(context.Background(), data, "invoice.png", "image/png")
	require.NoError(t, r.Err)
	assert.True(t, r.OCRUsed, "image should trigger OCR")
	assert.NotEmpty(t, r.ExtractedText)
}

func TestPipeline_PDFTextExtraction(t *testing.T) {
	if !HasPDFToText() {
		t.Skip("pdftotext not available")
	}

	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Skipf("test fixture not found: %s", pdfPath)
	}

	p := &Pipeline{}
	r := p.Run(context.Background(), data, "sample.pdf", "application/pdf")
	require.NoError(t, r.Err)
	assert.Contains(t, r.PdfText, "Invoice", "pdftotext should extract text")
	assert.Contains(t, r.ExtractedText, "Invoice")
	assert.False(t, r.LLMUsed, "no LLM client configured")
	assert.Nil(t, r.Hints)
}

func TestPipeline_NoLLMClient(t *testing.T) {
	p := &Pipeline{LLMClient: nil}
	r := p.Run(context.Background(), []byte("some extracted text"), "doc.txt", "text/plain")
	assert.Equal(t, "some extracted text", r.ExtractedText)
	assert.False(t, r.LLMUsed)
	assert.Nil(t, r.Hints)
}

func TestPipeline_OCRIntegration(t *testing.T) {
	if !OCRAvailable() {
		t.Skip("tesseract and/or pdftoppm not available")
	}
	if !HasPDFToText() {
		t.Skip("pdftotext not available")
	}

	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Skipf("test fixture not found: %s", pdfPath)
	}

	// Both pdftotext and OCR should run for PDFs.
	p := &Pipeline{MaxOCRPages: 5}
	r := p.Run(context.Background(), data, "sample.pdf", "application/pdf")
	require.NoError(t, r.Err)
	assert.True(t, r.OCRUsed, "OCR always runs for PDFs")
	assert.NotEmpty(t, r.PdfText, "pdftotext should extract text")
	assert.NotEmpty(t, r.OCRText, "OCR should also extract text")
	assert.Contains(t, r.ExtractedText, "Invoice")
}

func TestPipeline_MixedPDF(t *testing.T) {
	if !OCRAvailable() {
		t.Skip("tesseract and/or pdftoppm not available")
	}
	if !HasPDFToText() {
		t.Skip("pdftotext not available")
	}

	pdfPath := filepath.Join("testdata", "mixed-inspection.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Skipf("test fixture not found: %s", pdfPath)
	}

	p := &Pipeline{MaxOCRPages: 5}
	r := p.Run(context.Background(), data, "mixed-inspection.pdf", "application/pdf")
	require.NoError(t, r.Err)

	// Page 1 is digital text -- pdftotext should extract it.
	assert.NotEmpty(t, r.PdfText, "pdftotext should extract digital text pages")
	assert.Contains(t, r.PdfText, "Invoice")

	// Pages 2-3 are scanned -- OCR should run and find text.
	assert.True(t, r.OCRUsed, "OCR should run for mixed PDFs")
	assert.NotEmpty(t, r.OCRText, "OCR should extract text from scanned pages")

	// ExtractedText should have the pdftotext content (it takes priority).
	assert.Contains(t, r.ExtractedText, "Invoice")
}

func TestPipeline_MaxOCRPagesDefault(t *testing.T) {
	p := &Pipeline{MaxOCRPages: 0}
	// Just verify the default is applied (no panic on zero).
	r := p.Run(context.Background(), []byte("text"), "doc.txt", "text/plain")
	assert.NoError(t, r.Err)
}

func TestPipeline_EntityContext(t *testing.T) {
	p := &Pipeline{
		EntityContext: EntityContext{
			Vendors:    []string{"Garcia Plumbing"},
			Projects:   []string{"Kitchen Remodel"},
			Appliances: []string{"HVAC Unit"},
		},
	}
	// Without LLM client, entity context is loaded but not used.
	r := p.Run(context.Background(), []byte("invoice text"), "inv.txt", "text/plain")
	assert.Equal(t, "invoice text", r.ExtractedText)
	assert.Nil(t, r.Hints)
}
