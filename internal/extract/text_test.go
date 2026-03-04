// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractText_PlainText(t *testing.T) {
	t.Parallel()
	text, err := ExtractText([]byte("Hello, world!"), "text/plain", 0)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", text)
}

func TestExtractText_Markdown(t *testing.T) {
	t.Parallel()
	md := "# Heading\n\nSome paragraph text.\n"
	text, err := ExtractText([]byte(md), "text/markdown", 0)
	require.NoError(t, err)
	assert.Equal(t, "# Heading\n\nSome paragraph text.", text)
}

func TestExtractText_PlainTextWhitespaceNormalized(t *testing.T) {
	t.Parallel()
	input := "  lots   of    spaces  \n\n\n\n\nparagraph two  "
	text, err := ExtractText([]byte(input), "text/plain", 0)
	require.NoError(t, err)
	assert.Equal(t, "lots of spaces\n\nparagraph two", text)
}

func TestExtractText_EmptyData(t *testing.T) {
	t.Parallel()
	text, err := ExtractText(nil, "application/pdf", 0)
	require.NoError(t, err)
	assert.Empty(t, text)
}

func TestExtractText_UnsupportedMIME(t *testing.T) {
	t.Parallel()
	text, err := ExtractText([]byte{0xFF, 0xD8}, "image/jpeg", 0)
	require.NoError(t, err)
	assert.Empty(t, text)
}

func TestExtractText_OctetStream(t *testing.T) {
	t.Parallel()
	text, err := ExtractText([]byte{0x00, 0x01}, "application/octet-stream", 0)
	require.NoError(t, err)
	assert.Empty(t, text)
}

func TestExtractText_InvalidPDF(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}
	_, err := ExtractText([]byte("not a pdf"), "application/pdf", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdftotext")
}

func TestExtractText_PDF(t *testing.T) {
	t.Parallel()
	if !HasPDFToText() {
		skipOrFatalCI(t, "pdftotext not available")
	}
	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}
	text, err := ExtractText(data, "application/pdf", 0)
	require.NoError(t, err)
	assert.Contains(t, text, "Invoice")
}

func TestExtractText_PDFNoPdftotext(t *testing.T) {
	t.Parallel()
	// When pdftotext is missing, PDF extraction returns empty (not error).
	if HasPDFToText() {
		t.Skip("pdftotext is available; this test checks the missing-tool path")
	}
	text, err := ExtractText([]byte("%PDF-1.4 fake"), "application/pdf", 0)
	require.NoError(t, err)
	assert.Empty(t, text)
}

func TestIsScanned(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		text   string
		expect bool
	}{
		{"empty", "", true},
		{"whitespace only", "   \n\t  ", true},
		{"has content", "some text", false},
		{"single char", "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expect, IsScanned(tt.text))
		})
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"collapse spaces", "a   b   c", "a b c"},
		{"preserve single newline", "a\nb", "a\nb"},
		{"preserve double newline", "a\n\nb", "a\n\nb"},
		{"collapse triple+ newlines", "a\n\n\n\nb", "a\n\nb"},
		{"trim lines", "  a  \n  b  ", "a\nb"},
		{"tabs to space", "a\tb", "a b"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expect, normalizeWhitespace(tt.input))
		})
	}
}
