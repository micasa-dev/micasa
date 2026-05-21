<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->
<!-- verified: 2026-05-21 -->

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

8 implementations (one per entity tab; formHouse handled separately):
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
- Per-entity CRUD is split across store_*.go files

### Entity Models (models.go) - all have ID + CreatedAt + UpdatedAt unless noted
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
- SyncOplogEntry - ULID id, TableName, RowID, OpType, Payload (JSON),
                   DeviceID, CreatedAt, AppliedAt?, SyncedAt?
- SyncDevice - this device's identity (ID, Name, HouseholdID, RelayURL, LastSeq)

### Sync Oplog Hooks (oplog.go)
- syncApplyingKey: context key suppressing oplog writes when applying remote ops
- WithSyncApplying(ctx): mark ctx as remote-apply (prevents push loop)
- syncableTable(table): true for sync-eligible tables

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
Col* (e.g., ColID = "id", ColName = "name", ColDeletedAt = "deleted_at",
       ColOpType, ColPayload, ColAppliedAt, ColSyncedAt, ColTableName, ColRowID, ColDeviceID, ColCreatedAt)

## Config Types (internal/config/)

### Config (config.go)
- Chat (Enable *bool, LLM ChatLLM)
  - ChatLLM (Provider, BaseURL, Model, APIKey, Timeout, Effort, ExtraContext)
- Extraction (MaxPages int, LLM ExtractionLLM, OCR)
  - ExtractionLLM (Enable *bool, Provider, BaseURL, Model, APIKey, Timeout, Effort)
  - OCR (Enable *bool, TSV OCRTSV)
    - OCRTSV (Enable *bool, ConfidenceThreshold *int)
- Documents (MaxFileSize ByteSize, CacheTTL Duration)
- Locale (Currency string)
- Sync (Enable *bool, RelayURL, etc.)
Each pipeline section is self-contained; no cross-section inheritance.

### Defaults
- Provider: "ollama", Model: "qwen3", BaseURL: "http://localhost:11434"
- claude-cli: extraction-only (chat rejected by config validation),
  requires explicit model, base_url/api_key ignored
- MaxPages: 0, CacheTTL: 30 days, LLMTimeout: 5m, OCR TSV threshold: 70

## LLM Types (internal/llm/)

### Interfaces (provider.go)
- Base: shared model management (Model, SetModel, Ping, ListModels, Timeout, ...)
- ChatProvider: Base + ChatStream(ctx, messages)
- ExtractionProvider: Base + ExtractStream(ctx, messages, schema)

### Provider Constants (client.go)
providerOllama, providerLlamacpp, providerLlamafile, providerAnthropic,
providerOpenAI, providerOpenRouter, providerDeepseek, providerGemini,
providerGroq, providerMistral

### Client (client.go)
- Wraps any-llm-go provider, satisfies both ChatProvider and ExtractionProvider
- ChatStream: streaming text responses (NL->SQL, summaries)
- ExtractStream: streaming JSON schema-constrained responses
- SetEffort(level), Model(), ProviderName(), BaseURL(), IsLocalServer()

### claudecli.Client (internal/claudecli/)
- Implements ExtractionProvider by shelling out to claude CLI binary
- ExtractStream: NDJSON parser, input_json_delta events, first-turn early stop
- Uses cmdFactory DI for testability (TestHelperProcess re-exec pattern)
- Flags: --tools "" --disable-slash-commands --no-chrome --setting-sources local

## Extract Types (internal/extract/)

### Extractor (extractor.go)
- Interface: Matches(mime), Extract(ctx, data, mime) ([]TextSource, error)
- Implementations: PDFTextExtractor (pdftotext), PDFOCRExtractor (tesseract)
- Tool name constants: toolPDFToCairo, toolTesseract (ocr_progress.go)

### Pipeline (pipeline.go)
- Orchestrates extractors + optional LLM
- TextSource: Tool, Desc, Text, Data
- Operation: entity operation (INSERT/UPDATE/DELETE)
- Result: Sources, Operations, LLMRaw, LLMUsed, Err

### Schema Context (sqlcontext.go)
- ExtractionTableDefs: single source of truth for extraction table metadata
- Columns derived from generated meta via columnsFromMeta
- Actions, Required, Enum, Omit, synthetic columns: hand-maintained

### Constants
MIMEApplicationPDF = "application/pdf"

