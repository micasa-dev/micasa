<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

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

Implementations: projectHandler, quoteHandler, maintenanceHandler,
incidentHandler, applianceHandler, serviceLogHandler, vendorHandler, documentHandler

### columnSpec (types.go)
Title, Min/MaxWidth, Flex, Align, Kind (cellKind), Link (FK ref), FixedValues, HideOrder

### cell (types.go)
Value string, Kind cellKind, Null bool, LinkID uint

### formState (types.go)
formKind FormKind, form *huh.Form, formData any, formDirty bool,
confirmDiscard/confirmQuit bool, editID uint, notesEditMode bool

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
- MaintenanceItem - name, categoryID, applianceID?, interval, dueDate, cost
- ServiceLogEntry - MaintenanceItemID (CASCADE), vendorID?, date, cost
- Appliance - name, brand, model, serial, warranty, cost
- Incident - title, status, severity, dates, applianceID?, vendorID?, cost
- Document - polymorphic (EntityKind, EntityID), data BLOB, SHA256, OCR
- DeletionRecord - audit trail (Entity, TargetID, DeletedAt, RestoredAt)
- Setting - key-value store
- ChatInput - persistent chat history

### Project Status Constants
ProjectStatusIdeating, Planned, Quoted, InProgress, Delayed, Completed, Abandoned

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
- LLM (provider, model, baseURL, apiKey, timeout, thinking, extraContext)
  - Chat/Extraction overrides (LLMChatOverride, LLMExtractionOverride)
- Documents (MaxFileSize ByteSize, CacheTTL Duration)
- Extraction (MaxExtractPages int, Enabled *bool, TextTimeout, LLMTimeout)
- Locale (Currency string)

### Defaults
- Provider: "ollama", Model: "qwen3", BaseURL: "http://localhost:11434"
- MaxExtractPages: 20, CacheTTL: 30 days, TextTimeout: 30s, LLMTimeout: 5m

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
