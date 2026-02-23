<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Extract: Replace pdftoppm rasterization with pdfimages

## Status: In Progress

## Motivation

The extraction pipeline rasterizes every PDF page at 300 DPI via `pdftoppm`
before feeding them to tesseract. For a 20-page document this takes ~20s and
dominates the extraction time. But scanned PDFs already contain the page images
as embedded blobs -- rasterization just re-renders pixels that already exist.

## Design

Replace `pdftoppm` rasterization with `pdfimages` extraction:

1. **pdftotext** (existing, unchanged) -- digital text
2. **pdfimages** -- extract embedded images from the PDF
3. **tesseract** (parallel) -- OCR only the extracted images
4. **Fallback** -- if pdfimages finds no images AND pdftotext found
   little/no text, fall back to pdftoppm rasterization for vector-path PDFs

### Image filtering

`pdfimages` extracts ALL embedded images including logos, icons, etc. Filter by
minimum dimensions (e.g., 100x100 pixels) to skip images too small for
meaningful OCR.

### Changes

- `tools.go` -- add `HasPDFImages()` check (poppler-utils, same package as
  pdftotext and pdftoppm)
- `ocr.go` -- add `extractPDFImages()` function using `pdfimages -all -p`
- `ocr.go` -- update `ocrPDF` to use pdfimages path with pdftoppm fallback
- `ocr_progress.go` -- update `ocrPDFWithProgress` similarly; change "rasterize"
  phase to "images" phase
- `extractor.go` -- update `PDFOCRExtractor.Available()` to prefer pdfimages
  but accept pdftoppm as fallback
- `tools.go` -- update `OCRAvailable()` to accept either pdfimages or pdftoppm

### Progress reporting

- Phase "images": extracting embedded images (fast, near-instant)
- Phase "extract": parallel OCR of extracted images (same as today)
- Fallback shows "rasterize" phase if pdftoppm fallback triggers

### Fallback heuristic

If `pdfimages` produces zero images above the size threshold AND the caller
signals that pdftotext produced little text (< 50 chars), fall back to the
pdftoppm rasterization path. This handles the rare vector-path PDF case.
