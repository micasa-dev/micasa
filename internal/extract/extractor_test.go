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

// --- Tool/Matches/Available unit tests ---

func TestPDFTextExtractor_Tool(t *testing.T) {
	ext := &PDFTextExtractor{}
	assert.Equal(t, "pdftotext", ext.Tool())
}

func TestPDFTextExtractor_Matches(t *testing.T) {
	ext := &PDFTextExtractor{}
	assert.True(t, ext.Matches("application/pdf"))
	assert.False(t, ext.Matches("text/plain"))
	assert.False(t, ext.Matches("image/png"))
}

func TestPDFTextExtractor_Available(t *testing.T) {
	ext := &PDFTextExtractor{}
	assert.Equal(t, HasPDFToText(), ext.Available())
}

func TestPlainTextExtractor_Tool(t *testing.T) {
	ext := &PlainTextExtractor{}
	assert.Equal(t, "plaintext", ext.Tool())
}

func TestPlainTextExtractor_Matches(t *testing.T) {
	ext := &PlainTextExtractor{}
	assert.True(t, ext.Matches("text/plain"))
	assert.True(t, ext.Matches("text/markdown"))
	assert.True(t, ext.Matches("text/csv"))
	assert.False(t, ext.Matches("application/pdf"))
	assert.False(t, ext.Matches("image/png"))
}

func TestPlainTextExtractor_Available(t *testing.T) {
	ext := &PlainTextExtractor{}
	assert.True(t, ext.Available())
}

func TestPlainTextExtractor_Extract(t *testing.T) {
	ext := &PlainTextExtractor{}
	src, err := ext.Extract(context.Background(), []byte("  hello   world  "))
	require.NoError(t, err)
	assert.Equal(t, "plaintext", src.Tool)
	assert.Equal(t, "hello world", src.Text)
	assert.NotEmpty(t, src.Desc)
}

func TestPDFTextExtractor_Extract_EmptyData(t *testing.T) {
	ext := &PDFTextExtractor{}
	src, err := ext.Extract(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, src.Tool)
	assert.Empty(t, src.Text)
}

func TestPlainTextExtractor_Extract_EmptyData(t *testing.T) {
	ext := &PlainTextExtractor{}
	src, err := ext.Extract(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, src.Tool)
	assert.Empty(t, src.Text)
}

func TestImageOCRExtractor_Extract_EmptyData(t *testing.T) {
	ext := &ImageOCRExtractor{}
	src, err := ext.Extract(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, src.Tool)
	assert.Empty(t, src.Text)
}

func TestPDFOCRExtractor_Extract_EmptyData(t *testing.T) {
	ext := &PDFOCRExtractor{}
	src, err := ext.Extract(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, src.Tool)
	assert.Empty(t, src.Text)
}

func TestPDFOCRExtractor_Tool(t *testing.T) {
	ext := &PDFOCRExtractor{}
	assert.Equal(t, "tesseract", ext.Tool())
}

func TestPDFOCRExtractor_Matches(t *testing.T) {
	ext := &PDFOCRExtractor{}
	assert.True(t, ext.Matches("application/pdf"))
	assert.False(t, ext.Matches("text/plain"))
	assert.False(t, ext.Matches("image/png"))
}

func TestPDFOCRExtractor_Available(t *testing.T) {
	ext := &PDFOCRExtractor{}
	assert.Equal(t, OCRAvailable(), ext.Available())
}

func TestImageOCRExtractor_Tool(t *testing.T) {
	ext := &ImageOCRExtractor{}
	assert.Equal(t, "tesseract", ext.Tool())
}

func TestImageOCRExtractor_Matches(t *testing.T) {
	ext := &ImageOCRExtractor{}
	assert.True(t, ext.Matches("image/png"))
	assert.True(t, ext.Matches("image/jpeg"))
	assert.True(t, ext.Matches("image/tiff"))
	assert.False(t, ext.Matches("application/pdf"))
	assert.False(t, ext.Matches("text/plain"))
}

func TestImageOCRExtractor_Available(t *testing.T) {
	ext := &ImageOCRExtractor{}
	assert.Equal(t, ImageOCRAvailable(), ext.Available())
}

// --- DefaultExtractors ---

func TestDefaultExtractors_Order(t *testing.T) {
	extractors := DefaultExtractors(0, 0)
	require.Len(t, extractors, 4)
	assert.Equal(t, "pdftotext", extractors[0].Tool())
	assert.Equal(t, "plaintext", extractors[1].Tool())
	assert.Equal(t, "tesseract", extractors[2].Tool()) // PDFOCRExtractor
	assert.Equal(t, "tesseract", extractors[3].Tool()) // ImageOCRExtractor

	// Verify MIME matching distinguishes the two tesseract extractors.
	assert.True(t, extractors[2].Matches("application/pdf"))
	assert.False(t, extractors[2].Matches("image/png"))
	assert.True(t, extractors[3].Matches("image/png"))
	assert.False(t, extractors[3].Matches("application/pdf"))
}

