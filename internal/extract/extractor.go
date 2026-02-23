// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"strings"
	"time"
)

// MIMEApplicationPDF is the MIME type for PDF documents.
const MIMEApplicationPDF = "application/pdf"

// TextSource holds text from a single extraction method.
type TextSource struct {
	Tool string // "pdftotext", "plaintext", "tesseract"
	Desc string // human description for LLM context
	Text string
	Data []byte // optional structured data (e.g. OCR TSV)
}

// Extractor extracts text from document bytes.
type Extractor interface {
	Tool() string
	Matches(mime string) bool
	Available() bool
	Extract(ctx context.Context, data []byte) (TextSource, error)
}

// DefaultExtractors returns the standard extractors in priority order:
// pdftotext, plaintext, PDF OCR, image OCR. Zero values for maxPages
// and timeout cause the concrete extractors to use their own defaults.
func DefaultExtractors(maxPages int, timeout time.Duration) []Extractor {
	return []Extractor{
		&PDFTextExtractor{Timeout: timeout},
		&PlainTextExtractor{},
		&PDFOCRExtractor{MaxPages: maxPages},
		&ImageOCRExtractor{},
	}
}

// HasMatchingExtractor reports whether any extractor in the list with
// the given tool name matches the MIME type and is available.
func HasMatchingExtractor(extractors []Extractor, tool string, mime string) bool {
	for _, ext := range extractors {
		if ext.Tool() == tool && ext.Matches(mime) && ext.Available() {
			return true
		}
	}
	return false
}

// ExtractorTimeout returns the timeout from the first PDFTextExtractor
// in the list, or 0 (meaning "use default") if none is found.
func ExtractorTimeout(extractors []Extractor) time.Duration {
	for _, ext := range extractors {
		if pte, ok := ext.(*PDFTextExtractor); ok {
			return pte.Timeout
		}
	}
	return 0
}

// ExtractorMaxPages returns the max pages from the first PDFOCRExtractor
// in the list, or 0 (meaning "use default") if none is found.
func ExtractorMaxPages(extractors []Extractor) int {
	for _, ext := range extractors {
		if ocr, ok := ext.(*PDFOCRExtractor); ok {
			return ocr.MaxPages
		}
	}
	return 0
}

// --- Concrete extractors ---

// PDFTextExtractor wraps pdftotext for digital PDF text extraction.
type PDFTextExtractor struct {
	Timeout time.Duration
}

func (e *PDFTextExtractor) Tool() string             { return "pdftotext" }
func (e *PDFTextExtractor) Matches(mime string) bool { return mime == MIMEApplicationPDF }
func (e *PDFTextExtractor) Available() bool          { return HasPDFToText() }

func (e *PDFTextExtractor) Extract(_ context.Context, data []byte) (TextSource, error) {
	if len(data) == 0 {
		return TextSource{}, nil
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = DefaultTextTimeout
	}
	text, err := extractPDF(data, timeout)
	if err != nil {
		return TextSource{}, err
	}
	return TextSource{
		Tool: "pdftotext",
		Desc: "Digital text extracted directly from the PDF. Accurate for pages with selectable text.",
		Text: text,
	}, nil
}

// PlainTextExtractor normalizes whitespace from text/* content.
type PlainTextExtractor struct{}

func (e *PlainTextExtractor) Tool() string             { return "plaintext" }
func (e *PlainTextExtractor) Matches(mime string) bool { return strings.HasPrefix(mime, "text/") }
func (e *PlainTextExtractor) Available() bool          { return true }

func (e *PlainTextExtractor) Extract(_ context.Context, data []byte) (TextSource, error) {
	if len(data) == 0 {
		return TextSource{}, nil
	}
	return TextSource{
		Tool: "plaintext",
		Desc: "Plain text content with normalized whitespace.",
		Text: normalizeWhitespace(string(data)),
	}, nil
}

// PDFOCRExtractor wraps ocrPDF for scanned PDF pages.
type PDFOCRExtractor struct {
	MaxPages int
}

func (e *PDFOCRExtractor) Tool() string             { return "tesseract" }
func (e *PDFOCRExtractor) Matches(mime string) bool { return mime == MIMEApplicationPDF }
func (e *PDFOCRExtractor) Available() bool          { return OCRAvailable() }

func (e *PDFOCRExtractor) Extract(ctx context.Context, data []byte) (TextSource, error) {
	if len(data) == 0 {
		return TextSource{}, nil
	}
	maxPages := e.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultMaxExtractPages
	}
	text, tsv, err := ocrPDF(ctx, data, maxPages)
	if err != nil {
		return TextSource{}, err
	}
	return TextSource{
		Tool: "tesseract",
		Desc: "Text recognized from rasterized page images. Covers scanned pages that pdftotext misses, but may contain OCR errors.",
		Text: text,
		Data: tsv,
	}, nil
}

// ImageOCRExtractor wraps ocrImage for direct image OCR.
type ImageOCRExtractor struct{}

func (e *ImageOCRExtractor) Tool() string             { return "tesseract" }
func (e *ImageOCRExtractor) Matches(mime string) bool { return IsImageMIME(mime) }
func (e *ImageOCRExtractor) Available() bool          { return ImageOCRAvailable() }

func (e *ImageOCRExtractor) Extract(ctx context.Context, data []byte) (TextSource, error) {
	if len(data) == 0 {
		return TextSource{}, nil
	}
	text, tsv, err := ocrImage(ctx, data)
	if err != nil {
		return TextSource{}, err
	}
	return TextSource{
		Tool: "tesseract",
		Desc: "Text recognized from the image. May contain OCR errors.",
		Text: text,
		Data: tsv,
	}, nil
}
