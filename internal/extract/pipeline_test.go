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
	r := p.Run(context.Background(), []byte{0xFF, 0xD8}, "photo.jpg", "image/jpeg")
	// No text layer extracted, OCR would be attempted but tesseract may not be available.
	// Either way, no error.
	assert.NoError(t, r.Err)
}

func TestPipeline_PDFTextExtraction(t *testing.T) {
	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Skipf("test fixture not found: %s", pdfPath)
	}

	p := &Pipeline{}
	r := p.Run(context.Background(), data, "sample.pdf", "application/pdf")
	require.NoError(t, r.Err)
	assert.Contains(t, r.ExtractedText, "Invoice")
	assert.False(t, r.OCRUsed, "digital PDF should not need OCR")
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

	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Skipf("test fixture not found: %s", pdfPath)
	}

	// The sample PDF has a text layer, so OCR should not be used.
	p := &Pipeline{MaxOCRPages: 5}
	r := p.Run(context.Background(), data, "sample.pdf", "application/pdf")
	require.NoError(t, r.Err)
	assert.False(t, r.OCRUsed, "should use text layer, not OCR")
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
