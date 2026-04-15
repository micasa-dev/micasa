// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// DefaultMaxPages is the default page limit for extraction.
// 0 means no limit (all pages are processed).
const DefaultMaxPages = 0

// ocrPageResult holds the OCR output for a single page.
type ocrPageResult struct {
	text string
	tsv  []byte
	err  error
}

// ocrPDF extracts text from a PDF using parallel per-page rasterization
// with pdftocairo fused with tesseract OCR. Each page is rasterized and
// OCR'd in a single goroutine, eliminating the sequential bottleneck.
// tools must have PDFInfo, PDFToCairo, and Tesseract populated.
func ocrPDF(
	ctx context.Context,
	tools *OCRTools,
	data []byte,
	maxPages int,
) (string, []byte, error) {
	tmpDir, err := os.MkdirTemp("", "micasa-ocr-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil { //nolint:gosec // path is tmpDir + constant filename
		return "", nil, fmt.Errorf("write temp pdf: %w", err)
	}

	pageCount, err := pdfPageCount(ctx, tools.PDFInfo, pdfPath)
	if err != nil {
		return "", nil, fmt.Errorf("pdfinfo: %w", err)
	}
	if maxPages > 0 && pageCount > maxPages {
		pageCount = maxPages
	}
	if pageCount == 0 {
		return "", nil, nil
	}

	results := ocrPDFPages(ctx, tools, pdfPath, pageCount, nil, nil)
	text, tsv := collectOCRResults(results)
	return text, tsv, nil
}

// pdfPageCount returns the number of pages in a PDF using pdfinfo.
// pdfInfoPath is the absolute path to the pdfinfo binary.
func pdfPageCount(ctx context.Context, pdfInfoPath, pdfPath string) (int, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext( //nolint:gosec // pdfInfoPath is resolved at startup, pdfPath is a temp file we created
		ctx,
		pdfInfoPath,
		pdfPath,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("%s: %w", strings.TrimSpace(stderr.String()), err)
	}

	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if field, ok := strings.CutPrefix(line, "Pages:"); ok {
			field = strings.TrimSpace(field)
			n, err := strconv.Atoi(field)
			if err != nil {
				return 0, fmt.Errorf("parse page count %q: %w", field, err)
			}
			return n, nil
		}
	}
	return 0, errors.New("pdfinfo output missing Pages field")
}

