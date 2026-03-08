// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import "github.com/charmbracelet/lipgloss"

// Styles holds the application's pre-built lipgloss styles. Fields are private;
// use the public accessor methods to read them. Duplicate style definitions
// share a single backing field, cutting storage from 93 fields to 39.
type Styles struct {
	fgTextDim    lipgloss.Style
	fgTextMid    lipgloss.Style
	fgTextBright lipgloss.Style
	fgAccent     lipgloss.Style
	fgSecondary  lipgloss.Style
	fgSuccess    lipgloss.Style
	fgWarning    lipgloss.Style
	fgDanger     lipgloss.Style
	fgMuted      lipgloss.Style
	fgBorder     lipgloss.Style

	fgTextDimBold     lipgloss.Style
	fgTextDimItalic   lipgloss.Style
	fgAccentBold      lipgloss.Style
	fgAccentItalic    lipgloss.Style
	fgSecondaryBold   lipgloss.Style
	fgSecondaryItalic lipgloss.Style
	fgSuccessBold     lipgloss.Style
	fgSuccessItalic   lipgloss.Style
	fgDangerBold      lipgloss.Style

	bold             lipgloss.Style
	base             lipgloss.Style
	accentPill       lipgloss.Style
	headerBox        lipgloss.Style
	headerBadge      lipgloss.Style
	headerSection    lipgloss.Style
	keycap           lipgloss.Style
	tabInactive      lipgloss.Style
	tabLocked        lipgloss.Style
	tableSelected    lipgloss.Style
	modeEdit         lipgloss.Style
	dashSectionWarn  lipgloss.Style
	dashSectionAlert lipgloss.Style
	calCursor        lipgloss.Style
	calSelected      lipgloss.Style
	modelActiveHL    lipgloss.Style
	modelRemoteHL    lipgloss.Style
	deletedCell      lipgloss.Style
	overlayBox       lipgloss.Style
	breadcrumb       lipgloss.Style
	blinkCursor      lipgloss.Style
}

// Colorblind-safe palette (Wong) with adaptive light/dark variants.
//
// Each color uses lipgloss.AdaptiveColor{Light, Dark} so the UI looks
// correct on both dark and light terminal backgrounds. The Light values
// are darkened/saturated versions of the Dark values to maintain contrast
// on white backgrounds.
//
// Chromatic roles:
//
//	Primary accent:   sky blue     Dark #56B4E9  Light #0072B2
//	Secondary accent: orange       Dark #E69F00  Light #D55E00
//	Success/positive: bluish green Dark #009E73  Light #007A5A
//	Warning:          yellow       Dark #F0E442  Light #B8860B
//	Error/danger:     vermillion   Dark #D55E00  Light #CC3311
//	Muted accent:     rose         Dark #CC79A7  Light #AA4499
//
// Neutral roles:
//
//	Text bright:      Dark #E5E7EB  Light #1F2937
//	Text mid:         Dark #9CA3AF  Light #4B5563
//	Text dim:         Dark #6B7280  Light #4B5563
//	Surface:          Dark #1F2937  Light #F3F4F6
//	Surface deep:     Dark #111827  Light #E5E7EB
//	On-accent text:   Dark #0F172A  Light #FFFFFF
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
// the struct through function parameters.
var appStyles = DefaultStyles()

