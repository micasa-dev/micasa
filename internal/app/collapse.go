// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
)

// gapSeparators computes a per-gap separator for the header/data and divider.
// Gaps between visible columns that have hidden columns in between use ⋯ to
// signal a collapsed region. Returns one separator per gap (len(visToFull)-1).
func gapSeparators(
	visToFull []int,
	_ int,
	normalSep string,
) (plainSeps, collapsedSeps []string) {
	n := len(visToFull)
	if n <= 1 {
		return nil, nil
	}
	collapsedSep := appStyles.TableSeparator().Render(" ") +
		appStyles.SecondaryText().Render("⋯") +
		appStyles.TableSeparator().Render(" ")

	plainSeps = make([]string, n-1)
	collapsedSeps = make([]string, n-1)
	for i := 0; i < n-1; i++ {
		plainSeps[i] = normalSep
		if visToFull[i+1] > visToFull[i]+1 { //nolint:gosec // i+1 < n guaranteed by loop bound
			collapsedSeps[i] = collapsedSep
		} else {
			collapsedSeps[i] = normalSep
		}
	}
	return
}

// hiddenColumnNames returns the titles of all hidden columns.
func hiddenColumnNames(specs []columnSpec) []string {
	var names []string
	for _, s := range specs {
		if s.HideOrder > 0 {
			names = append(names, s.Title)
		}
	}
	return names
}

// renderHiddenBadges renders a single left-aligned line of hidden column
// names. Color indicates position relative to the cursor. The first
// left-of-cursor badge gets a leading leftward triangle; the last
// right-of-cursor badge gets a trailing rightward triangle.
//
// To avoid subtle one-character layout jitter while moving the cursor, this
// renderer always reserves space for both the left and right arrow slots,
// even when one side has no hidden columns.
func renderHiddenBadges(
	specs []columnSpec,
	colCursor int,
) string {
	sep := appStyles.HeaderHint().Render(" · ")

	var leftParts, rightParts []string
	for i, spec := range specs {
		if spec.HideOrder == 0 {
			continue
		}
		if i < colCursor {
			leftParts = append(leftParts, spec.Title)
		} else {
			rightParts = append(rightParts, spec.Title)
		}
	}
	if len(leftParts) == 0 && len(rightParts) == 0 {
		return ""
	}

	leftMarker := "  "
	if len(leftParts) > 0 {
		leftMarker = symTriLeft + " " // ◀ + gap
	}
	rightMarker := "  "
	if len(rightParts) > 0 {
		rightMarker = " " + symTriRight // gap + ▶
	}

	var allParts []string
	for i, name := range leftParts {
		if i == 0 {
			name = leftMarker + name
		}
		// Reserve right marker slot when all hidden columns are left of cursor.
		if len(rightParts) == 0 && i == len(leftParts)-1 {
			name += rightMarker
		}
		allParts = append(allParts, appStyles.HiddenLeft().Render(name))
	}
	for i, name := range rightParts {
		// Reserve left marker slot when all hidden columns are right of cursor.
		if len(leftParts) == 0 && i == 0 {
			name = leftMarker + name
		}
		if i == len(rightParts)-1 {
			name += rightMarker
		}
		allParts = append(allParts, appStyles.HiddenRight().Render(name))
	}
	return strings.Join(allParts, sep)
}
