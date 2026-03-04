// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestHasPDFToPPM_Consistent(t *testing.T) {
	t.Parallel()
	r1 := HasPDFToPPM()
	r2 := HasPDFToPPM()
	assert.Equal(t, r1, r2)
}
