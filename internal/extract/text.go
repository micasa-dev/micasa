// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DefaultTextTimeout is the default timeout for pdftotext.
const DefaultTextTimeout = 30 * time.Second

// ExtractText pulls plain text from document content based on MIME type.
// Returns empty string (not an error) for unsupported MIME types.
// PDF extraction uses pdftotext (poppler-utils) when available,
// returning empty for PDFs when the tool is missing. The timeout
// parameter caps how long pdftotext can run (0 = DefaultTextTimeout).
//
// This is a convenience wrapper that delegates to PDFTextExtractor and
// PlainTextExtractor. For full pipeline extraction, use Pipeline.Run.
func ExtractText(data []byte, mime string, timeout time.Duration) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	textExtractors := []Extractor{
		&PDFTextExtractor{Timeout: timeout},
		&PlainTextExtractor{},
	}
	for _, ext := range textExtractors {
		if !ext.Matches(mime) || !ext.Available() {
			continue
		}
		src, err := ext.Extract(context.Background(), data)
		if err != nil {
			return "", err
		}
		return src.Text, nil
	}
	return "", nil
}

// extractPDF shells out to pdftotext for text extraction. pdftotext
// preserves reading order and table layout better than pure-Go readers.
func extractPDF(data []byte, timeout time.Duration) (string, error) {
	tmpDir, err := os.MkdirTemp("", "micasa-text-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		return "", fmt.Errorf("write temp pdf: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext( //nolint:gosec // args are constructed internally
		ctx,
		"pdftotext",
		"-layout",
		pdfPath,
		"-", // stdout
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	return normalizeWhitespace(stdout.String()), nil
}

// collapseSpaces replaces runs of horizontal whitespace with a single space.
var collapseSpaces = regexp.MustCompile(`[^\S\n]+`)

// collapseNewlines replaces runs of 3+ newlines with exactly two.
var collapseNewlines = regexp.MustCompile(`\n{3,}`)

// normalizeWhitespace collapses excessive whitespace while preserving
// paragraph structure (double newlines).
func normalizeWhitespace(s string) string {
	s = collapseSpaces.ReplaceAllString(s, " ")
	s = collapseNewlines.ReplaceAllString(s, "\n\n")

	// Trim leading/trailing whitespace per line, then overall.
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// IsScanned returns true if the extracted text is empty or whitespace-only,
// indicating the document likely needs OCR.
func IsScanned(extractedText string) bool {
	return strings.TrimSpace(extractedText) == ""
}
