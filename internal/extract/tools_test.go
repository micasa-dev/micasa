// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOCRAvailable(t *testing.T) {
	t.Parallel()
	// Smoke test: just verify the functions don't panic and return
	// consistent results across calls (sync.Once caching).
	r1 := OCRAvailable()
	r2 := OCRAvailable()
	assert.Equal(t, r1, r2)
}

func TestImageOCRAvailable(t *testing.T) {
	t.Parallel()
	r1 := ImageOCRAvailable()
	r2 := ImageOCRAvailable()
	assert.Equal(t, r1, r2)
}

func TestHasTesseract_Consistent(t *testing.T) {
	t.Parallel()
	r1 := HasTesseract()
	r2 := HasTesseract()
	assert.Equal(t, r1, r2)
}

func TestHasPDFToCairo_Consistent(t *testing.T) {
	t.Parallel()
	r1 := HasPDFToCairo()
	r2 := HasPDFToCairo()
	assert.Equal(t, r1, r2)
}

func TestHasPDFInfo_Consistent(t *testing.T) {
	t.Parallel()
	r1 := HasPDFInfo()
	r2 := HasPDFInfo()
	assert.Equal(t, r1, r2)
}

func TestOCRTools_AvailableEmptyFields(t *testing.T) {
	t.Parallel()

	var nilTools *OCRTools
	assert.False(t, nilTools.PDFOCRAvailable(), "nil receiver must be unavailable")
	assert.False(t, nilTools.ImageOCRAvailable(), "nil receiver must be unavailable")

	empty := &OCRTools{}
	assert.False(t, empty.PDFOCRAvailable())
	assert.False(t, empty.ImageOCRAvailable())

	// Tesseract alone is sufficient for image OCR but not PDF OCR.
	tessOnly := &OCRTools{Tesseract: "/bin/true"}
	assert.False(t, tessOnly.PDFOCRAvailable())
	assert.True(t, tessOnly.ImageOCRAvailable())

	// Missing pdfinfo defeats PDF OCR.
	missingInfo := &OCRTools{
		Tesseract:  "/bin/true",
		PDFToCairo: "/bin/true",
	}
	assert.False(t, missingInfo.PDFOCRAvailable())

	// Missing pdftocairo defeats PDF OCR.
	missingCairo := &OCRTools{
		Tesseract: "/bin/true",
		PDFInfo:   "/bin/true",
	}
	assert.False(t, missingCairo.PDFOCRAvailable())
}

func TestOCRTools_AvailableAllSet(t *testing.T) {
	t.Parallel()

	tools := &OCRTools{
		PDFInfo:    "/bin/true",
		PDFToCairo: "/bin/true",
		PDFToText:  "/bin/true",
		Tesseract:  "/bin/true",
	}
	assert.True(t, tools.PDFOCRAvailable())
	assert.True(t, tools.ImageOCRAvailable())
}

func TestResolveOCRTools_PopulatesFields(t *testing.T) {
	t.Parallel()

	// ResolveOCRTools must not panic regardless of PATH state and must
	// always return a non-nil pointer. Field values depend on the host;
	// we only assert structural invariants.
	tools := ResolveOCRTools()
	require.NotNil(t, tools)

	// Each populated field must be an absolute path (LookPath result).
	for _, p := range []string{tools.PDFInfo, tools.PDFToCairo, tools.PDFToText, tools.Tesseract} {
		if p == "" {
			continue
		}
		assert.True(t, filepath.IsAbs(p), "expected absolute path, got %q", p)
	}
}

// stubBinPath returns a path that is guaranteed not to exist on the
// filesystem so exec.Cmd.Start fails synchronously. Each test gets a
// unique path under a per-test temp dir.
func stubBinPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// writePDFFixture creates a small file with a non-PDF body in a temp
// directory and returns its path. We only need a real file because the
// tests stub out the binary that would parse it.
func writePDFFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "input.pdf")
	require.NoError(t, os.WriteFile(path, []byte("%PDF-stub"), 0o600))
	return path
}

func TestOCRTools_StubPath_PDFPageCount(t *testing.T) {
	t.Parallel()

	pdfPath := writePDFFixture(t)
	_, err := pdfPageCount(t.Context(), stubBinPath(t, "pdfinfo"), pdfPath)
	require.Error(t, err)
}

func TestOCRTools_StubPath_OcrPage_Cairo(t *testing.T) {
	t.Parallel()

	pdfPath := writePDFFixture(t)
	tools := &OCRTools{
		PDFToCairo: stubBinPath(t, "pdftocairo"),
		Tesseract:  stubBinPath(t, "tesseract"),
	}
	result := ocrPage(t.Context(), tools, pdfPath, 1, nil)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "pdftocairo")
}

