// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/micasa-dev/micasa/internal/data"
)

// Dashboard section title constants.
const (
	dashSectionIncidents = "Incidents"
	dashSectionOverdue   = "Overdue"
	dashSectionUpcoming  = "Upcoming"
	dashSectionSeasonal  = "Seasonal"
	dashSectionProjects  = "Active Projects"
	dashSectionExpiring  = "Expiring Soon"
)

// ---------------------------------------------------------------------------
// Dashboard header
// ---------------------------------------------------------------------------

func (m *Model) dashboardHeader() string {
	return m.styles.DashSubtitle().Render(time.Now().Format("Monday, Jan 2, 2006"))
}

// ---------------------------------------------------------------------------
// Dashboard data types
// ---------------------------------------------------------------------------

// dashboardData holds pre-computed dashboard content. Loaded fresh each time
// the dashboard is opened or returned to.
type dashboardData struct {
	Overdue            []maintenanceUrgency
	Upcoming           []maintenanceUrgency
	Seasonal           []data.MaintenanceItem
	ActiveProjects     []data.Project
	OpenIncidents      []data.Incident
	ExpiringWarranties []warrantyStatus
	InsuranceRenewal   *insuranceStatus
}

func (d dashboardData) empty() bool {
	return len(d.Overdue) == 0 &&
		len(d.Upcoming) == 0 &&
		len(d.Seasonal) == 0 &&
		len(d.ActiveProjects) == 0 &&
		len(d.OpenIncidents) == 0 &&
		len(d.ExpiringWarranties) == 0 &&
		d.InsuranceRenewal == nil
}

type maintenanceUrgency struct {
	Item          data.MaintenanceItem
	NextDue       time.Time
	DaysFromNow   int // negative = overdue, positive = upcoming
	ApplianceName string
}

type warrantyStatus struct {
	Appliance   data.Appliance
	DaysFromNow int // negative = recently expired, positive = expiring soon
}

type insuranceStatus struct {
	Carrier     string
	RenewalDate time.Time
	DaysFromNow int
}

// dashNavEntry maps a dashboard cursor position to either a section header
// (toggle expand/collapse on Enter) or a data row (jump to tab on Enter).
type dashNavEntry struct {
	Tab      TabKind
	ID       string
	Section  string // section title this entry belongs to
	IsHeader bool   // true = section header, not a data row
	InfoOnly bool   // true = cursor can land here but Enter is a no-op
}

// ---------------------------------------------------------------------------
// Mini-table rendering (aligned columns per dashboard section)
// ---------------------------------------------------------------------------

// dashCell is one cell in a dashboard mini-table row.
type dashCell struct {
	Text  string         // raw (unstyled) text for width measurement
	Style lipgloss.Style // applied when rendering
	Align alignKind
}

// dashRow is one navigable row in a section.
type dashRow struct {
	Cells  []dashCell
	Target *dashNavEntry // nil = not navigable (e.g. summary lines)
}

