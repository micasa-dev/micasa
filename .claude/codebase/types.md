<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->
<!-- verified: 2026-03-10 -->

# Key Types & Interfaces

## Enums (internal/app/types.go)

### Mode
modeNormal, modeEdit, modeForm

### TabKind
tabProjects, tabQuotes, tabMaintenance, tabIncidents, tabAppliances, tabVendors, tabDocuments
- Methods: .String(), .singular(), .plural()

### FormKind
formHouse, formProject, formQuote, formMaintenance, formAppliance, formIncident, formServiceLog, formVendor, formDocument

### cellKind
cellText, cellMoney, cellDate, cellStatus, cellDrilldown, cellWarranty, cellUrgency, cellNotes, cellEntity, cellReadonly

## Core App Types (internal/app/)

### Model (model.go)
Central Bubbletea state. Key fields:
- zones (*zone.Manager), store (*data.Store)
- tabs []Tab, active int, detailStack []*detailContext
- mode Mode, fs formState, inlineInput *inlineInputState
- chat *chatState, dash dashState, ex extractState
- calendar *calendarState, columnFinder *columnFinderState
- undoStack/redoStack []undoEntry
- house data.HouseProfile, hasHouse bool
- styles *Styles, cur locale.Currency, unitSystem data.UnitSystem
- width, height int

### Tab (types.go)
- Kind TabKind, Name string, Handler TabHandler
- Table (bubbles table), Rows []rowMeta, CellRows [][]cell
- Specs []columnSpec (column metadata)
- ColCursor, ViewOffset (horizontal scroll)
- Sorts []sortEntry, Pins []filterPin
- FullRows/FullMeta/FullCellRows (pre-filter data)

### TabHandler interface (handlers.go)
- FormKind() FormKind
- Load(store, specs) (rows, cellrows, meta, error)
- Delete(store, id) error
- Restore(store, id) error
- StartAddForm(m) tea.Cmd
- StartEditForm(m, id) tea.Cmd
- InlineEdit(m, id, col, value) error
- SubmitForm(store, data, editID) (uint, error)
- Snapshot(store, id) (undoEntry, error)
- SyncFixedValues(store, specs)

8 implementations (one per entity tab; formHouse is handled separately):
projectHandler, quoteHandler, maintenanceHandler, incidentHandler,
applianceHandler, serviceLogHandler, vendorHandler, documentHandler

### columnSpec (types.go)
Title, Min/MaxWidth, Flex, Align, Kind (cellKind), Link (FK ref), FixedValues, HideOrder

### cell (types.go)
Value string, Kind cellKind, Null bool, LinkID uint

### formData interface (types.go)
Implemented by all 9 form data structs via formKind() FormKind. Replaces `any`.

### formState (types.go)
form *huh.Form, formData formData, formSnapshot formData, formDirty bool,
confirmDiscard/confirmQuit bool, editID uint, notesEditMode bool
- formKind() method derives FormKind from formData (formNone when nil)

### detailContext (types.go)
Parent info (tab, entity, ID), Breadcrumb string, Tab (the detail sub-tab)

## Data Layer Types (internal/data/)

### Store (store.go)
- db *gorm.DB, maxDocumentSize uint64, currency locale.Currency
- Key methods: Open, Close, Transaction, AutoMigrate, Backup, IsMicasaDB

### Entity Models (models.go) - all have ID uint, CreatedAt, UpdatedAt
- HouseProfile - address, year built, property details, insurance, HOA
- ProjectType - lookup table (not soft-deletable)
- Project - title, ProjectTypeID, status, dates, budget/actual (cents)
- Quote - ProjectID, VendorID, total/labor/materials/other (cents)
- Vendor - name (unique), contact, phone, email, website, notes
- MaintenanceCategory - lookup table (not soft-deletable)
- MaintenanceItem - name, categoryID, applianceID?, season, interval, dueDate, cost
- ServiceLogEntry - MaintenanceItemID (CASCADE), vendorID?, date, cost
- Appliance - name, brand, model, serial, warranty, cost
- Incident - title, status, severity, dates, applianceID?, vendorID?, cost
- Document - polymorphic (EntityKind, EntityID), data BLOB, SHA256, OCR
- DeletionRecord - audit trail (Entity, TargetID, DeletedAt, RestoredAt)
- Setting - key-value store
- ChatInput - persistent chat history

### Project Status Constants
ProjectStatusIdeating, Planned, Quoted, InProgress, Delayed, Completed, Abandoned

### Season Constants
SeasonSpring, SeasonSummer, SeasonFall, SeasonWinter

### Incident Constants
- Status: Open, InProgress, Resolved
- Severity: Urgent, Soon, Whenever

### FK Relationships
- Project -> ProjectType (RESTRICT)
- Quote -> Project (RESTRICT), Vendor (RESTRICT)
- MaintenanceItem -> MaintenanceCategory (RESTRICT), Appliance (SET NULL)
- ServiceLogEntry -> MaintenanceItem (CASCADE), Vendor (SET NULL)
- Incident -> Appliance (SET NULL), Vendor (SET NULL)
- Document -> polymorphic (EntityKind, EntityID), validated manually

### Generated Constants (meta_generated.go)
Table* (e.g., TableVendors = "vendors")
Col* (e.g., ColID = "id", ColName = "name", ColDeletedAt = "deleted_at")

## Config Types (internal/config/)

### Config (config.go)
- Chat (Enable *bool, LLM ChatLLM)
  - ChatLLM (Provider, BaseURL, Model, APIKey, Timeout, Thinking, ExtraContext)
- Extraction (MaxPages int, LLM ExtractionLLM, OCR)
  - ExtractionLLM (Enable *bool, Provider, BaseURL, Model, APIKey, Timeout, Thinking)
  - OCR (Enable *bool, TSV OCRTSV)
    - OCRTSV (Enable *bool, ConfidenceThreshold *int)
- Documents (MaxFileSize ByteSize, CacheTTL Duration)
- Locale (Currency string)
Each pipeline section is self-contained; no cross-section inheritance.

### Defaults
- Provider: "ollama", Model: "qwen3:0.6b", BaseURL: "http://localhost:11434"
- MaxPages: 0, CacheTTL: 30 days, LLMTimeout: 5m, OCR TSV threshold: 70

## LLM Types (internal/llm/)

### Client (client.go)
- Wraps any-llm-go provider
- Chat(ctx, messages, system, opts...) - streaming
- SetThinking(level), Model(), Provider(), BaseURL()

### Extract Types (internal/extract/)
- Extractor interface: Extract(ctx, data, mime) ([]TextSource, error)
- Pipeline: orchestrates extractors + optional LLM
- TextSource: Tool, Desc, Text, Data
- Operation: entity operation (INSERT/UPDATE/DELETE)
- Result: Sources, Operations, LLMRaw, LLMUsed, Err