func DefaultStyles() *Styles {
	return &Styles{
		fgTextDim:    lipgloss.NewStyle().Foreground(textDim),
		fgTextMid:    lipgloss.NewStyle().Foreground(textMid),
		fgTextBright: lipgloss.NewStyle().Foreground(textBright),
		fgAccent:     lipgloss.NewStyle().Foreground(accent),
		fgSecondary:  lipgloss.NewStyle().Foreground(secondary),
		fgSuccess:    lipgloss.NewStyle().Foreground(success),
		fgWarning:    lipgloss.NewStyle().Foreground(warning),
		fgDanger:     lipgloss.NewStyle().Foreground(danger),
		fgMuted:      lipgloss.NewStyle().Foreground(muted),
		fgBorder:     lipgloss.NewStyle().Foreground(border),

		fgTextDimBold:     lipgloss.NewStyle().Foreground(textDim).Bold(true),
		fgTextDimItalic:   lipgloss.NewStyle().Foreground(textDim).Italic(true),
		fgAccentBold:      lipgloss.NewStyle().Foreground(accent).Bold(true),
		fgAccentItalic:    lipgloss.NewStyle().Foreground(accent).Italic(true),
		fgSecondaryBold:   lipgloss.NewStyle().Foreground(secondary).Bold(true),
		fgSecondaryItalic: lipgloss.NewStyle().Foreground(secondary).Italic(true),
		fgSuccessBold:     lipgloss.NewStyle().Foreground(success).Bold(true),
		fgSuccessItalic:   lipgloss.NewStyle().Foreground(success).Italic(true),
		fgDangerBold:      lipgloss.NewStyle().Foreground(danger).Bold(true),

		bold: lipgloss.NewStyle().Bold(true),
		base: lipgloss.NewStyle(),
		accentPill: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(accent).
			Padding(0, 1).
			Bold(true),
		headerBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),
		headerBadge: lipgloss.NewStyle().
			Foreground(textBright).
			Background(surface).
			Padding(0, 1),
		headerSection: lipgloss.NewStyle().
			Foreground(textBright).
			Background(border).
			Padding(0, 1).
			Bold(true),
		keycap: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(textBright).
			Padding(0, 1).
			Bold(true),
		tabInactive: lipgloss.NewStyle().
			Foreground(textMid).
			Padding(0, 1),
		tabLocked: lipgloss.NewStyle().
			Foreground(textDim).
			Padding(0, 1).
			Strikethrough(true),
		tableSelected: lipgloss.NewStyle().
			Background(surface).
			Bold(true),
		modeEdit: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(secondary).
			Padding(0, 1).
			Bold(true),
		dashSectionWarn: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(danger).
			Padding(0, 1).
			Bold(true),
		dashSectionAlert: lipgloss.NewStyle().
			Foreground(onAccent).
			Background(warning).
			Padding(0, 1).
			Bold(true),
		calCursor: lipgloss.NewStyle().
			Background(accent).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#000000"}).
			Bold(true),
		calSelected: lipgloss.NewStyle().
			Foreground(secondary).
			Underline(true),
		modelActiveHL: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			Underline(true),
		modelRemoteHL: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			Italic(true),
		deletedCell: lipgloss.NewStyle().
			Foreground(textDim).
			Strikethrough(true).
			Italic(true),
		overlayBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2),
		breadcrumb: lipgloss.NewStyle().
			Foreground(textBright).
			Bold(true),
		blinkCursor: lipgloss.NewStyle().
			Foreground(accent).
			Blink(true),
	}
}

// --- Foreground(textDim) ---

func (s *Styles) HeaderLabel() lipgloss.Style { return s.fgTextDim }
func (s *Styles) Readonly() lipgloss.Style    { return s.fgTextDim }
func (s *Styles) Empty() lipgloss.Style       { return s.fgTextDim }
func (s *Styles) DashLabel() lipgloss.Style   { return s.fgTextDim }
func (s *Styles) ExtPending() lipgloss.Style  { return s.fgTextDim }
func (s *Styles) ExtRerun() lipgloss.Style    { return s.fgTextDim }
func (s *Styles) CellDim() lipgloss.Style     { return s.fgTextDim }
func (s *Styles) CalDayLabel() lipgloss.Style { return s.fgTextDim }
func (s *Styles) TextDim() lipgloss.Style     { return s.fgTextDim }
func (s *Styles) DimPath() lipgloss.Style     { return s.fgTextDim }

// --- Foreground(textMid) ---

func (s *Styles) HeaderHint() lipgloss.Style { return s.fgTextMid }
func (s *Styles) HouseWall() lipgloss.Style  { return s.fgTextMid }

// --- Foreground(textBright) ---

func (s *Styles) DashValue() lipgloss.Style  { return s.fgTextBright }
func (s *Styles) ExtDone() lipgloss.Style    { return s.fgTextBright }
func (s *Styles) ModelLocal() lipgloss.Style { return s.fgTextBright }

// --- Foreground(accent) ---

