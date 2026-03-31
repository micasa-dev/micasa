// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkOcrPDF(b *testing.B) {
	if !OCRAvailable() {
		b.Skip("tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		b.Skip("test fixture not found: testdata/sample.pdf")
	}

	b.ResetTimer()
	for b.Loop() {
		text, _, err := ocrPDF(b.Context(), data, 5)
		if err != nil {
			b.Fatal(err)
		}
		if text == "" {
			b.Fatal("no text extracted")
		}
	}
}

func BenchmarkOcrPage(b *testing.B) {
	if !OCRAvailable() {
		b.Skip("tesseract and/or pdftocairo not available")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "sample.pdf"))
	if err != nil {
		b.Skip("test fixture not found: testdata/sample.pdf")
	}

	tmpDir := b.TempDir()
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		result := ocrPage(b.Context(), pdfPath, 1, nil)
		if result.err != nil {
			b.Fatal(result.err)
		}
	}
}

func BenchmarkOcrImage(b *testing.B) {
	if !ImageOCRAvailable() {
		b.Skip("tesseract not available")
	}

	data, err := os.ReadFile(
		filepath.Join("testdata", "sample-text.png"),
	)
	if err != nil {
		b.Skip("test fixture not found: testdata/sample-text.png")
	}

	b.ResetTimer()
	for b.Loop() {
		text, _, err := ocrImage(b.Context(), data)
		if err != nil {
			b.Fatal(err)
		}
		if text == "" {
			b.Fatal("no text extracted")
		}
	}
}
