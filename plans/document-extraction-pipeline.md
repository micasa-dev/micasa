<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Document Extraction Pipeline

Issue: #200

Three independent, gracefully degrading layers for document intelligence:
text extraction, OCR, and LLM-powered structured pre-fill.

## Design Decisions

Settled in the issue; summarized here for implementation reference.

| Decision | Choice |
|---|---|
| PDF library | `ledongthuc/pdf` -- pure Go, no cgo, Apache-2.0, good text quality |
| Storage | Two columns on Document: `extracted_text` (string), `ocr_data` ([]byte) |
| Scanned detection | Empty/whitespace-only text layer = scanned |
| OCR rasterization | `pdftoppm` (poppler-utils) -- both `tesseract` and `pdftoppm` required |
| OCR output format | TSV -- captures confidence + coordinates; plain text derived from it |
| Extraction timing | Synchronous for v1; 1-3 page docs are fast enough |
| OCR page limit | 20 pages; important info is front-loaded in manuals |
| Extraction model | Configurable; recommend Qwen2.5 7B; separate from chat model |
| LLM strategy | One-pass, universal schema, all fields optional |
| Pre-fill UX | Hints struct pre-fills form fields; user confirms before save |
| Degradation | No errors ever; one-time tesseract hint; no hint for missing LLM |

## Architecture

```
internal/
  extract/
    extract.go      -- Pipeline orchestrator, Pipeline struct, Run()
    text.go         -- PDF/plain-text/markdown text extraction
    text_test.go
    ocr.go          -- Tesseract/pdftoppm integration, TSV parsing
    ocr_test.go
    llmextract.go   -- LLM-powered structured extraction, prompt, schema
    llmextract_test.go
    hints.go        -- ExtractionHints struct, universal schema types
    hints_test.go
    tools.go        -- External tool detection (tesseract, pdftoppm)
    tools_test.go
```

## Phased Implementation

### Phase 1: Foundation -- extract package + text extraction

1. Add `extracted_text` and `ocr_data` columns to `Document` model
   - `ExtractedText string` -- plain text for FTS and LLM input
   - `OCRData []byte` -- raw TSV from tesseract, preserved for future use
   - GORM AutoMigrate handles the column addition
   - Add column constants `ColExtractedText`, `ColOCRData`

2. Choose and integrate PDF text extraction library
   - Add `github.com/ledongthuc/pdf` to go.mod
   - Audit: pure Go, Apache-2.0, no transitive deps of concern

3. Create `internal/extract/` package
   - `text.go`: `ExtractText(data []byte, mime string) (string, error)`
     - `application/pdf` -> PDF text extraction via ledongthuc/pdf
     - `text/plain` -> direct string conversion
     - `text/markdown` -> direct string conversion
     - Other MIME types -> return empty string (not an error)
   - PDF extraction: read from `bytes.Reader`, concatenate page text
   - Normalize whitespace (collapse runs, trim)

4. Wire text extraction into document upload flow
   - After `parseDocumentFormData()` reads the file, call `extract.ExtractText()`
   - Store result in `doc.ExtractedText`
   - No UI change yet -- extraction happens silently

5. Tests
   - Unit tests with small embedded PDF fixtures (text + empty/scanned)
   - Verify empty text for non-text MIME types
   - Verify plain text passthrough

### Phase 2: OCR integration

1. External tool detection (`tools.go`)
   - `HasTesseract() bool` -- `exec.LookPath("tesseract")`
   - `HasPDFToPPM() bool` -- `exec.LookPath("pdftoppm")`
   - `OCRAvailable() bool` -- both present
   - Cache results per process lifetime (sync.Once)

2. Scanned PDF detection
   - After Phase 1 text extraction, if result is empty/whitespace -> scanned
   - Only attempt OCR for `application/pdf` and image MIME types

3. OCR pipeline (`ocr.go`)
   - `OCR(ctx context.Context, data []byte, mime string, maxPages int) (text string, tsv []byte, err error)`
   - For PDFs: write to temp file, `pdftoppm -png -r 300 -l <maxPages>` -> temp images
   - For images (png/jpg/tiff): use directly
   - Run `tesseract <image> stdout tsv` per page
   - Concatenate TSV output (single header row)
   - Derive plain text from TSV text column
   - Clean up temp files

4. Wire OCR into pipeline
   - If text extraction produced empty result and OCR is available -> run OCR
   - Store `doc.ExtractedText` (from TSV text column) and `doc.OCRData` (raw TSV)
   - If OCR not available for a scanned doc -> one-time status hint

5. Nix: add `tesseract` and `poppler_utils` to dev shell packages

6. Tests
   - Mock external tools (interface for command execution)
   - Test TSV parsing independently
   - Test scanned detection heuristic
   - Test graceful degradation when tools missing

