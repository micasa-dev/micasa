// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
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
	LLMConfig        *llmConfig // nil if LLM is not configured
	ExtractionConfig extractionConfig
}

// llmConfig holds resolved LLM settings passed from main after loading the
// TOML config. Kept as a separate type so the app package doesn't import
// config directly.
type llmConfig struct {
	BaseURL      string
	Model        string
	ExtraContext string
	Timeout      time.Duration
}

// extractionConfig holds resolved extraction pipeline settings.
type extractionConfig struct {
	Model       string // overrides LLM model; empty = use chat model
	MaxOCRPages int
	Enabled     bool // LLM extraction enabled
}

// SetExtraction configures the extraction pipeline on the Options.
func (o *Options) SetExtraction(model string, maxOCRPages int, enabled bool) {
	o.ExtractionConfig = extractionConfig{
		Model:       model,
		MaxOCRPages: maxOCRPages,
		Enabled:     enabled,
	}
}

// SetLLM configures the LLM backend on the Options. Pass empty strings to
// disable the LLM feature.
func (o *Options) SetLLM(baseURL, model, extraContext string, timeout time.Duration) {
	if baseURL == "" || model == "" {
		o.LLMConfig = nil
		return
	}
	o.LLMConfig = &llmConfig{
		BaseURL:      baseURL,
		Model:        model,
		ExtraContext: extraContext,
		Timeout:      timeout,
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
	FormKind FormKind
	FormData any
	FieldPtr *string            // pointer into FormData
	Validate func(string) error // nil = no validation
}

// editorState holds context for an in-flight $EDITOR session so we can
// restore the textarea with the edited content when the editor exits.
type editorState struct {
	EditID   uint
	FormKind FormKind
	FormData any
	FieldPtr *string // pointer into FormData for the notes field
	TempFile string  // path to the temp file passed to the editor
}

// editorFinishedMsg is sent when an external $EDITOR process exits.
type editorFinishedMsg struct{ Err error }
