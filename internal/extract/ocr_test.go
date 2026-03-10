// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextFromTSV(t *testing.T) {
	t.Parallel()
	// Simulated tesseract TSV output with header + data rows.
	// Columns: level page_num block_num par_num line_num word_num left top width height conf text
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t100\t200\t50\t12\t96\tHello\n" +
			"5\t1\t1\t1\t1\t2\t160\t200\t50\t12\t95\tworld\n" +
			"5\t1\t1\t1\t2\t1\t100\t220\t50\t12\t94\tSecond\n" +
			"5\t1\t1\t1\t2\t2\t160\t220\t50\t12\t93\tline\n" +
			"5\t1\t2\t1\t1\t1\t100\t300\t80\t12\t92\tNew\n" +
			"5\t1\t2\t1\t1\t2\t190\t300\t80\t12\t91\tblock\n",
	)

	text := textFromTSV(tsv)
	assert.Equal(t, "Hello world\nSecond line\n\nNew block", text)
}

func TestTextFromTSV_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, textFromTSV(nil))
	assert.Empty(t, textFromTSV([]byte("")))
	assert.Empty(t, textFromTSV([]byte("header\n")))
}

func TestTextFromTSV_EmptyWords(t *testing.T) {
	t.Parallel()
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t100\t200\t50\t12\t96\t\n" +
			"5\t1\t1\t1\t1\t2\t160\t200\t50\t12\t95\tword\n",
	)
	text := textFromTSV(tsv)
	assert.Equal(t, "word", text)
}

func TestTextFromTSV_ParagraphBreaks(t *testing.T) {
	t.Parallel()
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t100\t200\t50\t12\t96\tPar1\n" +
			"5\t1\t1\t2\t1\t1\t100\t250\t50\t12\t95\tPar2\n",
	)
	text := textFromTSV(tsv)
	assert.Equal(t, "Par1\n\nPar2", text)
}

func TestSpatialTextFromTSV(t *testing.T) {
	t.Parallel()
	// Same data as TestTextFromTSV: two lines in block 1, one line in block 2.
	// All confidence values are high (91-96), so none should appear with default threshold 70.
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t100\t200\t50\t12\t96\tHello\n" +
			"5\t1\t1\t1\t1\t2\t160\t200\t50\t12\t95\tworld\n" +
			"5\t1\t1\t1\t2\t1\t100\t220\t50\t12\t94\tSecond\n" +
			"5\t1\t1\t1\t2\t2\t160\t220\t50\t12\t93\tline\n" +
			"5\t1\t2\t1\t1\t1\t100\t300\t80\t12\t92\tNew\n" +
			"5\t1\t2\t1\t1\t2\t190\t300\t80\t12\t91\tblock\n",
	)

	result := SpatialTextFromTSV(tsv, DefaultOCRConfThreshold)
	// Each line should have a bounding box prefix and the words.
	assert.Contains(t, result, "Hello world")
	assert.Contains(t, result, "Second line")
	assert.Contains(t, result, "New block")
	// Bounding boxes should be present (no height, no confidence since all > 70).
	assert.Contains(t, result, "[100,200,110]")
	// No confidence annotation since all values are above threshold.
	assert.NotContains(t, result, ";")
	// Block break should produce a blank line between groups.
	assert.Contains(t, result, "\n\n")
}

func TestSpatialTextFromTSV_BoundingBoxNoHeight(t *testing.T) {
	t.Parallel()
	// Two words on one line: "Hello" at x=100,w=50 and "world" at x=160,w=50.
	// Line bbox should span from left=100 to right=210 (160+50), so width=110.
	// No height in the output format.
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t100\t200\t50\t12\t96\tHello\n" +
			"5\t1\t1\t1\t1\t2\t160\t200\t50\t12\t90\tworld\n",
	)

	result := SpatialTextFromTSV(tsv, DefaultOCRConfThreshold)
	// Bounding box: left=100, top=200, width=110 (160+50-100). No height.
	// Confidence 90 is above threshold 70, so not shown.
	assert.Contains(t, result, "[100,200,110]")
	assert.NotContains(t, result, ";")
	assert.Contains(t, result, "Hello world")
}

