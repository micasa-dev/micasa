// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractText pulls plain text from document content based on MIME type.
// Returns empty string (not an error) for unsupported MIME types.
func ExtractText(data []byte, mime string) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	switch {
	case mime == "application/pdf":
		return extractPDF(data)
	case strings.HasPrefix(mime, "text/"):
		return normalizeWhitespace(string(data)), nil
	default:
		return "", nil
	}
}

// extractPDF reads text from a PDF using ledongthuc/pdf.
func extractPDF(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}

	textReader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract pdf text: %w", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(textReader); err != nil {
		return "", fmt.Errorf("read pdf text: %w", err)
	}

	return normalizeWhitespace(buf.String()), nil
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
