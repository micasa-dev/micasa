// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cpcloud/micasa/internal/data"
)

// Zone ID prefix for clickable search result rows.
const zoneSearchRow = "search-"

// docSearchState holds the state for the document search overlay.
type docSearchState struct {
	Input   textinput.Model
	Results []data.DocumentSearchResult
	Cursor  int
	Names   entityNameMap
}

// openDocSearch shows the document search overlay.
func (m *Model) openDocSearch() tea.Cmd {
	ti := textinput.New()
	ti.Placeholder = "search documents..."
	ti.CharLimit = 200
	ti.SetWidth(m.searchInputWidth())
	blinkCmd := ti.Focus()

	m.docSearch = &docSearchState{
		Input: ti,
		Names: buildEntityNameMap(m.store),
	}
	return blinkCmd
}

// closeDocSearch dismisses the search overlay.
func (m *Model) closeDocSearch() {
	m.docSearch = nil
}

// searchInputWidth returns the input field width for the search overlay.
func (m *Model) searchInputWidth() int {
	return m.searchOverlayWidth() - 10
}

// searchOverlayWidth returns the outer width of the search overlay.
func (m *Model) searchOverlayWidth() int {
	w := m.effectiveWidth() - 12
	if w > 72 {
		w = 72
	}
	if w < 30 {
		w = 30
	}
	return w
}

// handleDocSearchKey processes keys while the search overlay is open.
func (m *Model) handleDocSearchKey(key tea.KeyPressMsg) tea.Cmd {
	ds := m.docSearch
	if ds == nil {
		return nil
	}

	switch key.String() {
	case keyEsc:
		m.closeDocSearch()
		return nil
	case keyEnter:
		m.docSearchNavigate()
		return nil
	case keyUp, keyCtrlP, keyCtrlK:
		if ds.Cursor > 0 {
			ds.Cursor--
		}
		return nil
	case keyDown, keyCtrlN, keyCtrlJ:
		if ds.Cursor < len(ds.Results)-1 {
			ds.Cursor++
		}
		return nil
	default:
		// Forward to textinput for typing.
		var cmd tea.Cmd
		ds.Input, cmd = ds.Input.Update(key)
		m.runDocSearch()
		return cmd
	}
}

// runDocSearch queries the FTS index with the current input value.
func (m *Model) runDocSearch() {
	ds := m.docSearch
	if ds == nil {
		return
	}
	query := ds.Input.Value()
	if strings.TrimSpace(query) == "" {
		ds.Results = nil
		ds.Cursor = 0
		return
	}
	results, err := m.store.SearchDocuments(query)
	if err != nil {
		ds.Results = nil
		ds.Cursor = 0
		return
	}
	ds.Results = results
	if ds.Cursor >= len(ds.Results) {
		ds.Cursor = len(ds.Results) - 1
	}
	if ds.Cursor < 0 {
		ds.Cursor = 0
	}
}

// docSearchNavigate jumps to the selected search result: switches to the
// Documents tab and selects the matching row.
func (m *Model) docSearchNavigate() {
	ds := m.docSearch
	if ds == nil || len(ds.Results) == 0 {
		return
	}
	result := ds.Results[ds.Cursor]
	m.closeDocSearch()

	// If in a scoped document detail view, pop back to the top-level
	// Documents tab so the full document list is visible for selection.
	if m.inDetail() {
		m.closeAllDetails()
		for i, tab := range m.tabs {
			if tab.Kind == tabDocuments {
				m.active = i
				break
			}
		}
	}

	// Select the matching row by document ID.
	tab := m.effectiveTab()
	if tab != nil {
		selectRowByID(tab, result.ID)
	}
}