// ocrPage rasterizes a single PDF page with pdftocairo and pipes the PNG
// directly into tesseract for OCR, with no intermediate file on disk.
// If onRasterDone is non-nil, it is called after pdftocairo finishes
// (before tesseract completes) to enable per-stage progress reporting.
// tools must have PDFToCairo and Tesseract populated.
func ocrPage(
	ctx context.Context,
	tools *OCRTools,
	pdfPath string,
	page int,
	onRasterDone func(),
) ocrPageResult {
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
	cairoCmd := exec.CommandContext( //nolint:gosec // tools.PDFToCairo is resolved at startup, args constructed internally
		ctx,
		tools.PDFToCairo,
		cairoArgs...,
	)
	var cairoErr bytes.Buffer
	cairoCmd.Stderr = &cairoErr

	tessCmd := exec.CommandContext( //nolint:gosec // tools.Tesseract is resolved at startup
		ctx,
		tools.Tesseract,
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
		// Close the pipe reader so pdftocairo gets EPIPE on its next
		// write. On Linux that alone terminates pdftocairo, but on
		// Windows the pipe-close propagation is not guaranteed to
		// unblock a writer that has not yet flushed output, so kill
		// the process explicitly before waiting. Without this, a stub
		// tesseract with a real pdftocairo can hang indefinitely while
		// cairoCmd.Wait() waits for pdftocairo to notice its consumer
		// is gone.
		_ = pipe.Close()
		_ = cairoCmd.Process.Kill()
		_ = cairoCmd.Wait()
		return ocrPageResult{err: fmt.Errorf(
			"tesseract page %d: %s: %w",
			page, strings.TrimSpace(tessErr.String()), err,
		)}
	}

	// Wait for both to finish. Cairo must finish first so the pipe closes.
	cairoWaitErr := cairoCmd.Wait()
	if onRasterDone != nil {
		onRasterDone()
	}
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
// order. If rasterDone is non-nil, a value is sent after each page's
// pdftocairo finishes. If pageDone is non-nil, a value is sent after each
// page's tesseract finishes. tools must have PDFToCairo and Tesseract
// populated.
func ocrPDFPages(
	ctx context.Context,
	tools *OCRTools,
	pdfPath string,
	pageCount int,
	rasterDone chan<- struct{},
	pageDone chan<- struct{},
) []ocrPageResult {
	results := make([]ocrPageResult, pageCount)

	workers := min(runtime.NumCPU(), pageCount)

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

			var onRasterDone func()
			if rasterDone != nil {
				onRasterDone = func() {
					select {
					case rasterDone <- struct{}{}:
					case <-ctx.Done():
					}
				}
			}

			results[idx] = ocrPage(ctx, tools, pdfPath, idx+1, onRasterDone)

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

// ocrImage runs tesseract on raw image bytes. tesseractPath is the
// absolute path to the tesseract binary.
func ocrImage(ctx context.Context, tesseractPath string, data []byte) (string, []byte, error) {
	tmpDir, err := os.MkdirTemp("", "micasa-ocr-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck // best-effort cleanup

	imgPath := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(imgPath, data, 0o600); err != nil { //nolint:gosec // path is tmpDir + constant filename
		return "", nil, fmt.Errorf("write temp image: %w", err)
	}

	return ocrImageFile(ctx, tesseractPath, imgPath)
}

// ocrImageFile runs tesseract on an image file, returning extracted text
// and raw TSV output. tesseractPath is the absolute path to the tesseract
// binary.
func ocrImageFile(ctx context.Context, tesseractPath, imgPath string) (string, []byte, error) {
	// Run tesseract with TSV output to capture confidence/coordinates.
	// OMP_THREAD_LIMIT=1 forces single-threaded mode per process so our
	// worker pool controls parallelism without OpenMP oversubscription.
	var tsvBuf bytes.Buffer
	var stderr bytes.Buffer
	tsvCmd := exec.CommandContext( //nolint:gosec // tesseractPath is resolved at startup, imgPath is a temp file we created
		ctx,
		tesseractPath,
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

// DefaultOCRConfThreshold is the default confidence threshold below which
// OCR confidence annotations are included in spatial output. Lines with
// min confidence >= this threshold omit the confidence score to save tokens.
const DefaultOCRConfThreshold = 70

// SpatialTextFromTSV converts tesseract TSV output into a compact spatial
// format with line-level bounding boxes. Each output line has the form:
//
//	[left,top,width] word1 word2 ...
//
// When the minimum confidence for a line falls below confThreshold, the
// confidence is appended:
//
//	[left,top,width;minConf] word1 word2 ...
//
// Lines within the same block/paragraph are separated by newlines; block or
// paragraph breaks produce a blank line.
func SpatialTextFromTSV(tsv []byte, confThreshold int) string {
	lines := bytes.Split(tsv, []byte("\n"))
	if len(lines) < 2 {
		return ""
	}

	type lineAccum struct {
		words               []string
		left, top, right    int
		minConf             int
		block, par, lineNum int
	}

	var result strings.Builder
	var cur *lineAccum
	first := true

	flush := func() {
		if cur == nil || len(cur.words) == 0 {
			return
		}
		if !first {
			result.WriteByte('\n')
		}
		first = false
		w := cur.right - cur.left
		if cur.minConf < confThreshold {
			fmt.Fprintf(&result, "[%d,%d,%d;%d] %s",
				cur.left, cur.top, w, cur.minConf,
				strings.Join(cur.words, " "))
		} else {
			fmt.Fprintf(&result, "[%d,%d,%d] %s",
				cur.left, cur.top, w,
				strings.Join(cur.words, " "))
		}
		cur = nil
	}

	var lastBlock, lastPar int
	firstLine := true

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
		left := atoi(fields[6])
		top := atoi(fields[7])
		width := atoi(fields[8])
		conf := atoi(fields[10])

		// Detect line/block/paragraph changes.
		newLine := firstLine
		if !newLine && cur != nil {
			newLine = lineNum != cur.lineNum || block != cur.block || par != cur.par
		}

		// Detect page breaks in concatenated per-page TSV output.
		// Each page is OCR'd independently so page_num is always 1;
		// a decreasing block number signals a new page's data.
		pageBreak := !firstLine && block < lastBlock

		if !firstLine && newLine {
			// Insert block/paragraph/page break before flushing.
			if pageBreak || block != lastBlock || par != lastPar {
				flush()
				result.WriteByte('\n') // blank line for break
			} else {
				flush()
			}
		}
		firstLine = false
		lastBlock = block
		lastPar = par

		if cur == nil {
			cur = &lineAccum{
				left:    left,
				top:     top,
				right:   left + width,
				minConf: conf,
				block:   block,
				par:     par,
				lineNum: lineNum,
			}
		}

		cur.words = append(cur.words, word)
		// Expand bounding box to cover this word.
		if left < cur.left {
			cur.left = left
		}
		if top < cur.top {
			cur.top = top
		}
		if right := left + width; right > cur.right {
			cur.right = right
		}
		if conf < cur.minConf {
			cur.minConf = conf
		}
	}
	flush()

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