func TestSpatialTextFromTSV_ConfidenceBelowThreshold(t *testing.T) {
	t.Parallel()
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t10\t20\t40\t15\t85\tLow\n" +
			"5\t1\t1\t1\t1\t2\t60\t20\t30\t15\t45\tconf\n",
	)

	result := SpatialTextFromTSV(tsv, DefaultOCRConfThreshold)
	// Min confidence 45 is below threshold 70, so it should appear.
	assert.Contains(t, result, ";45]")
	assert.Contains(t, result, "Low conf")
}

func TestSpatialTextFromTSV_ConfidenceAboveThreshold(t *testing.T) {
	t.Parallel()
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t10\t20\t40\t15\t85\tHigh\n" +
			"5\t1\t1\t1\t1\t2\t60\t20\t30\t15\t92\tconf\n",
	)

	result := SpatialTextFromTSV(tsv, DefaultOCRConfThreshold)
	// Min confidence 85 is above threshold 70, so no confidence shown.
	assert.NotContains(t, result, ";")
	assert.Contains(t, result, "High conf")
}

func TestSpatialTextFromTSV_CustomThreshold(t *testing.T) {
	t.Parallel()
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t10\t20\t40\t15\t85\tWord\n" +
			"5\t1\t1\t1\t2\t1\t10\t40\t40\t15\t60\tSuspect\n",
	)

	// With threshold 90, the 85-confidence line should show confidence.
	result := SpatialTextFromTSV(tsv, 90)
	assert.Contains(t, result, ";85]")
	assert.Contains(t, result, ";60]")

	// With threshold 70, only the 60-confidence line should show confidence.
	result = SpatialTextFromTSV(tsv, 70)
	assert.NotContains(t, result, ";85]")
	assert.Contains(t, result, ";60]")

	// With threshold 0, no confidence is ever shown.
	result = SpatialTextFromTSV(tsv, 0)
	assert.NotContains(t, result, ";")
}

func TestSpatialTextFromTSV_MixedConfidenceLines(t *testing.T) {
	t.Parallel()
	// First line has high confidence, second has low. Only the low one
	// should show a confidence annotation.
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t100\t200\t50\t12\t95\tClean\n" +
			"5\t1\t1\t1\t1\t2\t160\t200\t50\t12\t92\tline\n" +
			"5\t1\t1\t1\t2\t1\t100\t220\t50\t12\t30\tGarbled\n" +
			"5\t1\t1\t1\t2\t2\t160\t220\t50\t12\t25\ttext\n",
	)

	result := SpatialTextFromTSV(tsv, DefaultOCRConfThreshold)
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 2)
	// First line: no confidence.
	assert.NotContains(t, lines[0], ";")
	assert.Contains(t, lines[0], "Clean line")
	// Second line: confidence shown (min 25).
	assert.Contains(t, lines[1], ";25]")
	assert.Contains(t, lines[1], "Garbled text")
}

func TestSpatialTextFromTSV_PageBreak(t *testing.T) {
	t.Parallel()
	// Simulate concatenated per-page TSV output. Each page is OCR'd
	// independently, so page_num is always 1. A decreasing block number
	// (e.g., page 1 ends with block 2, page 2 starts with block 1)
	// signals a page boundary and should produce a blank line.
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			// Page 1, block 1
			"5\t1\t1\t1\t1\t1\t100\t200\t50\t12\t96\tPage1\n" +
			// Page 1, block 2
			"5\t1\t2\t1\t1\t1\t100\t300\t60\t12\t95\tStill1\n" +
			// Page 2 starts: block resets to 1 (< lastBlock 2) => page break
			"5\t1\t1\t1\t1\t1\t100\t200\t70\t12\t94\tPage2\n",
	)

	result := SpatialTextFromTSV(tsv, DefaultOCRConfThreshold)
	lines := strings.Split(result, "\n")
	// Expect: "Page1" / "" (block break) / "Still1" / "" (page break) / "Page2"
	require.Len(t, lines, 5, "expected 5 lines (3 content + 2 blank separators)")
	assert.Contains(t, lines[0], "Page1")
	assert.Empty(t, lines[1], "blank line between blocks")
	assert.Contains(t, lines[2], "Still1")
	assert.Empty(t, lines[3], "blank line at page break")
	assert.Contains(t, lines[4], "Page2")
}

