<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Extract: Fast image acquisition from PDFs

## Status: Implemented

## Motivation

The extraction pipeline rasterizes every PDF page at 300 DPI via `pdftoppm`
before feeding them to tesseract. For a 20-page document this takes ~20s and
dominates the extraction time. But scanned PDFs already contain the page images
as embedded blobs -- rasterization just re-renders pixels that already exist.

Some PDFs (e.g. home inspection reports) have no embedded image XObjects at all;
their content is drawn via vector operations. `pdfimages` returns nothing for
these, so we need a middle tier before falling all the way back to `pdftoppm`.

## Design

Three-tier fallback for image acquisition, in order of cost:

1. **pdfimages** -- extracts embedded image blobs directly (near-instant)
2. **pdftohtml** -- renders pages to PNG (faster than pdftoppm, catches
   vector-drawn content that pdfimages misses)
3. **pdftoppm** -- full 300 DPI rasterization (slowest, last resort)

After acquisition, all images are filtered by `isOCRWorthy` (>= 10KB) to skip
logos and icons, then OCR'd in parallel with tesseract.

### Changes

- `tools.go` -- `HasPDFImages()`, `HasPDFToHTML()` checks; `OCRAvailable()`
  accepts any of the three tools
- `ocr.go` -- `acquireImages()` implements the three-tier chain;
  `extractPDFImages()` for tier 1; `extractPDFToHTMLImages()` for tier 2
- `ocr_progress.go` -- `ocrPDFWithProgress` calls `acquireImages`; phase is
  `"images"` regardless of which tier succeeded
- `ocr.go` -- `OMP_THREAD_LIMIT=1` on tesseract subprocesses to prevent
  OpenMP oversubscription with our worker pool

### Progress reporting

- Phase `"images"`: image acquisition complete (any tier)
- Phase `"extract"`: parallel OCR progress (page N/M)
- Phase `"rasterize"`: only if pdftoppm fallback is the sole available tool
