// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	Header           lipgloss.Style
	HeaderBox        lipgloss.Style
	HeaderTitle      lipgloss.Style
	HeaderHint       lipgloss.Style
	HeaderBadge      lipgloss.Style
	HeaderSection    lipgloss.Style
	HeaderLabel      lipgloss.Style
	HeaderValue      lipgloss.Style
	Keycap           lipgloss.Style
	TabActive        lipgloss.Style
	TabInactive      lipgloss.Style
	TabLocked        lipgloss.Style
	TabUnderline     lipgloss.Style
	TableHeader      lipgloss.Style
	TableSelected    lipgloss.Style
	TableSeparator   lipgloss.Style
	ColActiveHeader  lipgloss.Style
	FormClean        lipgloss.Style
	FormDirty        lipgloss.Style
	ModeNormal       lipgloss.Style
	ModeEdit         lipgloss.Style
	Money            lipgloss.Style
	Readonly         lipgloss.Style
	Drilldown        lipgloss.Style
	Empty            lipgloss.Style
	Null             lipgloss.Style
	Error            lipgloss.Style
	Info             lipgloss.Style
	DeletedLabel     lipgloss.Style
	Pinned           lipgloss.Style
	LinkIndicator    lipgloss.Style
	Breadcrumb       lipgloss.Style
	BreadcrumbArrow  lipgloss.Style
	FilterMark       lipgloss.Style // dot between tabs when filter is active
	HiddenLeft       lipgloss.Style // hidden cols to the left of cursor
	HiddenRight      lipgloss.Style // hidden cols to the right of cursor
	DashSubtitle     lipgloss.Style // dashboard subtitle (house name, date)
	DashSection      lipgloss.Style // dashboard section header
	DashSectionWarn  lipgloss.Style // dashboard section header (overdue/warning)
	DashSectionAlert lipgloss.Style // dashboard section header (incidents/attention)
	DashRule         lipgloss.Style // dashboard horizontal rule
	DashLabel        lipgloss.Style // dashboard dim label text
	DashValue        lipgloss.Style // dashboard bright value text
	DashHouseValue   lipgloss.Style // dashboard house-level data (italic)
	DashOverdue      lipgloss.Style // overdue maintenance item
	DashUpcoming     lipgloss.Style // upcoming maintenance (within 30 days)
	CalCursor        lipgloss.Style // calendar: cursor day
	CalSelected      lipgloss.Style // calendar: previously selected day
	CalToday         lipgloss.Style // calendar: today marker
	ChatUser         lipgloss.Style // chat: user message label
	ChatAssistant    lipgloss.Style // chat: assistant message label
	ChatNotice       lipgloss.Style // chat: system notice (model switch, pull progress)
	ChatInterrupted  lipgloss.Style // chat: user-initiated cancellation
	ExtPending       lipgloss.Style // extraction step: pending name
	ExtRunning       lipgloss.Style // extraction step: running name
	ExtDone          lipgloss.Style // extraction step: done name
	ExtFailed        lipgloss.Style // extraction step: failed name
	ExtOk            lipgloss.Style // extraction step: ok icon
	ExtFail          lipgloss.Style // extraction step: fail icon
	ExtCursor        lipgloss.Style // extraction step: cursor arrow
	ExtRerun         lipgloss.Style // extraction step: rerun hint
	ModelActive      lipgloss.Style // model completer: active base
	ModelActiveHL    lipgloss.Style // model completer: active highlight
	ModelLocal       lipgloss.Style // model completer: local base
	ModelLocalHL     lipgloss.Style // model completer: local highlight
	ModelRemote      lipgloss.Style // model completer: remote base
	ModelRemoteHL    lipgloss.Style // model completer: remote highlight
	SortArrow        lipgloss.Style // table: sort direction arrow
	CellDim          lipgloss.Style // table: dim fallback cell
	DeletedCell      lipgloss.Style // table: deleted row cell
	CalHeader        lipgloss.Style // calendar: month/year header
	CalDayLabel      lipgloss.Style // calendar: day-of-week labels
	CalHintKey       lipgloss.Style // calendar: key legend keys
	HouseRoof        lipgloss.Style // house art: roof
	HouseWall        lipgloss.Style // house art: walls
	HouseWindow      lipgloss.Style // house art: windows
	HouseDoor        lipgloss.Style // house art: door
	StatusStyles     map[string]lipgloss.Style
	// Reusable utility styles to avoid per-frame lipgloss.NewStyle() allocations.
	OverlayBox       lipgloss.Style          // overlay: rounded border + accent + padding
	TextDim          lipgloss.Style          // dim text (textDim foreground)
	AccentBold       lipgloss.Style          // accent foreground + bold
	AccentText       lipgloss.Style          // accent foreground only
	SecondaryText    lipgloss.Style          // secondary foreground only
	Rule             lipgloss.Style          // border-colored separator
	Base             lipgloss.Style          // unstyled base (avoids per-frame NewStyle)
	UrgencyOverdue   lipgloss.Style          // cell: overdue urgency
	UrgencySoon      lipgloss.Style          // cell: soon urgency
	UrgencyUpcoming  lipgloss.Style          // cell: upcoming urgency
	UrgencyFar       lipgloss.Style          // cell: far-future urgency
	WarrantyExpired  lipgloss.Style          // cell: expired warranty
	WarrantyActive   lipgloss.Style          // cell: active warranty
	EntityKindStyles map[byte]lipgloss.Style // cell: entity kind letter colors
	DimPath          lipgloss.Style          // file picker: dimmed path text
}