func (s *Styles) TabUnderline() lipgloss.Style    { return s.fgAccent }
func (s *Styles) BreadcrumbArrow() lipgloss.Style { return s.fgAccent }
func (s *Styles) ChatAssistant() lipgloss.Style   { return s.fgAccent }
func (s *Styles) ExtRunning() lipgloss.Style      { return s.fgAccent }
func (s *Styles) ExtCursor() lipgloss.Style       { return s.fgAccent }
func (s *Styles) HouseRoof() lipgloss.Style       { return s.fgAccent }
func (s *Styles) AccentText() lipgloss.Style      { return s.fgAccent }

// --- Foreground(secondary) ---

func (s *Styles) HeaderValue() lipgloss.Style   { return s.fgSecondary }
func (s *Styles) ChatUser() lipgloss.Style      { return s.fgSecondary }
func (s *Styles) SortArrow() lipgloss.Style     { return s.fgSecondary }
func (s *Styles) SecondaryText() lipgloss.Style { return s.fgSecondary }
func (s *Styles) UrgencySoon() lipgloss.Style   { return s.fgSecondary }
func (s *Styles) HouseDoor() lipgloss.Style     { return s.fgSecondary }

// --- Foreground(success) ---

func (s *Styles) FormClean() lipgloss.Style      { return s.fgSuccess }
func (s *Styles) Money() lipgloss.Style          { return s.fgSuccess }
func (s *Styles) ExtOk() lipgloss.Style          { return s.fgSuccess }
func (s *Styles) UrgencyFar() lipgloss.Style     { return s.fgSuccess }
func (s *Styles) WarrantyActive() lipgloss.Style { return s.fgSuccess }

// --- Foreground(warning) ---

func (s *Styles) DashUpcoming() lipgloss.Style    { return s.fgWarning }
func (s *Styles) HouseWindow() lipgloss.Style     { return s.fgWarning }
func (s *Styles) UrgencyUpcoming() lipgloss.Style { return s.fgWarning }

// --- Foreground(danger) ---

func (s *Styles) DeletedLabel() lipgloss.Style    { return s.fgDanger }
func (s *Styles) ExtFailed() lipgloss.Style       { return s.fgDanger }
func (s *Styles) ExtFail() lipgloss.Style         { return s.fgDanger }
func (s *Styles) WarrantyExpired() lipgloss.Style { return s.fgDanger }

// --- Foreground(muted) ---

func (s *Styles) Pinned() lipgloss.Style        { return s.fgMuted }
func (s *Styles) LinkIndicator() lipgloss.Style { return s.fgMuted }
func (s *Styles) FilterMark() lipgloss.Style    { return s.fgMuted }
func (s *Styles) ExtSkipLog() lipgloss.Style    { return s.fgMuted }

// --- Foreground(border) ---

func (s *Styles) TableSeparator() lipgloss.Style { return s.fgBorder }
func (s *Styles) DashRule() lipgloss.Style       { return s.fgBorder }
func (s *Styles) Rule() lipgloss.Style           { return s.fgBorder }

// --- Foreground + bold ---

func (s *Styles) TableHeader() lipgloss.Style { return s.fgTextDimBold }

// --- Foreground + italic ---

func (s *Styles) Null() lipgloss.Style           { return s.fgTextDimItalic }
func (s *Styles) DashSubtitle() lipgloss.Style   { return s.fgTextDimItalic }
func (s *Styles) DashHouseValue() lipgloss.Style { return s.fgTextDimItalic }
func (s *Styles) ModelRemote() lipgloss.Style    { return s.fgTextDimItalic }

// --- Foreground(accent) + bold ---

func (s *Styles) ModelActive() lipgloss.Style  { return s.fgAccentBold }
func (s *Styles) ModelLocalHL() lipgloss.Style { return s.fgAccentBold }
func (s *Styles) CalHeader() lipgloss.Style    { return s.fgAccentBold }
func (s *Styles) CalHintKey() lipgloss.Style   { return s.fgAccentBold }
func (s *Styles) AccentBold() lipgloss.Style   { return s.fgAccentBold }

// --- Foreground(accent) + italic ---

func (s *Styles) HiddenRight() lipgloss.Style { return s.fgAccentItalic }

// --- Foreground(secondary) + bold ---

func (s *Styles) ColActiveHeader() lipgloss.Style { return s.fgSecondaryBold }
func (s *Styles) FormDirty() lipgloss.Style       { return s.fgSecondaryBold }

// --- Foreground(secondary) + italic ---

