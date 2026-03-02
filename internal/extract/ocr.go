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
	"runtime"
	"sort"
	"strings"
	"sync"
)

// DefaultMaxExtractPages is the default page limit for extraction. Front-loaded info
// (specs, warranty, maintenance) is typically in the first pages.
const DefaultMaxExtractPages = 20

// ocrPageResult holds the OCR output for a single page.
type ocrPageResult struct {
	text string
	tsv  []byte
	err  error
}

// minOCRImageBytes is the minimum file size for an extracted image to be
// worth OCR-ing. Full-page scans are typically >10KB even at low DPI;
// logos and icons are <5KB.
const minOCRImageBytes = 10 * 1024

// ocrPDF extracts images from a PDF and OCRs them in parallel. It prefers
// pdfimages (extracts embedded image blobs, near-instant) and falls back to
// pdftoppm rasterization when pdfimages is unavailable or produces no
// usable images.
func ocrPDF(ctx context.Context, data []byte, maxPages int) (string, []byte, error) {
	tmpDir, err := os.MkdirTemp("", "micasa-ocr-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		return "", nil, fmt.Errorf("write temp pdf: %w", err)
	}

	images, _, err := acquireImages(ctx, pdfPath, tmpDir, maxPages)
	if err != nil {
		return "", nil, err
	}
	if len(images) == 0 {
		return "", nil, nil
	}

	results := ocrPagesParallel(ctx, images, nil)
	text, tsv := collectOCRResults(results)
	return text, tsv, nil
}

// acquireImages tries increasingly expensive strategies to get page images:
// pdfimages (extracts embedded blobs), pdftohtml (renders to images), then
// pdftoppm (full rasterization at 300 DPI). Returns the images, the tool
// name that produced them, and any error.
func acquireImages(
	ctx context.Context,
	pdfPath string,
	tmpDir string,
	maxPages int,
) ([]string, string, error) {
	if HasPDFImages() {
		images, err := extractPDFImages(ctx, pdfPath, tmpDir, maxPages)
		if err == nil && len(images) > 0 {
			return images, "pdfimages", nil
		}
	}

	if HasPDFToHTML() {
		images, err := extractPDFToHTMLImages(ctx, pdfPath, tmpDir, maxPages)
		if err == nil && len(images) > 0 {
			return images, "pdftohtml", nil
		}
	}

	if !HasPDFToPPM() {
		return nil, "", fmt.Errorf("no PDF image extraction tool available")
	}

	outputPrefix := filepath.Join(tmpDir, "page")
	if err := rasterize(ctx, pdfPath, outputPrefix, maxPages); err != nil {
		return nil, "", fmt.Errorf("pdftoppm: %w", err)
	}

	images, err := filepath.Glob(outputPrefix + "*.png")
	if err != nil {
		return nil, "", fmt.Errorf("glob page images: %w", err)
	}
	sort.Strings(images)
	return images, "pdftoppm", nil
}

// extractPDFImages uses pdfimages to extract embedded images from a PDF,
// filtering out images smaller than minOCRImageDim in either dimension.
func extractPDFImages(
	ctx context.Context,
	pdfPath string,
	tmpDir string,
	maxPages int,
) ([]string, error) {
	outputPrefix := filepath.Join(tmpDir, "img")
	args := []string{"-all", "-p"}
	if maxPages > 0 {
		args = append(args, "-l", fmt.Sprintf("%d", maxPages))
	}
	args = append(args, pdfPath, outputPrefix)

	cmd := exec.CommandContext( //nolint:gosec // args are constructed internally
		ctx,
		"pdfimages",
		args...,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf(
			"pdfimages: %s: %w",
			strings.TrimSpace(stderr.String()),
			err,
		)
	}

	// Collect all extracted image files, filtering out tiny images.
	pattern := outputPrefix + "*"
	candidates, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob extracted images: %w", err)
	}
	sort.Strings(candidates)

	var images []string
	for _, path := range candidates {
		if isOCRWorthy(path) {
			images = append(images, path)
		}
	}
	return images, nil
}