## Sync Types (internal/sync/)

### Envelope (types.go)
Encrypted payload moved between client and relay. Fields: HouseholdID,
DeviceID, OpID, TableName, RowID, OpType, Ciphertext, Nonce, Seq?, CreatedAt.

### PushRequest / PushResponse / PushConfirmation
Push: client -> relay; relay assigns sequence numbers, returns confirmations.

### PullResponse
relay -> client; encrypted envelopes plus a more-pages flag.

### Household (types.go)
ID, OwnerDeviceID, CreatedAt, StripeCustomerID?, StripeSubscriptionID?, StripeStatus?

### BlobStorage (types.go)
Used and quota counters for a household.

### Client (client.go)
HTTP client against the relay (Push, Pull, etc.). NewClient(baseURL, token, key);
NewManagementClient for admin-only flows.

### Engine (engine.go)
Engine.Sync(ctx) orchestrates pushAll + pullAll + uploadPendingBlobs + fetchPendingBlobs.
Returns SyncResult (pushed/pulled/conflicts/blobs).

### OpPayload (client.go)
Decrypted-side view of a sync op: ID, TableName, RowID, OpType, Payload, DeviceID, CreatedAt.

### DecryptedOp (client.go)
Pull-side: Envelope + decoded OpPayload.

### Apply (apply.go)
ApplyOps(ctx, db, ops): applies decrypted ops with LWW conflict resolution.
- applyInsert / applyUpdate / applyDelete / applyRestore
- lwwLocalWins: compare CreatedAt; ties broken by DeviceID lex order
- recordAppliedOp / recordUnappliedOp: write sync_oplog_entries row

## Relay Types (internal/relay/)

### Store interface (store.go) - 24 methods
- Push / Pull (encrypted ops)
- CreateHousehold / RegisterDevice / AuthenticateDevice
- CreateInvite / StartJoin / GetPendingExchanges / CompleteKeyExchange / GetKeyExchangeResult
- ListDevices / RevokeDevice
- GetHousehold / UpdateSubscription / HouseholdBySubscription / UpdateCustomerID / HouseholdByCustomer
- OpsCount / PutBlob / GetBlob / HasBlob / BlobUsage
- SetEncryptionKey / Close

### Implementations
- PgStore (pgstore.go): Postgres via GORM + rlsdb.DB.Tx for row-level security
- MemStore (memstore.go): in-memory for tests

### Constants
- maxInviteAttempts = 5, maxActiveInvites = 3
- inviteExpiry = 4h, keyExchangeExpiry = 15m
- lockForUpdate = "UPDATE" (GORM clause.Locking.Strength)

### Handler (handler.go)
HTTP handlers wiring Store to bearer-token-auth routes. ServeHTTP delegates
to internal http.ServeMux.

### rlsdb (rlsdb/) - row-level security wrapper
- DB struct, DB.Tx(ctx, householdID, fn): runs fn within a transaction with
  Postgres session settings that enforce RLS on the household's data
- Unexported *gorm.DB structurally prevents bypass from relay package

## Crypto Types (internal/crypto/)

### HouseholdKey [KeySize]byte
Symmetric key for AEAD. String() returns "[REDACTED]" to prevent leaks.
- GenerateHouseholdKey(), SaveHouseholdKey, LoadHouseholdKey

### DeviceKeyPair
Curve25519 public/private pair for box encryption (key exchange).
- GenerateDeviceKeyPair(), SaveDeviceKeyPair, LoadDeviceKeyPair

### Encrypt / Decrypt (encrypt.go)
NaCl secretbox with random 24-byte nonces.

### BoxSeal / BoxOpen (box.go)
NaCl box (authenticated public-key) for key exchange.

### SecretsDir(), SaveDeviceToken / LoadDeviceToken (token.go)
Bearer token persistence with restrictive file perms.

## MCP Types (internal/mcp/)

### Server (server.go)
- Server struct wrapping *data.Store
- Serve(ctx, stdin, stdout): stdio JSON-RPC loop, MCP protocol

### Tools (tools.go)
- query: arbitrary SELECT against the local DB (ReadOnlyQuery enforced)
- get_schema: table list + columns
- search_documents: filter by entity, date range, MIME, full-text
- get_maintenance_schedule: due/overdue items
- get_house_profile: structured house metadata