// Colorblind-safe palette (Wong) with adaptive light/dark variants.
//
// Each color uses lipgloss.AdaptiveColor{Light, Dark} so the UI looks
// correct on both dark and light terminal backgrounds. The Light values
// are darkened/saturated versions of the Dark values to maintain contrast
// on white backgrounds.
//
// Chromatic roles:
//   Primary accent:   sky blue     Dark #56B4E9  Light #0072B2
//   Secondary accent: orange       Dark #E69F00  Light #D55E00
//   Success/positive: bluish green Dark #009E73  Light #007A5A
//   Warning:          yellow       Dark #F0E442  Light #B8860B
//   Error/danger:     vermillion   Dark #D55E00  Light #CC3311
//   Muted accent:     rose         Dark #CC79A7  Light #AA4499
//
// Neutral roles:
//   Text bright:      Dark #E5E7EB  Light #1F2937
//   Text mid:         Dark #9CA3AF  Light #4B5563
//   Text dim:         Dark #6B7280  Light #4B5563
//   Surface:          Dark #1F2937  Light #F3F4F6
//   Surface deep:     Dark #111827  Light #E5E7EB
//   On-accent text:   Dark #0F172A  Light #FFFFFF

var (
	accent    = lipgloss.AdaptiveColor{Light: "#0072B2", Dark: "#56B4E9"}
	secondary = lipgloss.AdaptiveColor{Light: "#D55E00", Dark: "#E69F00"}
	success   = lipgloss.AdaptiveColor{Light: "#007A5A", Dark: "#009E73"}
	warning   = lipgloss.AdaptiveColor{Light: "#B8860B", Dark: "#F0E442"}
	danger    = lipgloss.AdaptiveColor{Light: "#CC3311", Dark: "#D55E00"}
	muted     = lipgloss.AdaptiveColor{Light: "#AA4499", Dark: "#CC79A7"}

	textBright = lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#E5E7EB"}
	textMid    = lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#9CA3AF"}
	textDim    = lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#6B7280"}
	surface    = lipgloss.AdaptiveColor{Light: "#F3F4F6", Dark: "#1F2937"}
	onAccent   = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0F172A"}
	border     = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#374151"}
)

// appStyles is the package-level singleton. Created once at init; never
// mutated. All rendering code reads from this pointer instead of copying
// the 86-field struct through function parameters.
var appStyles = DefaultStyles()