// buildDocSearchOverlay renders the search overlay as a bordered box.
func (m *Model) buildDocSearchOverlay() string {
	ds := m.docSearch
	if ds == nil {
		return ""
	}

	contentW := m.searchOverlayWidth()
	innerW := contentW - 4 // padding

	var b strings.Builder

	// Title.
	b.WriteString(m.styles.HeaderSection().Render(" Search Documents "))
	b.WriteString("\n\n")

	// Input field.
	b.WriteString(ds.Input.View())
	b.WriteString("\n\n")

	query := strings.TrimSpace(ds.Input.Value())

	if query == "" {
		b.WriteString(m.styles.Empty().Render("type to search across all documents"))
	} else if len(ds.Results) == 0 {
		b.WriteString(m.styles.Empty().Render("no matches"))
	} else {
		// Show up to 8 results, centered around the cursor.
		maxVisible := 8
		if maxVisible > len(ds.Results) {
			maxVisible = len(ds.Results)
		}
		start := ds.Cursor - maxVisible/2
		if start < 0 {
			start = 0
		}
		end := start + maxVisible
		if end > len(ds.Results) {
			end = len(ds.Results)
			start = end - maxVisible
			if start < 0 {
				start = 0
			}
		}

		for i := start; i < end; i++ {
			result := ds.Results[i]
			selected := i == ds.Cursor

			line := m.renderSearchResult(result, selected, innerW)
			zoned := m.zones.Mark(fmt.Sprintf("%s%d", zoneSearchRow, i), line)
			b.WriteString(zoned)

			if i < end-1 {
				b.WriteString("\n")
			}
		}

		// Result count.
		if len(ds.Results) > maxVisible {
			b.WriteString("\n")
			countLabel := fmt.Sprintf("%d results", len(ds.Results))
			b.WriteString(m.styles.Empty().Render(countLabel))
		}
	}

	b.WriteString("\n\n")
	hints := joinWithSeparator(
		m.helpSeparator(),
		m.helpItem(symReturn, "open"),
		m.helpItem(symUp+"/"+symDown, "nav"),
		m.helpItem(keyEsc, "close"),
	)
	b.WriteString(hints)

	return appStyles.OverlayBox().
		Width(contentW).
		MaxHeight(m.overlayMaxHeight()).
		Render(b.String())
}

// renderSearchResult renders a single search result entry.
func (m *Model) renderSearchResult(
	result data.DocumentSearchResult,
	selected bool,
	maxW int,
) string {
	var lines []string

	// Line 1: pointer + title + entity association.
	pointer := "  "
	titleStyle := m.styles.HeaderHint()
	if selected {
		pointer = appStyles.AccentBold().Render(symTriRightSm) + " "
		titleStyle = appStyles.AccentBold()
	}

	title := titleStyle.Render(result.Title)

	// Entity label (e.g., "P Kitchen Reno").
	var entityLabel string
	if result.EntityKind != "" {
		entityLabel = documentEntityLabel(result.EntityKind, result.EntityID, m.docSearch.Names)
		if entityLabel != "" {
			entityLabel = " " + m.styles.TextDim().Render(entityLabel)
		}
	}

	titleLine := pointer + title + entityLabel
	if lipgloss.Width(titleLine) > maxW {
		titleLine = appStyles.Base().MaxWidth(maxW).Render(titleLine)
	}
	lines = append(lines, titleLine)

	// Line 2: snippet (indented, dimmed, with match highlights).
	if result.Snippet != "" {
		snippet := m.formatSnippet(result.Snippet, maxW-4)
		lines = append(lines, "    "+snippet)
	}

	return strings.Join(lines, "\n")
}

// formatSnippet formats an FTS5 snippet, replacing >>> and <<< markers
// with styled highlights, and truncating to fit.
func (m *Model) formatSnippet(snippet string, maxW int) string {
	// FTS5 snippet() uses >>> and <<< as match markers.
	// Replace with styled highlights.
	var b strings.Builder
	remaining := snippet
	for {
		startIdx := strings.Index(remaining, ">>>")
		if startIdx < 0 {
			b.WriteString(m.styles.TextDim().Render(remaining))
			break
		}
		// Text before match.
		if startIdx > 0 {
			b.WriteString(m.styles.TextDim().Render(remaining[:startIdx]))
		}
		remaining = remaining[startIdx+3:]

		endIdx := strings.Index(remaining, "<<<")
		if endIdx < 0 {
			// Unmatched marker -- render rest as-is.
			b.WriteString(appStyles.AccentBold().Render(remaining))
			break
		}
		// Highlighted match.
		b.WriteString(appStyles.AccentBold().Render(remaining[:endIdx]))
		remaining = remaining[endIdx+3:]
	}

	result := b.String()
	if lipgloss.Width(result) > maxW {
		result = appStyles.Base().MaxWidth(maxW).Render(result)
	}
	return result
}
