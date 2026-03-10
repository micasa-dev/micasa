// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/cpcloud/micasa/internal/llm"
)

type Mode int

const (
	modeNormal Mode = iota
	modeEdit
	modeForm
)

type FormKind int

const (
	formNone FormKind = iota
	formHouse
	formProject
	formQuote
	formMaintenance
	formAppliance
	formIncident
	formServiceLog
	formVendor
	formDocument
)

// confirmKind represents mutually exclusive confirmation dialog states.
// Only one confirmation can be active at a time, making illegal states
// unrepresentable.
type confirmKind int

const (
	confirmNone            confirmKind = iota
	confirmHardDelete                  // permanent incident deletion (y/n)
	confirmFormDiscard                 // discard dirty form changes, stay in app
	confirmFormQuitDiscard             // discard dirty form changes and quit
)

// isFormConfirm reports whether the confirmation is a form-related dialog.
func (k confirmKind) isFormConfirm() bool {
	return k == confirmFormDiscard || k == confirmFormQuitDiscard
}

// formData is implemented by every form data struct, binding it to a specific
// FormKind at compile time. This replaces the old `any` field and eliminates
// the risk of formKind/formData type mismatches.
type formData interface {
	formKind() FormKind
}

type formState struct {
	form            *huh.Form
	formData        formData
	formSnapshot    formData
	formDirty       bool
	formHasRequired bool
	pendingFormInit tea.Cmd
	editID          *uint
	notesEditMode   bool
	notesFieldPtr   *string
	pendingEditor   *editorState
}

// formKind returns the FormKind of the current form data, or formNone when no
// form is active. Derived from formData so mismatch is impossible.
func (fs *formState) formKind() FormKind {
	if fs.formData == nil {
		return formNone
	}
	return fs.formData.formKind()
}

type extractState struct {
	// Extraction-specific LLM connection settings. When extractionProvider
	// differs from the chat provider, an independent client is created.
	extractionProvider  string
	extractionBaseURL   string
	extractionModel     string
	extractionAPIKey    string
	extractionTimeout   time.Duration
	extractionThinking  string
	extractionEnabled   bool
	ocrTSV              bool
	ocrConfThreshold    int
	extractionClient    *llm.Client
	extractors          []extract.Extractor
	extractionReady     bool
	llmInferenceTimeout time.Duration

	pendingExtractionDocID *uint
	extraction             *extractionLogState
	bgExtractions          []*extractionLogState
}

type TabKind int

const (
	tabProjects TabKind = iota
	tabQuotes
	tabMaintenance
	tabIncidents
	tabAppliances
	tabVendors
	tabDocuments
)

func (k TabKind) String() string {
	switch k {
	case tabProjects:
		return "Projects"
	case tabQuotes:
		return "Quotes"
	case tabMaintenance:
		return "Maintenance"
	case tabIncidents:
		return "Incidents"
	case tabAppliances:
		return "Appliances"
	case tabVendors:
		return "Vendors"
	case tabDocuments:
		return "Docs"
	}
	panic(fmt.Sprintf("unhandled TabKind: %d", k))
}

// singular returns the lowercase singular noun for a tab kind, used in
// context-aware empty-state messages for detail drilldowns.
func (k TabKind) singular() string {
	switch k {
	case tabProjects:
		return "project"
	case tabQuotes:
		return "quote"
	case tabMaintenance:
		return "maintenance item"
	case tabIncidents:
		return "incident"
	case tabAppliances:
		return "appliance"
	case tabVendors:
		return "vendor"
	case tabDocuments:
		return "doc"
	}
	panic(fmt.Sprintf("unhandled TabKind: %d", k))
}

// plural returns the lowercase plural noun for a tab kind.
func (k TabKind) plural() string {
	switch k {
	case tabProjects:
		return "projects"
	case tabQuotes:
		return "quotes"
	case tabMaintenance:
		return "maintenance items"
	case tabIncidents:
		return "incidents"
	case tabAppliances:
		return "appliances"
	case tabVendors:
		return "vendors"
	case tabDocuments:
		return "documents"
	}
	panic(fmt.Sprintf("unhandled TabKind: %d", k))
}

type rowMeta struct {
	ID      uint
	Deleted bool
	Dimmed  bool // true in pin preview mode for non-matching rows
}

type sortDir int

const (
	sortAsc sortDir = iota
	sortDesc
)

type sortEntry struct {
	Col int
	Dir sortDir
}

// filterPin holds the set of pinned values for a single column.
// Multiple values in the same column use OR (IN) semantics.
type filterPin struct {
	Col    int             // index in tab.Specs
	Values map[string]bool // lowercased pinned values
}

type Tab struct {
	Kind        TabKind
	Name        string
	Handler     TabHandler
	Table       table.Model
	Rows        []rowMeta
	Specs       []columnSpec
	CellRows    [][]cell
	ColCursor   int
	ViewOffset  int // first visible column in horizontal scroll viewport
	LastDeleted *uint
	ShowDeleted bool
	Sorts       []sortEntry
	Stale       bool // true when data may be outdated; cleared on reload

	// Pin-and-filter state.
	Pins           []filterPin // active pins; AND across columns, OR within
	FilterActive   bool        // true = non-matching rows hidden; false = preview only
	FilterInverted bool        // true = show rows that DON'T match instead of rows that do

	// Full data (pre-row-filter). Populated by reloadTab after project status
	// filtering. Row filter operates on these without hitting the DB.
	FullRows     []table.Row
	FullMeta     []rowMeta
	FullCellRows [][]cell

	// cachedVP holds the last computed tableViewport, populated during View()
	// and reused by mouse click handlers to avoid O(rows*cols) recomputation.
	// Nil when stale; call Model.tabViewport to get-or-compute.
	cachedVP *tableViewport
}

