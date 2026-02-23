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
	"sort"
	"strings"
)

// DefaultMaxExtractPages is the default page limit for extraction. Front-loaded info
// (specs, warranty, maintenance) is typically in the first pages.
const DefaultMaxExtractPages = 20

// ocrPDF rasterizes a PDF with pdftoppm, then OCRs each page image.
func ocrPDF(ctx context.Context, data []byte, maxPages int) (string, []byte, error) {
	tmpDir, err := os.MkdirTemp("", "micasa-ocr-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	// Write PDF to temp file for pdftoppm.
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		return "", nil, fmt.Errorf("write temp pdf: %w", err)
	}

	// Rasterize: pdftoppm -png -r 300 -l <maxPages> input.pdf output
	outputPrefix := filepath.Join(tmpDir, "page")
	args := []string{
		"-png",
		"-r", "300",
		"-l", fmt.Sprintf("%d", maxPages),
		pdfPath,
		outputPrefix,
	}
	cmd := exec.CommandContext( //nolint:gosec // args are constructed internally, not user-supplied
		ctx,
		"pdftoppm",
		args...,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("pdftoppm: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	// Collect page images in sorted order.
	images, err := filepath.Glob(outputPrefix + "*.png")
	if err != nil {
		return "", nil, fmt.Errorf("glob page images: %w", err)
	}
	sort.Strings(images)

	if len(images) == 0 {
		return "", nil, nil
	}

	// OCR each page and collect TSV output.
	var allText strings.Builder
	var allTSV bytes.Buffer
	headerWritten := false

	for _, img := range images {
		pageText, pageTSV, err := ocrImageFile(ctx, img)
		if err != nil {
			continue // skip pages that fail
		}
		if pageText != "" {
			if allText.Len() > 0 {
				allText.WriteString("\n\n")
			}
			allText.WriteString(pageText)
		}
		// Concatenate TSV: write header once, skip header on subsequent pages.
		if len(pageTSV) > 0 {
			lines := bytes.SplitN(pageTSV, []byte("\n"), 2)
			if !headerWritten {
				allTSV.Write(pageTSV)
				headerWritten = true
			} else if len(lines) > 1 {
				// Skip TSV header line, append data lines only.
				allTSV.Write(lines[1])
			}
		}
	}

	return normalizeWhitespace(allText.String()), allTSV.Bytes(), nil
}

// ocrImage runs tesseract on raw image bytes.
func ocrImage(ctx context.Context, data []byte) (string, []byte, error) {
	tmpDir, err := os.MkdirTemp("", "micasa-ocr-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	imgPath := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(imgPath, data, 0o600); err != nil {
		return "", nil, fmt.Errorf("write temp image: %w", err)
	}

	return ocrImageFile(ctx, imgPath)
}

// ocrImageFile runs tesseract on an image file, returning extracted text
// and raw TSV output.
func ocrImageFile(ctx context.Context, imgPath string) (string, []byte, error) {
	// Run tesseract with TSV output to capture confidence/coordinates.
	var tsvBuf bytes.Buffer
	var stderr bytes.Buffer
	tsvCmd := exec.CommandContext(ctx, "tesseract", imgPath, "stdout", "tsv")
	tsvCmd.Stdout = &tsvBuf
	tsvCmd.Stderr = &stderr
	if err := tsvCmd.Run(); err != nil {
		return "", nil, fmt.Errorf("tesseract: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	tsvData := tsvBuf.Bytes()
	text := textFromTSV(tsvData)
	return text, tsvData, nil
}

// textFromTSV extracts plain text from tesseract TSV output.
// TSV columns: level, page_num, block_num, par_num, line_num, word_num,
// left, top, width, height, conf, text
// We extract the text column (index 11), grouping by line_num with spaces
// and by block/paragraph with newlines.
func textFromTSV(tsv []byte) string {
	lines := bytes.Split(tsv, []byte("\n"))
	if len(lines) < 2 {
		return ""
	}

	var result strings.Builder
	var lastBlock, lastPar, lastLine int
	first := true

	for _, line := range lines[1:] { // skip header
		fields := bytes.Split(line, []byte("\t"))
		if len(fields) < 12 {
			continue
		}

		word := strings.TrimSpace(string(fields[11]))
		if word == "" {
			continue
		}

		block := atoi(fields[2])
		par := atoi(fields[3])
		lineNum := atoi(fields[4])

		if !first {
			if block != lastBlock || par != lastPar {
				result.WriteString("\n\n")
			} else if lineNum != lastLine {
				result.WriteString("\n")
			} else {
				result.WriteString(" ")
			}
		}
		first = false

		result.WriteString(word)
		lastBlock = block
		lastPar = par
		lastLine = lineNum
	}

	return result.String()
}

// atoi parses a byte slice as an integer, returning 0 on failure.
func atoi(b []byte) int {
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// IsImageMIME reports whether the MIME type is an image format that
// tesseract can process.
func IsImageMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/tiff", "image/bmp", "image/webp":
		return true
	default:
		return false
	}
}
