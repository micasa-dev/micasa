// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
)

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

// fuzzyScored is the interface for any match type that carries a score
// and a tiebreaker index.
type fuzzyScored interface {
	fuzzyScore() int
	fuzzyIndex() int
}

// sortFuzzyScored sorts any fuzzyScored slice by score descending,
// breaking ties by index ascending. Uses insertion sort since match
// lists are always small.
func sortFuzzyScored[T fuzzyScored](matches []T) {
	for i := 1; i < len(matches); i++ {
		key := matches[i]
		j := i - 1
		for j >= 0 && fuzzyLessScored(key, matches[j]) {
			matches[j+1] = matches[j]
			j--
		}
		matches[j+1] = key
	}
}

func fuzzyLessScored[T fuzzyScored](a, b T) bool {
	if a.fuzzyScore() != b.fuzzyScore() {
		return a.fuzzyScore() > b.fuzzyScore()
	}
	return a.fuzzyIndex() < b.fuzzyIndex()
}

// highlightFuzzyPositions renders text with matched character positions
// in highlightStyle and unmatched characters in baseStyle.
func highlightFuzzyPositions(
	text string,
	positions []int,
	baseStyle, highlightStyle lipgloss.Style,
) string {
	if len(positions) == 0 {
		return baseStyle.Render(text)
	}

	posSet := make(map[int]bool, len(positions))
	for _, p := range positions {
		posSet[p] = true
	}

	runes := []rune(text)
	var b strings.Builder
	inMatch := false
	var run []rune

	flush := func() {
		if len(run) == 0 {
			return
		}
		if inMatch {
			b.WriteString(highlightStyle.Render(string(run)))
		} else {
			b.WriteString(baseStyle.Render(string(run)))
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