func TestSpatialTextFromTSV_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, SpatialTextFromTSV(nil, DefaultOCRConfThreshold))
	assert.Empty(t, SpatialTextFromTSV([]byte(""), DefaultOCRConfThreshold))
	assert.Empty(t, SpatialTextFromTSV([]byte("header\n"), DefaultOCRConfThreshold))
}

func TestSpatialTextFromTSV_EmptyWordsSkipped(t *testing.T) {
	t.Parallel()
	tsv := []byte(
		"level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
			"5\t1\t1\t1\t1\t1\t100\t200\t50\t12\t96\t\n" +
			"5\t1\t1\t1\t1\t2\t160\t200\t50\t12\t95\tword\n",
	)
	result := SpatialTextFromTSV(tsv, DefaultOCRConfThreshold)
	assert.Contains(t, result, "word")
	// Empty word should not produce its own bbox line.
	assert.NotContains(t, result, "\n")
}

func TestAtoi(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input  string
		expect int
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"100", 100},
		{"abc", 0},
		{"", 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expect, atoi([]byte(tt.input)), "input: %q", tt.input)
	}
}

func TestIsImageMIME(t *testing.T) {
	t.Parallel()
	assert.True(t, IsImageMIME("image/png"))
	assert.True(t, IsImageMIME("image/jpeg"))
	assert.True(t, IsImageMIME("image/tiff"))
	assert.True(t, IsImageMIME("image/bmp"))
	assert.True(t, IsImageMIME("image/webp"))
	assert.False(t, IsImageMIME("image/svg+xml"))
	assert.False(t, IsImageMIME("application/pdf"))
	assert.False(t, IsImageMIME("text/plain"))
}

func TestPDFOCRExtractor_UnsupportedMIME(t *testing.T) {
	t.Parallel()
	ext := &PDFOCRExtractor{}
	assert.False(t, ext.Matches("application/json"))
}

func TestImageOCRExtractor_UnsupportedMIME(t *testing.T) {
	t.Parallel()
	ext := &ImageOCRExtractor{}
	assert.False(t, ext.Matches("application/json"))
}

func TestOCRExtractor_EmptyData(t *testing.T) {
	t.Parallel()
	ext := &PDFOCRExtractor{}
	src, err := ext.Extract(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, src.Text)
}

func TestPDFOCR_Integration(t *testing.T) {
	t.Parallel()
	if !OCRAvailable() {
		skipOrFatalCI(t, "tesseract and/or pdftocairo not available")
	}

	pdfPath := filepath.Join("testdata", "sample.pdf")
	data, err := os.ReadFile(pdfPath) //nolint:gosec // test fixture path
	if err != nil {
		skipOrFatalCI(t, "test fixture not found: "+pdfPath)
	}

	ext := &PDFOCRExtractor{MaxPages: 20}
	src, err := ext.Extract(context.Background(), data)
	require.NoError(t, err)
	// The sample PDF has digital text, so tesseract should find something.
	assert.NotEmpty(t, src.Text)
	assert.NotEmpty(t, src.Data)
	assert.Contains(t, src.Text, "Invoice")
}

func TestImageOCR_Integration(t *testing.T) {
	t.Parallel()
	if !ImageOCRAvailable() {
		skipOrFatalCI(t, "tesseract not available")
	}

	imgPath := filepath.Join("testdata", "sample-text.png")
	if _, err := os.Stat(imgPath); err != nil {
		skipOrFatalCI(t, "test image fixture not found: "+imgPath)
	}

	data, err := os.ReadFile(imgPath) //nolint:gosec // test fixture path
	require.NoError(t, err)

	ext := &ImageOCRExtractor{}
	src, err := ext.Extract(context.Background(), data)
	require.NoError(t, err)
	assert.NotEmpty(t, src.Text)
	assert.NotEmpty(t, src.Data)
}
