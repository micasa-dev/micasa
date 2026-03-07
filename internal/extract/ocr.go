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
	"strconv"
	"strings"
	"sync"
)

// DefaultMaxExtractPages is the default page limit for extraction.
// 0 means no limit (all pages are processed).
const DefaultMaxExtractPages = 0

// ocrPageResult holds the OCR output for a single page.
type ocrPageResult struct {
	text string
	tsv  []byte
	err  error
}

// ocrPDF extracts text from a PDF using parallel per-page rasterization
// with pdftocairo fused with tesseract OCR. Each page is rasterized and
// OCR'd in a single goroutine, eliminating the sequential bottleneck.
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

	pageCount, err := pdfPageCount(ctx, pdfPath)
	if err != nil {
		return "", nil, fmt.Errorf("pdfinfo: %w", err)
	}
	if maxPages > 0 && pageCount > maxPages {
		pageCount = maxPages
	}
	if pageCount == 0 {
		return "", nil, nil
	}

	results := ocrPDFPages(ctx, pdfPath, pageCount, nil)
	text, tsv := collectOCRResults(results)
	return text, tsv, nil
}

// pdfPageCount returns the number of pages in a PDF using pdfinfo.
func pdfPageCount(ctx context.Context, pdfPath string) (int, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext( //nolint:gosec // pdfPath is a temp file we created
		ctx,
		"pdfinfo",
		pdfPath,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			field := strings.TrimSpace(strings.TrimPrefix(line, "Pages:"))
			n, err := strconv.Atoi(field)
			if err != nil {
				return 0, fmt.Errorf("parse page count %q: %w", field, err)
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("pdfinfo output missing Pages field")
}

// ocrPage rasterizes a single PDF page with pdftocairo and pipes the PNG
// directly into tesseract for OCR, with no intermediate file on disk.
func ocrPage(ctx context.Context, pdfPath string, page int) ocrPageResult {
	// pdftocairo streams the PNG to stdout; tesseract reads from stdin.
	cairoArgs := []string{
		"-png",
		"-r", "300",
		"-singlefile",
		"-f", strconv.Itoa(page),
		"-l", strconv.Itoa(page),
		pdfPath,
		"-", // stdout
	}
	cairoCmd := exec.CommandContext( //nolint:gosec // args are constructed internally
		ctx,
		"pdftocairo",
		cairoArgs...,
	)
	var cairoErr bytes.Buffer
	cairoCmd.Stderr = &cairoErr

	tessCmd := exec.CommandContext( //nolint:gosec // args are constructed internally
		ctx,
		"tesseract",
		"stdin",
		"stdout",
		"tsv",
	)
	tessCmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	var tsvBuf bytes.Buffer
	var tessErr bytes.Buffer
	tessCmd.Stdout = &tsvBuf
	tessCmd.Stderr = &tessErr

	// Connect pdftocairo stdout -> tesseract stdin.
	pipe, err := cairoCmd.StdoutPipe()
	if err != nil {
		return ocrPageResult{err: fmt.Errorf("pipe setup: %w", err)}
	}
	tessCmd.Stdin = pipe

	// Start both processes.
	if err := cairoCmd.Start(); err != nil {
		return ocrPageResult{err: fmt.Errorf(
			"pdftocairo page %d: %s: %w",
			page, strings.TrimSpace(cairoErr.String()), err,
		)}
	}
	if err := tessCmd.Start(); err != nil {
		_ = cairoCmd.Wait()
		return ocrPageResult{err: fmt.Errorf(
			"tesseract page %d: %s: %w",
			page, strings.TrimSpace(tessErr.String()), err,
		)}
	}

	// Wait for both to finish. Cairo must finish first so the pipe closes.
	cairoWaitErr := cairoCmd.Wait()
	tessWaitErr := tessCmd.Wait()

	if cairoWaitErr != nil {
		return ocrPageResult{err: fmt.Errorf(
			"pdftocairo page %d: %s: %w",
			page, strings.TrimSpace(cairoErr.String()), cairoWaitErr,
		)}
	}
	if tessWaitErr != nil {
		return ocrPageResult{err: fmt.Errorf(
			"tesseract page %d: %s: %w",
			page, strings.TrimSpace(tessErr.String()), tessWaitErr,
		)}
	}

	tsvData := tsvBuf.Bytes()
	text := textFromTSV(tsvData)
	return ocrPageResult{text: text, tsv: tsvData}
}

// ocrPDFPages runs fused pdftocairo|tesseract on each page in parallel,
// capping concurrency at runtime.NumCPU(). Results are returned in page
// order. If pageDone is non-nil, a value is sent after each page completes.
func ocrPDFPages(
	ctx context.Context,
	pdfPath string,
	pageCount int,
	pageDone chan<- struct{},
) []ocrPageResult {
	results := make([]ocrPageResult, pageCount)

	workers := runtime.NumCPU()
	if workers > pageCount {
		workers = pageCount
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i := range pageCount {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = ocrPageResult{err: ctx.Err()}
				return
			}

			results[idx] = ocrPage(ctx, pdfPath, idx+1) // 1-indexed pages

			if pageDone != nil {
				select {
				case pageDone <- struct{}{}:
				case <-ctx.Done():
				}
			}
		}(i)
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
