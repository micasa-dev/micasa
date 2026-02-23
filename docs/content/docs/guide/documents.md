+++
title = "Documents"
weight = 9
description = "Attach files to projects, appliances, and other records."
linkTitle = "Documents"
+++

Attach files to your home records -- warranties, manuals, invoices, photos.

![Documents table](/images/documents.webp)

## Adding a document

1. Switch to the Docs tab (`f` to cycle forward)
2. Enter Edit mode (`i`), press `a`
3. Fill in a title and optional file path, then save (`ctrl+s`)

If you provide a file path, micasa reads the file into the database as a BLOB
(up to 50 MB). The title auto-fills from the filename when left blank.

### Quick add with extraction

Press `A` (shift+a) on the Docs tab to open a streamlined add form that
picks a file and immediately runs the extraction pipeline. This is the
fastest way to import a document when you want OCR and LLM hints.

You can also add documents from within a project or appliance detail view --
drill into the `Docs` column and press `a`. Documents added this way are
automatically linked to that record.

## Fields

| Column | Type | Description | Notes |
|-------:|------|-------------|-------|
| `ID` | auto | Auto-assigned | Read-only |
| `Title` | text | Document name | Required. Auto-filled from filename if blank |
| `Entity` | text | Linked record | E.g., "project #3". Only shown on top-level Docs tab |
| `Type` | text | MIME type | E.g., "application/pdf", "image/jpeg" |
| `Size` | text | File size | Human-readable (e.g., "2.5 MB"). Read-only |
| `Notes` | notes | Free-text annotations | Press `enter` to preview |
| `Updated` | date | Last modified | Read-only |

## File handling

- **Storage**: files are stored as BLOBs inside the SQLite database, so
  `micasa backup backup.db` backs up everything -- no sidecar files
- **Size limit**: 50 MB per file
- **MIME detection**: automatic from file contents and extension
- **Checksum**: SHA-256 hash stored for integrity
- **Cache**: when you open a document (`o`), micasa extracts it to the XDG
  cache directory and opens it with your OS viewer

## Entity linking

Documents can be linked to any record type: projects, incidents, appliances,
quotes, maintenance items, vendors, or service log entries. The link is set
automatically when adding from a drill view, or can be left empty for
standalone documents.

The `Entity` column on the top-level Docs tab shows which record a document
belongs to (e.g., "project #3", "appliance #7").

## Drill columns

The `Docs` column appears on the **Projects** and **Appliances** tabs, showing
how many documents are linked to each record. In Nav mode, press `enter` to
drill into a scoped document list for that record.

## Extraction pipeline

When you save a document with file data, micasa runs a three-layer extraction
pipeline to pull structured information out of the file. Each layer is
independent and degrades gracefully when its tools are unavailable.

### Layer 1: text extraction

Runs immediately during save. Extracts selectable text from PDFs using
`pdftotext` (from poppler-utils) which preserves reading order and table
layout. Plain-text files are read directly. Images skip this layer entirely.

### Layer 2: OCR

Triggers automatically when text extraction returns little or no text (scanned
PDFs) or when the file is an image (PNG, JPEG, TIFF, etc.). Requires
`pdftoppm` (for PDF rasterization) and `tesseract` to be installed. If these
tools are missing, OCR is silently skipped.

The OCR phase shows live progress in an overlay: rasterization page count, then
per-page OCR status.

### Layer 3: LLM extraction

When an LLM is configured, micasa sends the extracted text to a local model
that returns a JSON array of database operations (creates and updates) for
vendors, quotes, maintenance items, appliances, and the document itself. The
operations are validated against a strict allowlist before display.

The results appear as a **tabbed table preview** below the pipeline steps --
one tab per affected table, using the same column layout as the main UI. The
user reviews proposed changes and explicitly accepts before anything touches
the database. The LLM never writes directly.

The extraction model can be configured separately from the chat model (a small,
fast model works well here). See [Configuration]({{< ref
"/docs/reference/configuration" >}}) for the `[extraction]` section.

### Extraction overlay

An overlay shows real-time progress during OCR and LLM extraction. Each step
displays a status icon, elapsed time, and detail (page count, character count,
model name). The overlay has two modes:

**Pipeline mode** (default): navigate steps, expand logs, review the dimmed
operation preview below.

**Explore mode** (press `x`): full table navigation of the proposed operations.
Pipeline steps dim and the table preview becomes interactive with row/column
cursors and tab switching. Press `x` or `esc` to return to pipeline mode.

When extraction completes successfully, press `a` to accept the results and
apply them. On error the overlay stays open showing which step failed. Press
`esc` at any time to cancel and close.

| Key | Action |
|-----|--------|
| `a` | Accept results (when done, no errors) |
| `esc` | Cancel / exit explore mode |
| `j`/`k` | Navigate steps (pipeline) or rows (explore) |
| `h`/`l` | Navigate columns (explore) |
| `b`/`f` | Switch tabs (explore) |
| `enter` | Expand/collapse step logs |
| `r` | Rerun LLM step |
| `x` | Toggle explore mode |

See [Keybindings]({{< ref "/docs/reference/keybindings" >}}) for the full
reference.

### Requirements

Each pipeline layer depends on external tools. All are optional -- the
document always saves regardless of which tools are installed.

| Pipeline step | File types | Tools needed | Without it |
|---------------|------------|--------------|------------|
| Text extraction | PDF | `pdftotext` | No digital text extracted |
| Text extraction | `text/*` | _(none)_ | _(always available)_ |
| OCR | Scanned PDF | `pdftoppm` + `tesseract` | OCR skipped |
| OCR | Images (PNG, JPEG, TIFF, ...) | `tesseract` | OCR skipped |
| LLM extraction | Any with extracted text | Ollama (or compatible) | No structured hints |

`pdftotext` and `pdftoppm` ship together in the **poppler** utilities package.

#### Installing dependencies

| Platform | Command |
|----------|---------|
| Ubuntu / Debian | `sudo apt install poppler-utils tesseract-ocr` |
| Fedora / RHEL | `sudo dnf install poppler-utils tesseract` |
| Arch | `sudo pacman -S poppler tesseract` |
| macOS (Homebrew) | `brew install poppler tesseract` |
| Windows (MSYS2) | `pacman -S mingw-w64-x86_64-poppler mingw-w64-x86_64-tesseract-ocr` |
| Nix | `nix shell 'nixpkgs#poppler-utils' 'nixpkgs#tesseract'` |

The micasa dev shell (`nix develop`) includes both tools automatically.

For the LLM step, install [Ollama](https://ollama.com) and pull a model
(a small model like `qwen2.5:7b` works well). See
[Configuration]({{< ref "/docs/reference/configuration" >}}) for the
`[extraction]` section.

## Inline editing

In Edit mode, press `e` on the `Title` or `Notes` column to edit inline. Press
`e` on any other column to open the full edit form. The file attachment cannot
be changed after creation.