func (s *Styles) HiddenLeft() lipgloss.Style      { return s.fgSecondaryItalic }
func (s *Styles) ChatInterrupted() lipgloss.Style { return s.fgSecondaryItalic }

// --- Foreground(success) + bold ---

func (s *Styles) Info() lipgloss.Style     { return s.fgSuccessBold }
func (s *Styles) CalToday() lipgloss.Style { return s.fgSuccessBold }

// --- Foreground(success) + italic ---

func (s *Styles) ChatNotice() lipgloss.Style { return s.fgSuccessItalic }

// --- Foreground(danger) + bold ---

func (s *Styles) Error() lipgloss.Style          { return s.fgDangerBold }
func (s *Styles) DashOverdue() lipgloss.Style    { return s.fgDangerBold }
func (s *Styles) UrgencyOverdue() lipgloss.Style { return s.fgDangerBold }

// --- Complex / unique ---

func (s *Styles) Header() lipgloss.Style           { return s.bold }
func (s *Styles) Base() lipgloss.Style             { return s.base }
func (s *Styles) HeaderTitle() lipgloss.Style      { return s.accentPill }
func (s *Styles) TabActive() lipgloss.Style        { return s.accentPill }
func (s *Styles) ModeNormal() lipgloss.Style       { return s.accentPill }
func (s *Styles) Drilldown() lipgloss.Style        { return s.accentPill }
func (s *Styles) DashSection() lipgloss.Style      { return s.accentPill }
func (s *Styles) HeaderBox() lipgloss.Style        { return s.headerBox }
func (s *Styles) HeaderBadge() lipgloss.Style      { return s.headerBadge }
func (s *Styles) HeaderSection() lipgloss.Style    { return s.headerSection }
func (s *Styles) Keycap() lipgloss.Style           { return s.keycap }
func (s *Styles) TabInactive() lipgloss.Style      { return s.tabInactive }
func (s *Styles) TabLocked() lipgloss.Style        { return s.tabLocked }
func (s *Styles) TableSelected() lipgloss.Style    { return s.tableSelected }
func (s *Styles) ModeEdit() lipgloss.Style         { return s.modeEdit }
func (s *Styles) DashSectionWarn() lipgloss.Style  { return s.dashSectionWarn }
func (s *Styles) DashSectionAlert() lipgloss.Style { return s.dashSectionAlert }
func (s *Styles) CalCursor() lipgloss.Style        { return s.calCursor }
func (s *Styles) CalSelected() lipgloss.Style      { return s.calSelected }
func (s *Styles) ModelActiveHL() lipgloss.Style    { return s.modelActiveHL }
func (s *Styles) ModelRemoteHL() lipgloss.Style    { return s.modelRemoteHL }
func (s *Styles) DeletedCell() lipgloss.Style      { return s.deletedCell }
func (s *Styles) ExtSkipped() lipgloss.Style       { return s.deletedCell }
func (s *Styles) OverlayBox() lipgloss.Style       { return s.overlayBox }
func (s *Styles) Breadcrumb() lipgloss.Style       { return s.breadcrumb }
func (s *Styles) BlinkCursor() lipgloss.Style      { return s.blinkCursor }

// --- Map-lookup methods ---

func (s *Styles) StatusStyle(key string) (lipgloss.Style, bool) {
	switch key {
	case "ideating":
		return s.fgMuted, true
	case "planned", "open":
		return s.fgAccent, true
	case "quoted":
		return s.fgSecondary, true
	case "underway", "in_progress":
		return s.fgSuccess, true
	case "delayed", "soon":
		return s.fgWarning, true
	case "completed", "resolved", "whenever":
		return s.fgTextDim, true
	case "abandoned", "urgent":
		return s.fgDanger, true
	default:
		return lipgloss.Style{}, false
	}
}

func (s *Styles) EntityKindStyle(letter byte) (lipgloss.Style, bool) {
	switch letter {
	case 'A':
		return s.fgMuted, true
	case 'I':
		return s.fgDanger, true
	case 'M':
		return s.fgSecondary, true
	case 'P':
		return s.fgAccent, true
	case 'Q':
		return s.fgSuccess, true
	case 'V':
		return s.fgWarning, true
	default:
		return lipgloss.Style{}, false
	}
}