type statusKind int

const (
	statusInfo statusKind = iota
	statusError
)

type statusMsg struct {
	Text string
	Kind statusKind
}

// detailContext holds state for a drill-down sub-table (e.g. service log for
// a maintenance item). When non-nil on the Model, the detail tab replaces the
// main tab for all interaction.
type detailContext struct {
	ParentTabIndex int
	ParentRowID    uint
	Breadcrumb     string
	Tab            Tab
}

type Options struct {
	DBPath           string
	ConfigPath       string
	FilePickerDir    string     // starting directory for document file picker
	LLMConfig        *llmConfig // nil if LLM is not configured
	ExtractionConfig extractionConfig
}

// llmConfig holds resolved LLM settings passed from main after loading the
// TOML config. Kept as a separate type so the app package doesn't import
// config directly.
type llmConfig struct {
	Provider     string
	BaseURL      string
	Model        string
	APIKey       string //nolint:gosec // G101 false positive: field name triggers heuristic, not a hardcoded credential
	ExtraContext string
	Timeout      time.Duration
	Thinking     string // reasoning effort: none|low|medium|high|auto
}

// extractionConfig holds resolved extraction pipeline settings.
type extractionConfig struct {
	// LLM connection settings for extraction. When Provider is non-empty,
	// the extraction pipeline creates its own LLM client independent of
	// the chat client. When empty, falls back to the chat client.
	Provider            string
	BaseURL             string
	Model               string
	APIKey              string //nolint:gosec // G117 false positive: field name, not a hardcoded credential
	Timeout             time.Duration
	Thinking            string // reasoning effort level
	LLMInferenceTimeout time.Duration

	Extractors       []extract.Extractor // configured extractors; nil = defaults
	Enabled          bool                // LLM extraction enabled
	OCRTSV           bool                // send spatial layout annotations to LLM
	OCRConfThreshold int                 // confidence threshold for spatial annotations
}

// SetExtraction configures the extraction pipeline on the Options.
func (o *Options) SetExtraction(
	provider, baseURL, model, apiKey string,
	timeout time.Duration,
	thinking string,
	extractors []extract.Extractor,
	enabled bool,
	llmInferenceTimeout time.Duration,
	ocrTSV bool,
	ocrConfThreshold int,
) {
	o.ExtractionConfig = extractionConfig{
		Provider:            provider,
		BaseURL:             baseURL,
		Model:               model,
		APIKey:              apiKey,
		Timeout:             timeout,
		Thinking:            thinking,
		LLMInferenceTimeout: llmInferenceTimeout,
		Extractors:          extractors,
		Enabled:             enabled,
		OCRTSV:              ocrTSV,
		OCRConfThreshold:    ocrConfThreshold,
	}
}

// SetLLM configures the LLM backend on the Options. Pass empty strings to
// disable the LLM feature.
func (o *Options) SetLLM(
	provider, baseURL, model, apiKey, extraContext string,
	timeout time.Duration,
	thinking string,
) {
	if model == "" {
		o.LLMConfig = nil
		return
	}
	o.LLMConfig = &llmConfig{
		Provider:     provider,
		BaseURL:      baseURL,
		Model:        model,
		APIKey:       apiKey,
		ExtraContext: extraContext,
		Timeout:      timeout,
		Thinking:     thinking,
	}
}

type alignKind int

const (
	alignLeft alignKind = iota
	alignRight
)

type cellKind int

const (
	cellText cellKind = iota
	cellMoney
	cellReadonly
	cellDate
	cellStatus
	cellDrilldown // interactive count that opens a detail view
	cellWarranty  // date with green/red coloring based on expiry
	cellUrgency   // date colored by proximity (green -> yellow -> red)
	cellNotes     // text that can be expanded in a read-only overlay
	cellEntity    // entity ref with colored kind-letter prefix
)

type cell struct {
	Value  string
	Kind   cellKind
	Null   bool // true when the database value is NULL (not just empty)
	LinkID uint // FK target ID for cross-tab navigation; 0 = no link
}

// nullPinKey is the internal key used by the pin/filter system to represent
// NULL cells. It cannot collide with any real display value.
const nullPinKey = "\x00null"

// columnLink describes a foreign-key relationship to another tab.
type columnLink struct {
	TargetTab TabKind
}

type columnSpec struct {
	Title       string
	Min         int
	Max         int
	Flex        bool
	Align       alignKind
	Kind        cellKind
	Link        *columnLink // non-nil if this column references another tab
	FixedValues []string    // all possible values; used to stabilize column width
	HideOrder   int         // 0 = visible; >0 = hidden (higher = more recently hidden)
}

// inlineInputState holds state for a single-field text edit rendered in the
// status bar, keeping the table visible. Used instead of a full form overlay
// for simple text/number fields.
type inlineInputState struct {
	Input    textinput.Model
	Title    string
	EditID   uint
	FormData formData
	FieldPtr *string            // pointer into FormData
	Validate func(string) error // nil = no validation
}

// editorState holds context for an in-flight $EDITOR session so we can
// restore the textarea with the edited content when the editor exits.
type editorState struct {
	EditID   uint
	FormData formData
	FieldPtr *string // pointer into FormData for the notes field
	TempFile string  // path to the temp file passed to the editor
}

// editorFinishedMsg is sent when an external $EDITOR process exits.
type editorFinishedMsg struct{ Err error }