// renderMiniTable renders rows with aligned columns and returns the rendered
// lines. colGap is the space between columns. maxWidth caps the total line
// width; the first column is truncated with an ellipsis when rows would
// otherwise wrap. Pass 0 to disable width capping. When headers is non-nil
// and contains at least one non-empty label, a dim header row is prepended.
func renderMiniTable(
	headers []string, rows []dashRow,
	colGap, maxWidth, cursor int,
	selected, headerStyle lipgloss.Style,
) []string {
	if len(rows) == 0 {
		return nil
	}

	// Compute max display width per column.
	nCols := 0
	for _, r := range rows {
		if len(r.Cells) > nCols {
			nCols = len(r.Cells)
		}
	}
	widths := make([]int, nCols)
	for _, r := range rows {
		for i, c := range r.Cells {
			if w := lipgloss.Width(c.Text); w > widths[i] {
				widths[i] = w
			}
		}
	}
	// Include header labels in width calculation.
	for i, h := range headers {
		if i < nCols {
			if w := lipgloss.Width(h); w > widths[i] {
				widths[i] = w
			}
		}
	}

	// If total width exceeds maxWidth, shrink the first column.
	const indent = 2
	if maxWidth > 0 && nCols > 0 {
		total := indent
		for i, w := range widths {
			total += w
			if i > 0 {
				total += colGap
			}
		}
		if overflow := total - maxWidth; overflow > 0 {
			minFirst := 6
			newFirst := widths[0] - overflow
			if newFirst < minFirst {
				newFirst = minFirst
			}
			widths[0] = newFirst
		}
	}

	gap := strings.Repeat(" ", colGap)
	lines := make([]string, 0, len(rows)+1)

	// Render column header row if any labels are non-empty.
	hasHeaders := false
	for _, h := range headers {
		if h != "" {
			hasHeaders = true
			break
		}
	}
	if hasHeaders {
		parts := make([]string, nCols)
		for i := range nCols {
			label := ""
			if i < len(headers) {
				label = headers[i]
			}
			styled := headerStyle.Render(label)
			pad := widths[i] - lipgloss.Width(label)
			if pad < 0 {
				pad = 0
			}
			parts[i] = styled + strings.Repeat(" ", pad)
		}
		lines = append(lines, "  "+strings.Join(parts, gap))

		// Thin separator under column labels.
		totalW := 0
		for i, w := range widths {
			totalW += w
			if i > 0 {
				totalW += colGap
			}
		}
		lines = append(lines, "  "+headerStyle.Render(strings.Repeat("─", totalW)))
	}

	for rowIdx, r := range rows {
		parts := make([]string, len(r.Cells))
		for i, c := range r.Cells {
			text := c.Text
			// Truncate text that exceeds its column width.
			if tw := lipgloss.Width(text); tw > widths[i] {
				text = truncateToWidth(text, widths[i])
			}
			styled := c.Style.Render(text)
			tw := lipgloss.Width(text)
			pad := widths[i] - tw
			if pad < 0 {
				pad = 0
			}
			if c.Align == alignRight {
				parts[i] = strings.Repeat(" ", pad) + styled
			} else {
				parts[i] = styled + strings.Repeat(" ", pad)
			}
		}
		line := strings.Join(parts, gap)
		if rowIdx == cursor {
			line = "  " + selected.Render(line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return lines
}

// truncateToWidth trims text to fit within maxW display columns, appending
// an ellipsis if truncation occurs. Delegates to ansi.Truncate for correct
// grapheme-cluster and wide-character handling.
func truncateToWidth(text string, maxW int) string {
	return ansi.Truncate(text, maxW, symEllipsis)
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------

func (m *Model) loadDashboard() error {
	return m.loadDashboardAt(time.Now())
}

func (m *Model) loadDashboardAt(now time.Time) error {
	if m.store == nil {
		return nil
	}
	var d dashboardData

	// Maintenance urgency.
	items, err := m.store.ListMaintenanceWithSchedule()
	if err != nil {
		return fmt.Errorf("load maintenance: %w", err)
	}
	for _, item := range items {
		nextDue := data.ComputeNextDue(item.LastServicedAt, item.IntervalMonths, item.DueDate)
		if nextDue == nil {
			continue
		}
		days := daysUntil(now, *nextDue)
		appName := ""
		if item.ApplianceID != nil && item.Appliance.Name != "" {
			appName = item.Appliance.Name
		}
		entry := maintenanceUrgency{
			Item:          item,
			NextDue:       *nextDue,
			DaysFromNow:   days,
			ApplianceName: appName,
		}
		if days < 0 {
			d.Overdue = append(d.Overdue, entry)
		} else if days <= 30 {
			d.Upcoming = append(d.Upcoming, entry)
		}
	}
	sortByDays(d.Overdue)
	sortByDays(d.Upcoming)
	d.Overdue = capSlice(d.Overdue, 10)
	d.Upcoming = capSlice(d.Upcoming, 10)

	// Seasonal maintenance (items tagged with the current season).
	currentSeason := data.SeasonForMonth(now.Month())
	d.Seasonal, err = m.store.ListMaintenanceBySeason(currentSeason)
	if err != nil {
		return fmt.Errorf("load seasonal maintenance: %w", err)
	}

	// Active projects.
	d.ActiveProjects, err = m.store.ListActiveProjects()
	if err != nil {
		return fmt.Errorf("load active projects: %w", err)
	}

	// Open incidents.
	d.OpenIncidents, err = m.store.ListOpenIncidents()
	if err != nil {
		return fmt.Errorf("load open incidents: %w", err)
	}

	// Expiring warranties (expired within 30 days or expiring within 90).
	appliances, err := m.store.ListExpiringWarranties(
		now, 30*24*time.Hour, 90*24*time.Hour,
	)
	if err != nil {
		return fmt.Errorf("load warranties: %w", err)
	}
	for _, a := range appliances {
		if a.WarrantyExpiry == nil {
			continue
		}
		days := daysUntil(now, *a.WarrantyExpiry)
		d.ExpiringWarranties = append(d.ExpiringWarranties, warrantyStatus{
			Appliance:   a,
			DaysFromNow: days,
		})
	}

	// Insurance renewal.
	if m.hasHouse && m.house.InsuranceRenewal != nil {
		days := daysUntil(now, *m.house.InsuranceRenewal)
		if days >= -30 && days <= 90 {
			d.InsuranceRenewal = &insuranceStatus{
				Carrier:     m.house.InsuranceCarrier,
				RenewalDate: *m.house.InsuranceRenewal,
				DaysFromNow: days,
			}
		}
	}

	m.dash.data = d
	m.dash.scrollOffset = 0
	if m.dash.expanded == nil {
		m.dash.expanded = map[string]bool{
			dashSectionIncidents: true,
		}
	}
	m.buildDashNav()
	return nil
}

// ---------------------------------------------------------------------------
// Navigation index
// ---------------------------------------------------------------------------

// buildDashNav builds the flat navigation list from dashboard data. Each
// navigable item maps cursor position -> (tab, id).
// dashNavSection builds nav entries for a dashboard section from a typed slice.
func dashNavSection[T any](
	items []T,
	tab TabKind,
	section string,
	id func(T) string,
) []dashNavEntry {
	entries := make([]dashNavEntry, len(items))
	for i, item := range items {
		entries[i] = dashNavEntry{Tab: tab, ID: id(item), Section: section}
	}
	return entries
}

func (m *Model) buildDashNav() {
	type sectionData struct {
		title   string
		entries []dashNavEntry
	}
	var groups []sectionData

	d := m.dash.data
	add := func(section string, entries []dashNavEntry) {
		if len(entries) > 0 {
			groups = append(groups, sectionData{section, entries})
		}
	}

	add(dashSectionIncidents, dashNavSection(
		d.OpenIncidents, tabIncidents, dashSectionIncidents,
		func(inc data.Incident) string { return inc.ID },
	))
	add(dashSectionOverdue, dashNavSection(
		d.Overdue, tabMaintenance, dashSectionOverdue,
		func(e maintenanceUrgency) string { return e.Item.ID },
	))
	add(dashSectionUpcoming, dashNavSection(
		d.Upcoming, tabMaintenance, dashSectionUpcoming,
		func(e maintenanceUrgency) string { return e.Item.ID },
	))
	add(dashSectionSeasonal, dashNavSection(
		d.Seasonal, tabMaintenance, dashSectionSeasonal,
		func(item data.MaintenanceItem) string { return item.ID },
	))
	add(dashSectionProjects, dashNavSection(
		d.ActiveProjects, tabProjects, dashSectionProjects,
		func(p data.Project) string { return p.ID },
	))

	// Expiring: warranties + optional insurance renewal row.
	expiring := dashNavSection(
		d.ExpiringWarranties, tabAppliances, dashSectionExpiring,
		func(w warrantyStatus) string { return w.Appliance.ID },
	)
	if d.InsuranceRenewal != nil {
		expiring = append(expiring, dashNavEntry{
			Section:  dashSectionExpiring,
			InfoOnly: true,
		})
	}
	add(dashSectionExpiring, expiring)

	var nav []dashNavEntry
	for _, g := range groups {
		nav = append(nav, dashNavEntry{
			Section: g.title, IsHeader: true,
		})
		if m.dash.expanded[g.title] {
			nav = append(nav, g.entries...)
		}
	}

	m.dash.nav = nav
	if m.dash.cursor >= len(nav) {
		m.dash.cursor = max(0, len(nav)-1)
	}
}

func (m *Model) dashNavCount() int {
	return len(m.dash.nav)
}

// ---------------------------------------------------------------------------
// Dashboard view (main entry)
// ---------------------------------------------------------------------------

// dashSection holds one dashboard section's data.
type dashSection struct {
	title   string
	headers []string // optional column labels (rendered dim above data)
	rows    []dashRow
}

// prepareDashboardView rebuilds the dashboard navigation index and clamps
// the cursor. Call this before dashboardView so the view method stays
// read-only with respect to nav state.
func (m *Model) prepareDashboardView() {
	m.buildDashNav()
}

// dashboardView renders the dashboard content, fitting within budget content
// lines and maxWidth display columns. Empty sections are skipped. Sections
// are collapsible — collapsed ones show only a header with item count.
// Scroll windowing keeps the cursor visible when content exceeds budget.
//
// prepareDashboardView must be called before this method to rebuild the nav
// index. The only mutations remaining here are dashTotalLines and
// dashScrollOffset, which depend on rendered line positions and cannot be
// cleanly separated from the render pass.
func (m *Model) dashboardView(budget, maxWidth int) string {
	// Collect non-empty sections. Incidents first — they're urgent reactive
	// issues that need immediate attention. Overdue and upcoming are separate
	// sections so they can be independently collapsed.
	var sections []dashSection

	if incRows := m.dashIncidentRows(); len(incRows) > 0 {
		sections = append(sections, dashSection{
			title:   dashSectionIncidents,
			headers: []string{"", "prio", "reported"},
			rows:    incRows,
		})
	}

	overdueRows, upcomingRows := m.dashMaintSplitRows()
	if len(overdueRows) > 0 {
		sections = append(sections, dashSection{
			title:   dashSectionOverdue,
			headers: []string{"", "overdue"},
			rows:    overdueRows,
		})
	}
	if len(upcomingRows) > 0 {
		sections = append(sections, dashSection{
			title:   dashSectionUpcoming,
			headers: []string{"", "due"},
			rows:    upcomingRows,
		})
	}

	if seasonalRows := m.dashSeasonalRows(); len(seasonalRows) > 0 {
		sections = append(sections, dashSection{
			title:   dashSectionSeasonal,
			headers: []string{"", "category"},
			rows:    seasonalRows,
		})
	}

	if projRows := m.dashProjectRows(); len(projRows) > 0 {
		sections = append(sections, dashSection{
			title:   dashSectionProjects,
			headers: []string{"", "status", "started"},
			rows:    projRows,
		})
	}

	if expRows := m.dashExpiringRows(); len(expRows) > 0 {
		sections = append(sections, dashSection{
			title:   dashSectionExpiring,
			headers: []string{"", "expires"},
			rows:    expRows,
		})
	}

	if len(sections) == 0 {
		return ""
	}

	// Render sections. Collapsed ones show only a header with count.
	sel := m.styles.TableSelected()
	colGap := 3
	navIdx := 0
	var lines []string
	cursorLine := 0

	// Find which section the cursor belongs to so we can dim the rest.
	cursorSection := ""
	if m.dash.cursor >= 0 && m.dash.cursor < len(m.dash.nav) {
		cursorSection = m.dash.nav[m.dash.cursor].Section
	}

	for i, s := range sections {
		expanded := m.dash.expanded[s.title]

		if i > 0 {
			lines = append(lines, "") // blank between sections
		}

		// Section header is always a nav stop. Dim if cursor is in another section.
		isHeaderCursor := navIdx == m.dash.cursor
		dimmed := cursorSection != "" && cursorSection != s.title
		hdr := m.dashSectionHeader(s.title, len(s.rows), dimmed)
		hdr = m.zones.Mark(fmt.Sprintf("%s%d", zoneDashRow, navIdx), hdr)
		if isHeaderCursor {
			cursorLine = len(lines)
		}
		navIdx++
		lines = append(lines, hdr)

		if !expanded {
			continue
		}

		// Expanded: render data rows below the header.
		localCursor := m.dash.cursor - navIdx
		tbl := renderMiniTable(
			s.headers, s.rows, colGap, maxWidth, localCursor, sel, m.styles.DashLabel(),
		)
		// Column header row (if present) offsets data rows by 1.
		headerOffset := len(tbl) - len(s.rows)
		dataNavIdx := navIdx
		for j, row := range tbl {
			dataIdx := j - headerOffset
			if localCursor >= 0 && dataIdx == localCursor {
				cursorLine = len(lines)
			}
			if dataIdx >= 0 && dataIdx < len(s.rows) && s.rows[dataIdx].Target != nil {
				row = m.zones.Mark(fmt.Sprintf("%s%d", zoneDashRow, dataNavIdx), row)
				dataNavIdx++
			}
			lines = append(lines, row)
		}
		navIdx = dataNavIdx
	}

	// Scroll windowing: show only `budget` lines, following the cursor.
	// Reserve lines for scroll indicators (▲/▼) so content is never clipped
	// without feedback. Iterate to convergence since reserving indicator
	// lines can shift whether an indicator is needed.
	m.dash.totalLines = len(lines)
	if budget > 0 && len(lines) > budget {
		indicatorLines := 0
		for range 3 {
			viewportH := budget - indicatorLines
			if viewportH < 1 {
				viewportH = 1
			}
			m.scrollDashTo(cursorLine, viewportH, len(lines))
			end := m.dash.scrollOffset + viewportH
			if end > len(lines) {
				end = len(lines)
			}
			needed := 0
			if m.dash.scrollOffset > 0 {
				needed++
			}
			if end < len(lines) {
				needed++
			}
			if needed == indicatorLines {
				break
			}
			indicatorLines = needed
		}

		viewportH := budget - indicatorLines
		if viewportH < 1 {
			viewportH = 1
		}

		end := m.dash.scrollOffset + viewportH
		if end > len(lines) {
			end = len(lines)
		}

		visible := lines[m.dash.scrollOffset:end]
		var result []string
		if m.dash.scrollOffset > 0 {
			result = append(result, m.styles.DashLabel().Render(
				fmt.Sprintf("  %s %d more", symTriUp, m.dash.scrollOffset)))
		}
		result = append(result, visible...)
		if end < len(lines) {
			result = append(result, m.styles.DashLabel().Render(
				fmt.Sprintf("  %s %d more", symTriDown, len(lines)-end)))
		}
		lines = result
	} else {
		m.dash.scrollOffset = 0
	}

	return strings.Join(lines, "\n")
}

// dashSectionHeader renders a section header badge with a dim item count.
// When dimmed is true, the pill flattens to colored text using the pill's
// background as foreground — so the section color is still visible but the
// full pill only appears on the active section.
func (m *Model) dashSectionHeader(
	title string, count int, dimmed bool,
) string {
	style := m.styles.DashSection()
	switch title {
	case dashSectionIncidents:
		style = m.styles.DashSectionWarn()
	case dashSectionOverdue:
		style = m.styles.DashSectionAlert()
	}
	if dimmed {
		style = appStyles.Base().
			Foreground(style.GetBackground()).
			Padding(0, 1)
	}
	badge := style.Render(title)
	dim := m.styles.DashLabel().Render(fmt.Sprintf(" %d", count))
	return badge + dim
}

// ---------------------------------------------------------------------------
// Row builders (produce dashRow slices for mini-table rendering)
// ---------------------------------------------------------------------------

// dashMaintSplitRows returns overdue and upcoming rows as separate slices.
// Duration cells use the section's accent color: warning for overdue,
// upcoming style for due-soon.
func (m *Model) dashMaintSplitRows() (overdue, upcoming []dashRow) {
	overdue = m.maintUrgencyRows(m.dash.data.Overdue, m.styles.DashOverdue())
	upcoming = m.maintUrgencyRows(m.dash.data.Upcoming, m.styles.DashUpcoming())
	return overdue, upcoming
}

func (m *Model) maintUrgencyRows(
	items []maintenanceUrgency, durStyle lipgloss.Style,
) []dashRow {
	if len(items) == 0 {
		return nil
	}
	rows := make([]dashRow, 0, len(items))
	for _, e := range items {
		rows = append(rows, dashRow{
			Cells: []dashCell{
				{Text: e.Item.Name, Style: m.styles.DashValue()},
				{Text: daysText(e.DaysFromNow), Style: durStyle, Align: alignRight},
			},
			Target: &dashNavEntry{Tab: tabMaintenance, ID: e.Item.ID},
		})
	}
	return rows
}

func (m *Model) dashSeasonalRows() []dashRow {
	d := m.dash.data
	rows := make([]dashRow, 0, len(d.Seasonal))
	for _, item := range d.Seasonal {
		rows = append(rows, dashRow{
			Cells: []dashCell{
				{Text: item.Name, Style: m.styles.DashValue()},
				{Text: item.Category.Name, Style: m.styles.DashLabel()},
			},
			Target: &dashNavEntry{Tab: tabMaintenance, ID: item.ID},
		})
	}
	return rows
}

func (m *Model) dashProjectRows() []dashRow {
	d := m.dash.data
	now := time.Now()
	rows := make([]dashRow, 0, len(d.ActiveProjects))
	for _, p := range d.ActiveProjects {
		statusStyle, _ := m.styles.StatusStyle(p.Status)
		statusText := statusLabel(p.Status)
		started := pastDur(now.Sub(p.CreatedAt))
		rows = append(rows, dashRow{
			Cells: []dashCell{
				{Text: p.Title, Style: m.styles.DashValue()},
				{Text: statusText, Style: statusStyle},
				{Text: started, Style: m.styles.DashLabel(), Align: alignRight},
			},
			Target: &dashNavEntry{Tab: tabProjects, ID: p.ID},
		})
	}
	return rows
}

func (m *Model) dashIncidentRows() []dashRow {
	d := m.dash.data
	now := time.Now()
	rows := make([]dashRow, 0, len(d.OpenIncidents))
	for _, inc := range d.OpenIncidents {
		sevStyle, _ := m.styles.StatusStyle(inc.Severity)
		sevText := statusLabel(inc.Severity)
		rows = append(rows, dashRow{
			Cells: []dashCell{
				{Text: inc.Title, Style: m.styles.DashValue()},
				{Text: sevText, Style: sevStyle},
				{
					Text:  pastDur(now.Sub(inc.DateNoticed)),
					Style: m.styles.DashOverdue(),
					Align: alignRight,
				},
			},
			Target: &dashNavEntry{Tab: tabIncidents, ID: inc.ID},
		})
	}
	return rows
}

func (m *Model) dashExpiringRows() []dashRow {
	d := m.dash.data
	var rows []dashRow
	// WarrantyExpiry is guaranteed non-nil here: ListExpiringWarranties uses
	// WHERE warranty_expiry IS NOT NULL, and loadDashboardAt skips nil entries
	// before populating ExpiringWarranties.
	for _, w := range d.ExpiringWarranties {
		overdue := w.DaysFromNow < 0
		rows = append(rows, dashRow{
			Cells: []dashCell{
				{Text: w.Appliance.Name + " warranty", Style: m.styles.DashValue()},
				{
					Text:  daysText(w.DaysFromNow),
					Style: m.daysStyle(w.DaysFromNow, overdue),
					Align: alignRight,
				},
			},
			Target: &dashNavEntry{Tab: tabAppliances, ID: w.Appliance.ID},
		})
	}
	// Insurance renewal is not navigable (no tab row to jump to).
	if d.InsuranceRenewal != nil {
		ins := d.InsuranceRenewal
		overdue := ins.DaysFromNow < 0
		label := "Insurance renewal"
		if ins.Carrier != "" {
			label = fmt.Sprintf("%s (%s)", label, ins.Carrier)
		}
		rows = append(rows, dashRow{
			Cells: []dashCell{
				{Text: label, Style: m.styles.DashHouseValue()},
				{
					Text:  daysText(ins.DaysFromNow),
					Style: m.daysStyle(ins.DaysFromNow, overdue),
					Align: alignRight,
				},
			},
			Target: &dashNavEntry{InfoOnly: true},
		})
	}
	return rows
}

// ---------------------------------------------------------------------------
// Dashboard keyboard navigation
// ---------------------------------------------------------------------------

func (m *Model) dashDown() {
	n := m.dashNavCount()
	if n == 0 {
		return
	}
	m.dash.cursor++
	if m.dash.cursor >= n {
		m.dash.cursor = n - 1
	}
}

func (m *Model) dashUp() {
	m.dash.cursor--
	if m.dash.cursor < 0 {
		m.dash.cursor = 0
	}
}

func (m *Model) dashTop() {
	m.dash.cursor = 0
	m.dash.scrollOffset = 0
}

func (m *Model) dashBottom() {
	n := m.dashNavCount()
	if n == 0 {
		return
	}
	m.dash.cursor = n - 1
}

// dashNextSection jumps the cursor to the next section header.
func (m *Model) dashNextSection() {
	n := len(m.dash.nav)
	for i := m.dash.cursor + 1; i < n; i++ {
		if m.dash.nav[i].IsHeader {
			m.dash.cursor = i
			return
		}
	}
}

// dashPrevSection jumps the cursor to the previous section header.
func (m *Model) dashPrevSection() {
	for i := m.dash.cursor - 1; i >= 0; i-- {
		if m.dash.nav[i].IsHeader {
			m.dash.cursor = i
			return
		}
	}
}

func (m *Model) dashJump() {
	nav := m.dash.nav
	if m.dash.cursor < 0 || m.dash.cursor >= len(nav) {
		return
	}
	entry := nav[m.dash.cursor]
	if entry.IsHeader {
		return
	}
	if entry.InfoOnly {
		m.dash.flash = "house data, not in any tab"
		return
	}
	m.showDashboard = false
	m.switchToTab(tabIndex(entry.Tab))
	if tab := m.activeTab(); tab != nil {
		selectRowByID(tab, entry.ID)
	}
}

func (m *Model) dashToggleSection(section string) {
	if m.dash.expanded == nil {
		m.dash.expanded = make(map[string]bool)
	}
	m.dash.expanded[section] = !m.dash.expanded[section]
}

// dashToggleCurrent toggles expand/collapse for the section the cursor is in.
func (m *Model) dashToggleCurrent() {
	nav := m.dash.nav
	if m.dash.cursor < 0 || m.dash.cursor >= len(nav) {
		return
	}
	section := nav[m.dash.cursor].Section
	if section == "" {
		return
	}
	m.dashToggleSection(section)
}

// dashToggleAll expands all sections if any are collapsed, otherwise
// collapses all.
func (m *Model) dashToggleAll() {
	if m.dash.expanded == nil {
		m.dash.expanded = make(map[string]bool)
	}
	allExpanded := true
	for _, entry := range m.dash.nav {
		if entry.IsHeader && !m.dash.expanded[entry.Section] {
			allExpanded = false
			break
		}
	}
	for _, entry := range m.dash.nav {
		if entry.IsHeader {
			m.dash.expanded[entry.Section] = !allExpanded
		}
	}
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

// daysText returns a bare compressed duration like "5d" or "today".
func daysText(days int) string {
	if days == 0 {
		return "today"
	}
	abs := days
	if abs < 0 {
		abs = -abs
	}
	return shortDur(time.Duration(abs) * 24 * time.Hour)
}

// daysStyle returns the appropriate style for a timing label, using the
// Styles struct to stay consistent with the colorblind-safe palette.
func (m *Model) daysStyle(days int, overdue bool) lipgloss.Style {
	if days == 0 || overdue {
		return m.styles.DashOverdue()
	}
	return m.styles.DashUpcoming()
}

// pastDur returns a compressed past-duration string. Sub-minute is "<1m".
func pastDur(d time.Duration) string {
	s := shortDur(d)
	if s == "now" {
		return "<1m"
	}
	return s
}

// shortDur returns a compressed duration string like "3d", "2mo", "1y".
func shortDur(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

// daysUntil returns the number of calendar days from now to target using
// each time's local Y/M/D. Negative means target is in the past.
func daysUntil(now, target time.Time) int {
	return dateDiffDays(now, target)
}

func sortByDays(items []maintenanceUrgency) {
	slices.SortFunc(items, func(a, b maintenanceUrgency) int {
		return cmp.Compare(a.DaysFromNow, b.DaysFromNow)
	})
}

func capSlice[T any](s []T, maxLen int) []T {
	if maxLen < 0 {
		maxLen = 0
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// scrollDashTo adjusts dashScrollOffset so that targetLine is visible within
// a window of viewportH lines out of totalLines.
func (m *Model) scrollDashTo(targetLine, viewportH, totalLines int) {
	if targetLine < m.dash.scrollOffset {
		m.dash.scrollOffset = targetLine
	} else if targetLine >= m.dash.scrollOffset+viewportH {
		m.dash.scrollOffset = targetLine - viewportH + 1
	}
	maxOffset := totalLines - viewportH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.dash.scrollOffset > maxOffset {
		m.dash.scrollOffset = maxOffset
	}
	if m.dash.scrollOffset < 0 {
		m.dash.scrollOffset = 0
	}
}
