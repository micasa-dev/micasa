// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// columnFinderState holds the state for the fuzzy column jump overlay.
type columnFinderState struct {
	Query   string
	Matches []columnFinderMatch
	Cursor  int
	// All columns eligible for selection (visible + hidden).
	All []columnFinderEntry
}

// columnFinderEntry represents a single column available for jumping.
type columnFinderEntry struct {
	FullIndex int    // index in tab.Specs
	Title     string // column title
	Hidden    bool   // true if the column is currently hidden
}

// columnFinderMatch is a scored match result with character positions.
type columnFinderMatch struct {
	Entry     columnFinderEntry
	Score     int
	Positions []int // indices of matched characters in Entry.Title
}

func (m columnFinderMatch) fuzzyScore() int { return m.Score }
func (m columnFinderMatch) fuzzyIndex() int { return m.Entry.FullIndex }

// openColumnFinder initializes the column finder overlay for the effective tab.
func (m *Model) openColumnFinder() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	entries := make([]columnFinderEntry, 0, len(tab.Specs))
	for i, spec := range tab.Specs {
		entries = append(entries, columnFinderEntry{
			FullIndex: i,
			Title:     spec.Title,
			Hidden:    spec.HideOrder > 0,
		})
	}
	state := &columnFinderState{All: entries}
	state.refilter()
	m.columnFinder = state
}

// closeColumnFinder dismisses the overlay without jumping.
func (m *Model) closeColumnFinder() {
	m.columnFinder = nil
}

// columnFinderJump jumps to the selected column and closes the finder.
func (m *Model) columnFinderJump() {
	cf := m.columnFinder
	if cf == nil || len(cf.Matches) == 0 {
		m.closeColumnFinder()
		return
	}
	match := cf.Matches[cf.Cursor]
	tab := m.effectiveTab()
	if tab == nil {
		m.closeColumnFinder()
		return
	}

	// If the column is hidden, unhide it first.
	idx := match.Entry.FullIndex
	if idx < len(tab.Specs) && tab.Specs[idx].HideOrder > 0 {
		tab.Specs[idx].HideOrder = 0
	}

	tab.ColCursor = idx
	m.updateTabViewport(tab)
	m.closeColumnFinder()
}

// refilter recomputes matches from the current query.
func (cf *columnFinderState) refilter() {
	if cf.Query == "" {
		// No query: show all columns in original order.
		cf.Matches = make([]columnFinderMatch, len(cf.All))
		for i, entry := range cf.All {
			cf.Matches[i] = columnFinderMatch{Entry: entry, Score: 0}
		}
		cf.clampCursor()
		return
	}

	cf.Matches = cf.Matches[:0]
	for _, entry := range cf.All {
		if score, positions := fuzzyMatch(cf.Query, entry.Title); score > 0 {
			cf.Matches = append(cf.Matches, columnFinderMatch{
				Entry:     entry,
				Score:     score,
				Positions: positions,
			})
		}
	}

	sortFuzzyScored(cf.Matches)
	cf.clampCursor()
}

func (cf *columnFinderState) clampCursor() {
	if cf.Cursor >= len(cf.Matches) {
		cf.Cursor = len(cf.Matches) - 1
	}
	if cf.Cursor < 0 {
		cf.Cursor = 0
	}
}

// handleColumnFinderKey processes keys while the column finder is open.
func (m *Model) handleColumnFinderKey(key tea.KeyPressMsg) tea.Cmd {
	cf := m.columnFinder
	if cf == nil {
		return nil
	}

	switch key.String() {
	case keyEsc:
		m.closeColumnFinder()
		return nil
	case keyEnter:
		m.columnFinderJump()
		return nil
	case keyUp, keyCtrlP:
		if cf.Cursor > 0 {
			cf.Cursor--
		}
		return nil
	case keyDown, keyCtrlN:
		if cf.Cursor < len(cf.Matches)-1 {
			cf.Cursor++
		}
		return nil
	case keyBackspace:
		if len(cf.Query) > 0 {
			_, size := utf8.DecodeLastRuneInString(cf.Query)
			cf.Query = cf.Query[:len(cf.Query)-size]
			cf.refilter()
		}
		return nil
	case keyCtrlU:
		cf.Query = ""
		cf.refilter()
		return nil
	default:
		// Append printable characters to the query.
		if key.Text != "" {
			cf.Query += key.Text
			cf.refilter()
		}
		return nil
	}
}

// buildColumnFinderOverlay renders the fuzzy finder as a bordered box.
func (m *Model) buildColumnFinderOverlay() string {
	cf := m.columnFinder
	if cf == nil {
		return ""
	}

	contentW := max(20, min(40, m.effectiveWidth()-12))
	innerW := contentW - appStyles.OverlayBox().GetHorizontalFrameSize()

	var b strings.Builder

	// Title.
	b.WriteString(m.styles.HeaderSection().Render(" Jump to Column "))
	b.WriteString("\n\n")

	// Input line with "/" prompt.
	prompt := m.styles.Keycap().Render("/")
	cursor := m.styles.BlinkCursor().Render("│")
	queryText := cf.Query + cursor
	if cf.Query == "" {
		queryText = cursor + m.styles.Empty().Render("type to filter")
	}
	b.WriteString(prompt + " " + queryText)
	b.WriteString("\n\n")

	// Match list — fixed height to prevent layout jitter.
	maxVisible := min(10, len(cf.All))

	if len(cf.Matches) == 0 {
		b.WriteString(m.styles.Empty().Render("No matching columns"))
		// Pad remaining lines.
		for i := 1; i < maxVisible; i++ {
			b.WriteString("\n")
		}
	} else {
		visible := min(maxVisible, len(cf.Matches))
		start := max(0, cf.Cursor-visible/2)
		end := start + visible
		if end > len(cf.Matches) {
			end = len(cf.Matches)
			start = max(0, end-visible)
		}

		for i := start; i < end; i++ {
			match := cf.Matches[i]
			selected := i == cf.Cursor

			title := highlightFuzzyMatch(match)

			// Hidden indicator.
			if match.Entry.Hidden {
				title += " " + m.styles.HeaderHint().Render("(hidden)")
			}

			line := "  " + title
			if selected {
				pointer := appStyles.AccentBold().Render("▸ ")
				line = pointer + title
			}

			// Truncate to fit.
			if lipgloss.Width(line) > innerW {
				line = appStyles.Base().MaxWidth(innerW).Render(line)
			}

			b.WriteString(line)
			if i < end-1 {
				b.WriteString("\n")
			}
		}

		// Pad to stable height.
		for i := visible; i < maxVisible; i++ {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	hints := joinWithSeparator(
		m.helpSeparator(),
		m.helpItem(symReturn, "jump"),
		m.helpItem(keyEsc, "cancel"),
	)
	b.WriteString(hints)

	return appStyles.OverlayBox().
		Width(contentW).
		Render(b.String())
}

// highlightFuzzyMatch renders a column title with matched characters
// in the accent color and bold.
func highlightFuzzyMatch(match columnFinderMatch) string {
	return highlightFuzzyPositions(
		match.Entry.Title,
		match.Positions,
		appStyles.HeaderHint(),
		appStyles.AccentBold(),
	)
}