func TestOCRTools_StubPath_OcrPage_TesseractOnly(t *testing.T) {
	if !HasPDFToCairo() {
		t.Skip("pdftocairo not available")
	}
	t.Parallel()

	// Need a real PDF so pdftocairo starts successfully and only
	// tesseract fails. Using an existing fixture from testdata.
	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		t.Skip("test fixture not found: testdata/sample.pdf")
	}
	pdfPath := filepath.Join(t.TempDir(), "input.pdf")
	//nolint:gosec // pdfPath is t.TempDir() + constant filename; data is a test fixture
	err = os.WriteFile(pdfPath, data, 0o600)
	require.NoError(t, err)

	tools := &OCRTools{
		PDFToCairo: DefaultOCRTools().PDFToCairo,
		Tesseract:  stubBinPath(t, "tesseract"),
	}
	result := ocrPage(t.Context(), tools, pdfPath, 1, nil)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "tesseract")
}

func TestOCRTools_StubPath_ExtractPDF(t *testing.T) {
	t.Parallel()

	_, err := extractPDF(t.Context(), stubBinPath(t, "pdftotext"), []byte("%PDF-stub"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdftotext")
}

func TestOCRTools_StubPath_OcrImageFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	imgPath := filepath.Join(dir, "input.png")
	require.NoError(t, os.WriteFile(imgPath, []byte("not a real png"), 0o600))

	_, _, err := ocrImageFile(t.Context(), stubBinPath(t, "tesseract"), imgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tesseract")
}

func TestOCRTools_StubPath_OcrImage(t *testing.T) {
	t.Parallel()

	_, _, err := ocrImage(t.Context(), stubBinPath(t, "tesseract"), []byte("not a real png"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tesseract")
}

func TestOCRTools_StubPath_OcrPDF(t *testing.T) {
	t.Parallel()

	tools := &OCRTools{
		PDFInfo:    stubBinPath(t, "pdfinfo"),
		PDFToCairo: stubBinPath(t, "pdftocairo"),
		Tesseract:  stubBinPath(t, "tesseract"),
	}
	_, _, err := ocrPDF(t.Context(), tools, []byte("%PDF-stub"), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdfinfo")
}

func TestOCRTools_StubPath_PDFTextExtractor(t *testing.T) {
	t.Parallel()

	stub := &OCRTools{
		PDFToText: stubBinPath(t, "pdftotext"),
	}
	ext := &PDFTextExtractor{Tools: stub}
	assert.True(
		t,
		ext.Available(),
		"non-empty path must be reported available regardless of existence",
	)

	src, err := ext.Extract(t.Context(), []byte("%PDF-stub"))
	require.Error(t, err)
	assert.Empty(t, src.Text)
}

func TestOCRTools_StubPath_ImageOCRExtractor(t *testing.T) {
	t.Parallel()

	stub := &OCRTools{
		Tesseract: stubBinPath(t, "tesseract"),
	}
	ext := &ImageOCRExtractor{Tools: stub}
	assert.True(t, ext.Available())

	src, err := ext.Extract(t.Context(), []byte("not a real png"))
	require.Error(t, err)
	assert.Empty(t, src.Text)
}

func TestOCRTools_StubPath_PDFOCRExtractor(t *testing.T) {
	t.Parallel()

	stub := &OCRTools{
		PDFInfo:    stubBinPath(t, "pdfinfo"),
		PDFToCairo: stubBinPath(t, "pdftocairo"),
		Tesseract:  stubBinPath(t, "tesseract"),
	}
	ext := &PDFOCRExtractor{Tools: stub}
	assert.True(t, ext.Available())

	src, err := ext.Extract(t.Context(), []byte("%PDF-stub"))
	require.Error(t, err)
	assert.Empty(t, src.Text)
}

func TestOCRTools_StubPath_ErrorIsRich(t *testing.T) {
	t.Parallel()

	// Verify the error wraps the underlying exec.Error so callers can
	// reach the path that failed via errors.As.
	stub := stubBinPath(t, "pdfinfo")
	_, err := pdfPageCount(t.Context(), stub, writePDFFixture(t))
	require.Error(t, err)

	pathErr, ok := errors.AsType[*os.PathError](err)
	require.True(t, ok, "expected wrapped *os.PathError, got %T: %v", err, err)
	assert.Equal(t, stub, pathErr.Path)
}
