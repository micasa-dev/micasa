// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"os/exec"
	"sync"
)

var (
	tesseractOnce  sync.Once
	tesseractFound bool
	pdftoppmOnce   sync.Once
	pdftoppmFound  bool
	pdftotextOnce  sync.Once
	pdftotextFound bool
	pdfimagesOnce  sync.Once
	pdfimagesFound bool
)

// HasTesseract reports whether the tesseract binary is on PATH.
// The result is cached for the process lifetime.
func HasTesseract() bool {
	tesseractOnce.Do(func() {
		_, err := exec.LookPath("tesseract")
		tesseractFound = err == nil
	})
	return tesseractFound
}

// HasPDFToPPM reports whether the pdftoppm binary (from poppler-utils)
// is on PATH. The result is cached for the process lifetime.
func HasPDFToPPM() bool {
	pdftoppmOnce.Do(func() {
		_, err := exec.LookPath("pdftoppm")
		pdftoppmFound = err == nil
	})
	return pdftoppmFound
}

// HasPDFToText reports whether the pdftotext binary (from poppler-utils)
// is on PATH. The result is cached for the process lifetime.
func HasPDFToText() bool {
	pdftotextOnce.Do(func() {
		_, err := exec.LookPath("pdftotext")
		pdftotextFound = err == nil
	})
	return pdftotextFound
}

// HasPDFImages reports whether the pdfimages binary (from poppler-utils)
// is on PATH. The result is cached for the process lifetime.
func HasPDFImages() bool {
	pdfimagesOnce.Do(func() {
		_, err := exec.LookPath("pdfimages")
		pdfimagesFound = err == nil
	})
	return pdfimagesFound
}

// OCRAvailable reports whether tesseract and at least one PDF image
// extraction tool (pdfimages or pdftoppm) are available.
func OCRAvailable() bool {
	return HasTesseract() && (HasPDFImages() || HasPDFToPPM())
}

// ImageOCRAvailable reports whether tesseract is available for direct
// image OCR (no pdftoppm needed for image files).
func ImageOCRAvailable() bool {
	return HasTesseract()
}