// extractPDFToHTMLImages uses pdftohtml to render PDF pages to PNG images.
// This catches PDFs whose content is drawn with vector operations rather
// than embedded image XObjects (which pdfimages would miss).
func extractPDFToHTMLImages(
	ctx context.Context,
	pdfPath string,
	tmpDir string,
	maxPages int,
) ([]string, error) {
	htmlDir := filepath.Join(tmpDir, "html")
	if err := os.MkdirAll(htmlDir, 0o700); err != nil {
		return nil, fmt.Errorf("create html dir: %w", err)
	}

	outputPrefix := filepath.Join(htmlDir, "page")
	args := []string{
		"-noframes",
		"-fmt", "png",
		"-q",
	}
	if maxPages > 0 {
		args = append(args, "-l", fmt.Sprintf("%d", maxPages))
	}
	args = append(args, pdfPath, outputPrefix+".html")

	cmd := exec.CommandContext( //nolint:gosec // args are constructed internally
		ctx,
		"pdftohtml",
		args...,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf(
			"pdftohtml: %s: %w",
			strings.TrimSpace(stderr.String()),
			err,
		)
	}

	candidates, err := filepath.Glob(filepath.Join(htmlDir, "*.png"))
	if err != nil {
		return nil, fmt.Errorf("glob html images: %w", err)
	}
	sort.Strings(candidates)

	var images []string
	for _, path := range candidates {
		if isOCRWorthy(path) {
			images = append(images, path)
		}
	}
	return images, nil
}

// isOCRWorthy checks whether an image file is large enough to contain
// meaningful text, using file size as a proxy for image dimensions.
func isOCRWorthy(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Size() >= minOCRImageBytes
}

// ocrPagesParallel runs tesseract on multiple page images concurrently,
// capping parallelism at runtime.NumCPU(). Results are returned in page
// order. If pageDone is non-nil, a value is sent after each page completes
// (for progress reporting).
func ocrPagesParallel(
	ctx context.Context,
	images []string,
	pageDone chan<- struct{},
) []ocrPageResult {
	n := len(images)
	results := make([]ocrPageResult, n)

	workers := runtime.NumCPU()
	if workers > n {
		workers = n
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, img := range images {
		wg.Add(1)
		go func(idx int, imgPath string) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = ocrPageResult{err: ctx.Err()}
				return
			}

			text, tsv, err := ocrImageFile(ctx, imgPath)
			results[idx] = ocrPageResult{text: text, tsv: tsv, err: err}

			if pageDone != nil {
				select {
				case pageDone <- struct{}{}:
				case <-ctx.Done():
				}
			}
		}(i, img)
	}

	wg.Wait()
	return results
}

// collectOCRResults concatenates page results in order into combined text
// and TSV output. Pages that failed are silently skipped.
func collectOCRResults(results []ocrPageResult) (string, []byte) {
	var allText strings.Builder
	var allTSV bytes.Buffer
	headerWritten := false

	for _, r := range results {
		if r.err != nil {
			continue
		}
		if r.text != "" {
			if allText.Len() > 0 {
				allText.WriteString("\n\n")
			}
			allText.WriteString(r.text)
		}
		if len(r.tsv) > 0 {
			lines := bytes.SplitN(r.tsv, []byte("\n"), 2)
			if !headerWritten {
				allTSV.Write(r.tsv)
				headerWritten = true
			} else if len(lines) > 1 {
				allTSV.Write(lines[1])
			}
		}
	}

	return normalizeWhitespace(allText.String()), allTSV.Bytes()
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
	// OMP_THREAD_LIMIT=1 forces single-threaded mode per process so our
	// worker pool controls parallelism without OpenMP oversubscription.
	var tsvBuf bytes.Buffer
	var stderr bytes.Buffer
	tsvCmd := exec.CommandContext( //nolint:gosec // imgPath is a temp file we created
		ctx,
		"tesseract",
		imgPath,
		"stdout",
		"tsv",
	)
	tsvCmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
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
