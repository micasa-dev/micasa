// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"os/exec"
	"sync"
)

// OCRTools holds resolved absolute filesystem paths for the external
// binaries the extract package shells out to. An empty string for any
// field means the binary was not found on PATH at resolution time.
//
// Production code should obtain a process-wide instance via
// DefaultOCRTools (lazy LookPath cached across the process). Tests
// can construct an OCRTools directly with stub or deliberately
// invalid paths to drive failure paths without mutating PATH.
type OCRTools struct {
	// PDFInfo is the absolute path to the pdfinfo binary
	// (poppler-utils). Used by ocrPDF for page-count discovery.
	PDFInfo string
	// PDFToCairo is the absolute path to the pdftocairo binary
	// (poppler-utils). Used by ocrPage to rasterize PDF pages.
	PDFToCairo string
	// PDFToText is the absolute path to the pdftotext binary
	// (poppler-utils). Used by extractPDF for digital text extraction.
	PDFToText string
	// Tesseract is the absolute path to the tesseract binary. Used by
	// every OCR helper for image-to-text recognition.
	Tesseract string
}

// PDFOCRAvailable reports whether the tools needed for PDF OCR are all
// resolved: tesseract (recognition), pdftocairo (rasterization), and
// pdfinfo (page-count discovery).
func (t *OCRTools) PDFOCRAvailable() bool {
	return t != nil && t.Tesseract != "" && t.PDFToCairo != "" && t.PDFInfo != ""
}

// ImageOCRAvailable reports whether tesseract is resolved (the only
// binary needed for direct image OCR).
func (t *OCRTools) ImageOCRAvailable() bool {
	return t != nil && t.Tesseract != ""
}

// ResolveOCRTools runs exec.LookPath for each external binary the
// extract package depends on and returns an OCRTools populated with
// the absolute paths. Missing binaries produce an empty string in the
// corresponding field rather than an error so callers can interrogate
// individual fields without separate error handling.
func ResolveOCRTools() *OCRTools {
	return &OCRTools{
		PDFInfo:    lookPathOrEmpty("pdfinfo"),
		PDFToCairo: lookPathOrEmpty("pdftocairo"),
		PDFToText:  lookPathOrEmpty("pdftotext"),
		Tesseract:  lookPathOrEmpty("tesseract"),
	}
}

// DefaultOCRTools returns the process-wide OCRTools instance, resolving
// paths via ResolveOCRTools on first call. The result is cached for the
// lifetime of the process so the LookPath cost is paid once.
func DefaultOCRTools() *OCRTools {
	defaultOCRToolsOnce.Do(func() {
		defaultOCRTools = ResolveOCRTools()
	})
	return defaultOCRTools
}

var (
	defaultOCRToolsOnce sync.Once
	defaultOCRTools     *OCRTools
)

func lookPathOrEmpty(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// HasTesseract reports whether the tesseract binary is on PATH.
// Thin facade over DefaultOCRTools for callers that only need a bool.
func HasTesseract() bool { return DefaultOCRTools().Tesseract != "" }

// HasPDFToCairo reports whether the pdftocairo binary (from poppler-utils)
// is on PATH. Thin facade over DefaultOCRTools.
func HasPDFToCairo() bool { return DefaultOCRTools().PDFToCairo != "" }

// HasPDFToText reports whether the pdftotext binary (from poppler-utils)
// is on PATH. Thin facade over DefaultOCRTools.
func HasPDFToText() bool { return DefaultOCRTools().PDFToText != "" }

// HasPDFInfo reports whether the pdfinfo binary (from poppler-utils)
// is on PATH. Thin facade over DefaultOCRTools.
func HasPDFInfo() bool { return DefaultOCRTools().PDFInfo != "" }

// OCRAvailable reports whether tesseract and pdftocairo (with pdfinfo
// for page count discovery) are available.
func OCRAvailable() bool { return DefaultOCRTools().PDFOCRAvailable() }

// ImageOCRAvailable reports whether tesseract is available for direct
// image OCR (no PDF tools needed for image files).
func ImageOCRAvailable() bool { return DefaultOCRTools().ImageOCRAvailable() }