func TestDefaultExtractors_Passthrough(t *testing.T) {
	extractors := DefaultExtractors(42, 99)
	pdfExt, ok := extractors[0].(*PDFTextExtractor)
	require.True(t, ok)
	assert.Equal(t, 99, int(pdfExt.Timeout))

	ocrExt, ok := extractors[2].(*PDFOCRExtractor)
	require.True(t, ok)
	assert.Equal(t, 42, ocrExt.MaxPages)
}

// --- HasMatchingExtractor ---

func TestHasMatchingExtractor_Tesseract_PDF(t *testing.T) {
	extractors := DefaultExtractors(0, 0)
	got := HasMatchingExtractor(extractors, "tesseract", "application/pdf")
	assert.Equal(t, OCRAvailable(), got)
}

func TestHasMatchingExtractor_Tesseract_Image(t *testing.T) {
	extractors := DefaultExtractors(0, 0)
	got := HasMatchingExtractor(extractors, "tesseract", "image/png")
	assert.Equal(t, ImageOCRAvailable(), got)
}

func TestHasMatchingExtractor_Pdftotext(t *testing.T) {
	extractors := DefaultExtractors(0, 0)
	got := HasMatchingExtractor(extractors, "pdftotext", "application/pdf")
	assert.Equal(t, HasPDFToText(), got)
}

func TestHasMatchingExtractor_NoMatch(t *testing.T) {
	extractors := DefaultExtractors(0, 0)
	assert.False(t, HasMatchingExtractor(extractors, "tesseract", "text/plain"))
	assert.False(t, HasMatchingExtractor(extractors, "pdftotext", "image/png"))
	assert.False(t, HasMatchingExtractor(extractors, "nonexistent", "application/pdf"))
}

// --- NeedsOCR ---

func TestNeedsOCR_PDF(t *testing.T) {
	extractors := DefaultExtractors(0, 0)
	got := NeedsOCR(extractors, "application/pdf")
	assert.Equal(t, OCRAvailable(), got)
}

func TestNeedsOCR_Image(t *testing.T) {
	extractors := DefaultExtractors(0, 0)
	got := NeedsOCR(extractors, "image/png")
	assert.Equal(t, ImageOCRAvailable(), got)
}

func TestNeedsOCR_PlainText(t *testing.T) {
	extractors := DefaultExtractors(0, 0)
	assert.False(t, NeedsOCR(extractors, "text/plain"))
}

func TestNeedsOCR_NoOCRExtractors(t *testing.T) {
	extractors := []Extractor{&PlainTextExtractor{}, &PDFTextExtractor{}}
	assert.False(t, NeedsOCR(extractors, "application/pdf"))
	assert.False(t, NeedsOCR(extractors, "image/png"))
}

// --- ExtractorTimeout / ExtractorMaxPages ---

func TestExtractorTimeout(t *testing.T) {
	extractors := DefaultExtractors(0, 42)
	assert.Equal(t, time.Duration(42), ExtractorTimeout(extractors))
}

func TestExtractorTimeout_NoPDFText(t *testing.T) {
	extractors := []Extractor{&PlainTextExtractor{}}
	assert.Equal(t, time.Duration(0), ExtractorTimeout(extractors))
}

func TestExtractorMaxPages(t *testing.T) {
	extractors := DefaultExtractors(15, 0)
	assert.Equal(t, 15, ExtractorMaxPages(extractors))
}

func TestExtractorMaxPages_NoOCR(t *testing.T) {
	extractors := []Extractor{&PlainTextExtractor{}}
	assert.Equal(t, 0, ExtractorMaxPages(extractors))
}

// --- Integration Extract tests ---

func TestPDFTextExtractor_Extract(t *testing.T) {
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}

	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}

	ext := &PDFTextExtractor{}
	src, err := ext.Extract(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, "pdftotext", src.Tool)
	assert.Contains(t, src.Text, "Invoice")
	assert.NotEmpty(t, src.Desc)
}

func TestPDFOCRExtractor_Extract(t *testing.T) {
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftoppm not available")
	}

	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}

	ext := &PDFOCRExtractor{MaxPages: 5}
	src, err := ext.Extract(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, "tesseract", src.Tool)
	assert.NotEmpty(t, src.Text)
	assert.NotEmpty(t, src.Data)
}

func TestImageOCRExtractor_Extract(t *testing.T) {
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	imgPath := filepath.Join("testdata", "invoice.png")
	data, err := os.ReadFile(imgPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+imgPath)
	}

	ext := &ImageOCRExtractor{}
	src, err := ext.Extract(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, "tesseract", src.Tool)
	assert.NotEmpty(t, src.Text)
	assert.NotEmpty(t, src.Data)
}
