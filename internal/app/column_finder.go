// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// Sort by score descending, then by original column order.
	sortFuzzyMatches(cf.Matches)
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

// fuzzyMatch scores how well query matches target (case-insensitive).
// Returns 0 if the query doesn't match. Higher scores are better.
// Bonuses: consecutive chars, word-boundary matches, prefix match.
func fuzzyMatch(query, target string) (int, []int) {
	qRunes := []rune(strings.ToLower(query))
	tRunes := []rune(strings.ToLower(target))

	if len(qRunes) == 0 {
		return 1, nil
	}
	if len(qRunes) > len(tRunes) {
		return 0, nil
	}

	positions := make([]int, 0, len(qRunes))
	score := 0
	qi := 0
	prevMatchIdx := -1

	for ti := 0; ti < len(tRunes) && qi < len(qRunes); ti++ {
		if tRunes[ti] == qRunes[qi] {
			positions = append(positions, ti)
			score += 10 // base match point

			// Consecutive bonus.
			if prevMatchIdx == ti-1 {
				score += 15
			}

			// Word boundary bonus: start of string or preceded by
			// a non-letter (space, underscore, etc.).
			if ti == 0 || !unicode.IsLetter(tRunes[ti-1]) {
				score += 20
			}

			// Exact prefix bonus.
			if ti == qi {
				score += 25
			}

			prevMatchIdx = ti
			qi++
		}
	}

	if qi < len(qRunes) {
		return 0, nil // not all query chars matched
	}

	// Bonus for matching a larger fraction of the target.
	score += (len(qRunes) * 10) / len(tRunes)

	return score, positions
}

// sortFuzzyMatches sorts matches by score descending, breaking ties by
// original column order (FullIndex ascending).
func sortFuzzyMatches(matches []columnFinderMatch) {
	// Simple insertion sort -- column count is always small.
	for i := 1; i < len(matches); i++ {
		key := matches[i]
		j := i - 1
		for j >= 0 && fuzzyLess(key, matches[j]) {
			matches[j+1] = matches[j]
			j--
		}
		matches[j+1] = key
	}
}

func fuzzyLess(a, b columnFinderMatch) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Entry.FullIndex < b.Entry.FullIndex
}

// handleColumnFinderKey processes keys while the column finder is open.
func (m *Model) handleColumnFinderKey(key tea.KeyMsg) tea.Cmd {
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
		for _, r := range key.Runes {
			cf.Query += string(r)
		}
		if len(key.Runes) > 0 {
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

	contentW := 40
	if m.effectiveWidth()-12 < contentW {
		contentW = m.effectiveWidth() - 12
	}
	if contentW < 20 {
		contentW = 20
	}
	innerW := contentW - 4 // padding

	var b strings.Builder

	// Title.
	b.WriteString(m.styles.HeaderSection.Render(" Jump to Column "))
	b.WriteString("\n\n")

	// Input line with "/" prompt.
	prompt := m.styles.Keycap.Render("/")
	cursor := m.styles.HeaderHint.Render("│")
	queryText := cf.Query + cursor
	if cf.Query == "" {
		queryText = m.styles.Empty.Render("type to filter") + cursor
	}
	b.WriteString(prompt + " " + queryText)
	b.WriteString("\n\n")

	// Match list.
	if len(cf.Matches) == 0 {
		b.WriteString(m.styles.Empty.Render("No matching columns"))
	} else {
		// Show up to 10 matches, centered around the cursor.
		maxVisible := 10
		if maxVisible > len(cf.Matches) {
			maxVisible = len(cf.Matches)
		}
		start := cf.Cursor - maxVisible/2
		if start < 0 {
			start = 0
		}
		end := start + maxVisible
		if end > len(cf.Matches) {
			end = len(cf.Matches)
			start = end - maxVisible
			if start < 0 {
				start = 0
			}
		}

		for i := start; i < end; i++ {
			match := cf.Matches[i]
			selected := i == cf.Cursor

			title := highlightFuzzyMatch(match)

			// Hidden indicator.
			if match.Entry.Hidden {
				title += " " + m.styles.HeaderHint.Render("(hidden)")
			}

			line := "  " + title
			if selected {
				pointer := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("▸ ")
				line = pointer + title
			}

			// Truncate to fit.
			if lipgloss.Width(line) > innerW {
				line = lipgloss.NewStyle().MaxWidth(innerW).Render(line)
			}

			b.WriteString(line)
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	hints := joinWithSeparator(
		m.helpSeparator(),
		m.helpItem(symReturn, "jump"),
		m.helpItem(keyEsc, "cancel"),
	)
	b.WriteString(hints)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Width(contentW).
		Render(b.String())
}

// highlightFuzzyMatch renders a column title with matched characters
// in the accent color and bold.
func highlightFuzzyMatch(match columnFinderMatch) string {
	title := match.Entry.Title
	if len(match.Positions) == 0 {
		return appStyles.HeaderHint.Render(title)
	}

	posSet := make(map[int]bool, len(match.Positions))
	for _, p := range match.Positions {
		posSet[p] = true
	}

	matchStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)
	dimStyle := appStyles.HeaderHint

	runes := []rune(title)
	var b strings.Builder
	inMatch := false
	var run []rune

	flush := func() {
		if len(run) == 0 {
			return
		}
		if inMatch {
			b.WriteString(matchStyle.Render(string(run)))
		} else {
			b.WriteString(dimStyle.Render(string(run)))
		}
		run = run[:0]
	}

	for i, r := range runes {
		matched := posSet[i]
		if matched != inMatch {
			flush()
			inMatch = matched
		}
		run = append(run, r)
	}
	flush()

	return b.String()
}