func DefaultStyles() *Styles {
	return &Styles{
		Header: lipgloss.NewStyle().Bold(true),
		HeaderBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),
		HeaderTitle: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(accent).
			Padding(0, 1).
			Bold(true),
		HeaderHint: lipgloss.NewStyle().
			Foreground(textMid),
		HeaderBadge: lipgloss.NewStyle().
			Foreground(textBright).
			Background(surface).
			Padding(0, 1),
		HeaderSection: lipgloss.NewStyle().
			Foreground(textBright).
			Background(border).
			Padding(0, 1).
			Bold(true),
		HeaderLabel: lipgloss.NewStyle().
			Foreground(textDim),
		HeaderValue: lipgloss.NewStyle().
			Foreground(secondary),
		Keycap: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(textBright).
			Padding(0, 1).
			Bold(true),
		TabActive: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(accent).
			Padding(0, 1).
			Bold(true),
		TabInactive: lipgloss.NewStyle().
			Foreground(textMid).
			Padding(0, 1),
		TabLocked: lipgloss.NewStyle().
			Foreground(textDim).
			Padding(0, 1).
			Strikethrough(true),
		TabUnderline: lipgloss.NewStyle().
			Foreground(accent),
		TableHeader: lipgloss.NewStyle().
			Foreground(textDim).
			Bold(true),
		TableSelected: lipgloss.NewStyle().
			Background(surface).
			Bold(true),
		TableSeparator: lipgloss.NewStyle().
			Foreground(border),
		ColActiveHeader: lipgloss.NewStyle().
			Foreground(secondary).
			Bold(true),
		FormClean: lipgloss.NewStyle().
			Foreground(success),
		FormDirty: lipgloss.NewStyle().
			Foreground(secondary).
			Bold(true),
		ModeNormal: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(accent).
			Padding(0, 1).
			Bold(true),
		ModeEdit: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(secondary).
			Padding(0, 1).
			Bold(true),
		Money: lipgloss.NewStyle().
			Foreground(success),
		Readonly: lipgloss.NewStyle().
			Foreground(textDim),
		Drilldown: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(accent).
			Bold(true).
			Padding(0, 1),
		Empty: lipgloss.NewStyle().
			Foreground(textDim),
		Null: lipgloss.NewStyle().
			Foreground(textDim).
			Italic(true),
		Error: lipgloss.NewStyle().
			Foreground(danger).
			Bold(true),
		Info: lipgloss.NewStyle().
			Foreground(success).
			Bold(true),
		DeletedLabel: lipgloss.NewStyle().
			Foreground(danger),
		Pinned: lipgloss.NewStyle().
			Foreground(muted),
		LinkIndicator: lipgloss.NewStyle().
			Foreground(muted),
		Breadcrumb: lipgloss.NewStyle().
			Foreground(textBright).
			Bold(true),
		BreadcrumbArrow: lipgloss.NewStyle().
			Foreground(accent),
		FilterMark: lipgloss.NewStyle().
			Foreground(muted),
		HiddenLeft: lipgloss.NewStyle().
			Foreground(secondary).
			Italic(true),
		HiddenRight: lipgloss.NewStyle().
			Foreground(accent).
			Italic(true),
		DashSubtitle: lipgloss.NewStyle().
			Foreground(textDim).
			Italic(true),
		DashSection: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(accent).
			Padding(0, 1).
			Bold(true),
		DashSectionWarn: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(danger).
			Padding(0, 1).
			Bold(true),
		DashSectionAlert: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(warning).
			Padding(0, 1).
			Bold(true),
		DashRule: lipgloss.NewStyle().
			Foreground(border),
		DashLabel: lipgloss.NewStyle().
			Foreground(textDim),
		DashValue: lipgloss.NewStyle().
			Foreground(textBright),
		DashHouseValue: lipgloss.NewStyle().
			Foreground(textDim).
			Italic(true),
		DashOverdue: lipgloss.NewStyle().
			Foreground(danger).
			Bold(true),
		DashUpcoming: lipgloss.NewStyle().
			Foreground(warning),
		CalCursor: lipgloss.NewStyle().
			Background(accent).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#000000"}).
			Bold(true),
		CalSelected: lipgloss.NewStyle().
			Foreground(secondary).
			Underline(true),
		CalToday: lipgloss.NewStyle().
			Foreground(success).
			Bold(true),
		ChatUser: lipgloss.NewStyle().
			Foreground(secondary),
		ChatAssistant: lipgloss.NewStyle().
			Foreground(accent),
		ChatNotice: lipgloss.NewStyle().
			Foreground(success).
			Italic(true),
		ChatInterrupted: lipgloss.NewStyle().
			Foreground(secondary).
			Italic(true),
		ExtPending: lipgloss.NewStyle().
			Foreground(textDim),
		ExtRunning: lipgloss.NewStyle().
			Foreground(accent),
		ExtDone: lipgloss.NewStyle().
			Foreground(textBright),
		ExtFailed: lipgloss.NewStyle().
			Foreground(danger),
		ExtOk: lipgloss.NewStyle().
			Foreground(success),
		ExtFail: lipgloss.NewStyle().
			Foreground(danger),
		ExtCursor: lipgloss.NewStyle().
			Foreground(accent),
		ExtRerun: lipgloss.NewStyle().
			Foreground(textDim),
		ModelActive: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),
		ModelActiveHL: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			Underline(true),
		ModelLocal: lipgloss.NewStyle().
			Foreground(textBright),
		ModelLocalHL: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),
		ModelRemote: lipgloss.NewStyle().
			Foreground(textDim).
			Italic(true),
		ModelRemoteHL: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			Italic(true),
		SortArrow: lipgloss.NewStyle().
			Foreground(secondary),
		CellDim: lipgloss.NewStyle().
			Foreground(textDim),
		DeletedCell: lipgloss.NewStyle().
			Foreground(textDim).
			Strikethrough(true).
			Italic(true),
		CalHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(accent),
		CalDayLabel: lipgloss.NewStyle().
			Foreground(textDim),
		CalHintKey: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),
		HouseRoof: lipgloss.NewStyle().
			Foreground(accent),
		HouseWall: lipgloss.NewStyle().
			Foreground(textMid),
		HouseWindow: lipgloss.NewStyle().
			Foreground(warning),
		HouseDoor: lipgloss.NewStyle().
			Foreground(secondary),
		OverlayBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2),
		TextDim:         lipgloss.NewStyle().Foreground(textDim),
		AccentBold:      lipgloss.NewStyle().Foreground(accent).Bold(true),
		AccentText:      lipgloss.NewStyle().Foreground(accent),
		SecondaryText:   lipgloss.NewStyle().Foreground(secondary),
		Rule:            lipgloss.NewStyle().Foreground(border),
		Base:            lipgloss.NewStyle(),
		UrgencyOverdue:  lipgloss.NewStyle().Foreground(danger).Bold(true),
		UrgencySoon:     lipgloss.NewStyle().Foreground(secondary),
		UrgencyUpcoming: lipgloss.NewStyle().Foreground(warning),
		UrgencyFar:      lipgloss.NewStyle().Foreground(success),
		WarrantyExpired: lipgloss.NewStyle().Foreground(danger),
		WarrantyActive:  lipgloss.NewStyle().Foreground(success),
		EntityKindStyles: map[byte]lipgloss.Style{
			'A': lipgloss.NewStyle().Foreground(muted),
			'I': lipgloss.NewStyle().Foreground(danger),
			'M': lipgloss.NewStyle().Foreground(secondary),
			'P': lipgloss.NewStyle().Foreground(accent),
			'Q': lipgloss.NewStyle().Foreground(success),
			'V': lipgloss.NewStyle().Foreground(warning),
		},
		DimPath: lipgloss.NewStyle().Foreground(textDim),
		StatusStyles: map[string]lipgloss.Style{
			// Project statuses.
			"ideating":  lipgloss.NewStyle().Foreground(muted),
			"planned":   lipgloss.NewStyle().Foreground(accent),
			"quoted":    lipgloss.NewStyle().Foreground(secondary),
			"underway":  lipgloss.NewStyle().Foreground(success),
			"delayed":   lipgloss.NewStyle().Foreground(warning),
			"completed": lipgloss.NewStyle().Foreground(textDim),
			"abandoned": lipgloss.NewStyle().Foreground(danger),
			// Incident statuses.
			"open":        lipgloss.NewStyle().Foreground(accent),
			"in_progress": lipgloss.NewStyle().Foreground(success),
			"resolved":    lipgloss.NewStyle().Foreground(textDim),
			// Incident severities.
			"urgent":   lipgloss.NewStyle().Foreground(danger),
			"soon":     lipgloss.NewStyle().Foreground(warning),
			"whenever": lipgloss.NewStyle().Foreground(textDim),
		},
	}
}