### Phase 3: LLM extraction

1. Extraction hints struct (`hints.go`)
   ```go
   type ExtractionHints struct {
       DocumentType     string   // quote|invoice|receipt|manual|warranty|permit|inspection|contract|other
       TitleSuggestion  string
       Summary          string
       VendorHint       string   // matched against existing vendors
       TotalCents       *int64
       LaborCents       *int64
       MaterialsCents   *int64
       Date             *time.Time
       WarrantyExpiry   *time.Time
       EntityKindHint   string   // project|appliance|vendor|maintenance|quote|service_log
       EntityNameHint   string   // matched against existing entity names
       MaintenanceItems []MaintenanceHint
       Notes            string
   }

   type MaintenanceHint struct {
       Name           string
       IntervalMonths int
   }
   ```

2. Extraction prompt (`llmextract.go`)
   - `BuildExtractionPrompt(filename, mime string, sizeBytes int64, entities EntityContext, text string) []llm.Message`
   - System prompt: role, JSON schema, rules (all fields optional, match existing entities)
   - User message: metadata header + extracted text
   - Entity context: existing vendor names, project names, appliance names

3. JSON response parsing
   - `ParseExtractionResponse(raw string) (ExtractionHints, error)`
   - Strip markdown fences, unmarshal JSON
   - Parse money strings to cents (handle "$1,234.56", "1234.56", "1234")
   - Parse date strings to time.Time (handle common formats)
   - Validate enum values for document_type, entity_kind_hint

4. Extraction model config
   - Add `[extraction]` section to config.toml (or reuse `[llm]` with optional override)
   - `extraction.model` -- defaults to same as `llm.model`
   - `extraction.enabled` -- defaults to true when LLM is configured
   - Wire through Options -> Model

5. Tests
   - Test prompt construction with various inputs
   - Test JSON parsing with valid/malformed/partial responses
   - Test money and date parsing edge cases
   - Test entity matching logic

### Phase 4: Pipeline orchestrator + UI integration

1. Pipeline struct (`extract.go`)
   ```go
   type Pipeline struct {
       maxOCRPages    int
       llmClient      *llm.Client  // nil = skip LLM extraction
       entityContext  EntityContext  // existing entities for matching
   }

   type Result struct {
       ExtractedText string
       OCRData       []byte
       Hints         *ExtractionHints // nil if LLM unavailable or failed
       OCRUsed       bool
       LLMUsed       bool
   }

   func (p *Pipeline) Run(ctx context.Context, data []byte, filename string, mime string) (*Result, error)
   ```

2. Run sequence:
   a. Text extraction (always)
   b. OCR if text empty + tools available (optional)
   c. LLM extraction if client configured + text available (optional)
   d. Return Result; never return an error for missing capabilities

3. Wire into document upload form
   - After file read in `parseDocumentFormData()`, run `Pipeline.Run()`
   - Store `ExtractedText` and `OCRData` on the Document
   - If hints available, pre-fill form fields:
     - `TitleSuggestion` -> Title (if user left blank)
     - `VendorHint` -> match to existing vendor for quote forms
     - `TotalCents` etc -> pre-fill money fields
     - `Notes` / `Summary` -> Notes field
   - Status bar: brief "extracted N chars" or "OCR + extraction complete"

4. Graceful degradation UX
   - Track tesseract hint state in a Setting row ("tesseract_hint_shown")
   - On scanned doc without OCR: one-time `setStatusInfo("install tesseract for text extraction")`
   - Mark shown in DB so it's never repeated
   - No hint at all for missing LLM

5. Entity context loading
   - `LoadEntityContext(store *data.Store) EntityContext` -- fetch vendor/project/appliance names
   - Passed to Pipeline for LLM matching

6. Tests
   - Integration test: full pipeline with mock LLM
   - Test pre-fill mapping from hints to form fields
   - Test one-time hint logic
   - Test pipeline with various capability combinations (all 4 matrix quadrants)

## Nix Changes

- Add `tesseract` and `poppler_utils` to devShell packages (development only)
- They're runtime-optional; the app checks PATH at startup

## Config Changes

```toml
[extraction]
# Model for document extraction. Defaults to llm.model.
# model = "qwen2.5:7b"

# Maximum pages to OCR for scanned documents. Default: 20.
# max_ocr_pages = 20

# Set to false to disable LLM-powered extraction even when LLM is configured.
# Text extraction and OCR still work independently.
# enabled = true
```

## What's NOT in Scope

Per the issue:
- Semantic search / embeddings
- Vision model alternative to tesseract
- Cross-document reasoning ("compare these quotes")
- Auto-creating maintenance items from extracted schedules
- Source highlighting using OCR bounding boxes
- Async extraction / background processing
- Re-process existing documents (future "retroactive enrichment")
