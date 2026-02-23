<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Extraction Pipeline Evolution

## Order of Operations

### Phase 1: Extractor Interface (#465)

Pure refactor, no behavior change. Replace PDF special-casing with generic
`Extractor` interface and `[]TextSource` pipeline.

- Implement `Extractor` interface (`Tool()`, `Matches()`, `Available()`, `Extract()`)
- Build `PlaintextExtractor`, `PdftotextExtractor`, `TesseractExtractor`
- Replace `PdfText`/`OCRText`/`ExtractedText` with `[]TextSource`
- Update LLM prompt builder to iterate sources generically
- Update extraction overlay to display `[]TextSource`

### Phase 2: SQL Output (#474)

Replace `ExtractionHints` JSON with LLM-generated SQL.

- Include `CREATE TABLE` DDL in the extraction prompt
- Include existing entity rows `(id, name)` for FK resolution
- LLM outputs `INSERT`/`UPDATE` statements
- Parse and validate SQL (whitelist INSERT/UPDATE on known tables only)
- Execute in a savepoint, show human-readable diff, user accepts/rejects
- Remove `ExtractionHints`, `MaintenanceHint`, JSON parsing, hints-to-form mapping

### Phase 3: Vision LLM Extractor (#466)

Add `VisionLLMExtractor` as a new `Extractor` implementation.

- Implement `VisionLLMExtractor` (sends page images to vision-capable model)
- Fire when images are present (`pdfimages -list` for PDFs, always for image files)
- Auto-detect vision-capable models from ollama
- Parallel execution: text extractors + vision extractor run concurrently

### Phase 4: Multi-Document PDFs (#469)

LLM emits multiple INSERT statements for PDFs containing distinct documents.

- Falls out naturally from Phase 2 (SQL output already supports multiple statements)
- UX for reviewing multiple proposed inserts from a single upload

## Data Flow

### Extractors

```
Extractor        Matches              Requires           Output
-----------      -----------------    -----------------  ----------------------
Plaintext        text/*               nothing            TextSource{"plaintext", ...}
Pdftotext        application/pdf      pdftotext          TextSource{"pdftotext", ...}
Tesseract        application/pdf,     tesseract,         TextSource{"tesseract", ...}
                 image/*              pdftoppm (PDFs)
VisionLLM        application/pdf,     vision model       TextSource{"vision-llm", ...}
                 image/*              in ollama
```

### Per File Type

**Plain text / markdown** (`text/*`)

```
Plaintext ──> TextSource{"plaintext", ...}
                │
                v
           LLM (text model) ──> SQL ──> savepoint ──> user review
```

**Digital PDF** (selectable text, no meaningful images)

```
Pdftotext ──> TextSource{"pdftotext", ...}
                │
                v
           LLM (text model) ──> SQL ──> savepoint ──> user review
```

Tesseract and VisionLLM skipped: pdftotext produced clean text, no images.

**Scanned PDF** (empty pdftotext, good OCR)

```
Pdftotext ──> TextSource{"pdftotext", ""}  (empty)
Tesseract ──> TextSource{"tesseract", ...}
                │
                v
           LLM (text model) ──> SQL ──> savepoint ──> user review
```

VisionLLM skipped if tesseract produced confident output.

**Scanned PDF, bad OCR** (handwriting, faded thermal, stamps)

```
Pdftotext  ──> TextSource{"pdftotext", ""}
Tesseract  ──> TextSource{"tesseract", ...}   (garbled)
VisionLLM  ──> TextSource{"vision-llm", ...}  (reads pixels directly)
                 │
                 v
            LLM (text model) ──> SQL ──> savepoint ──> user review
```

Final LLM sees all sources, picks the best interpretation.

**Image file** (phone photo of receipt, appliance label)

```
Tesseract ──> TextSource{"tesseract", ...}
VisionLLM ──> TextSource{"vision-llm", ...}
                │
                v
           LLM (text model) ──> SQL ──> savepoint ──> user review
```

**Mixed PDF** (digital pages + scanned pages)

```
Pdftotext  ──> TextSource{"pdftotext", ...}   (digital pages only)
Tesseract  ──> TextSource{"tesseract", ...}   (scanned pages)
VisionLLM  ──> TextSource{"vision-llm", ...}  (if images present)
                 │
                 v
            LLM (text model) ──> multiple INSERTs ──> savepoint ──> user review
```

### VisionLLM Trigger Heuristic

VisionLLM fires when images are present in the document:

- **PDFs**: `pdfimages -list` reports embedded images
- **Image files**: always (the file itself is the image)
- **Text files**: never

The final SQL-generating LLM always sees text only (`[]TextSource`). The
VisionLLM extractor does the image-to-text conversion upstream, keeping the
final step cheap.
