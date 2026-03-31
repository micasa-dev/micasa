// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/micasa-dev/micasa/internal/data"
)

// calendarState tracks the date picker overlay.
type calendarState struct {
	Cursor    time.Time // the date the cursor is on
	Selected  time.Time // the date the field currently has (dim highlight)
	HasValue  bool      // whether Selected is meaningful
	FieldPtr  *string   // pointer to the form field's value string
	OnConfirm func()    // called after writing the picked date to FieldPtr
}

// calendarMaxRows is the maximum number of week-rows a month can span.
// A 31-day month starting on Saturday needs 6 rows.
const calendarMaxRows = 6

// calendarGrid renders a single month calendar with the cursor highlighted
// and a key-hint column on the left. The grid is always calendarMaxRows tall
// so the overlay never changes size when switching months.
func calendarGrid(cal calendarState) string {
	cursor := cal.Cursor
	year, month := cursor.Year(), cursor.Month()

	// Header: month name + year, centered over the day grid.
	header := appStyles.CalHeader().
		Render(fmt.Sprintf(" %s %d ", month.String(), year))

	// Day-of-week labels.
	dayLabels := appStyles.CalDayLabel().
		Render("Su Mo Tu We Th Fr Sa")

	calW := lipgloss.Width(dayLabels) // 20

	// Build the day grid.
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	startDow := int(first.Weekday()) // 0=Sun
	daysInMonth := daysIn(year, month)

	var gridRows []string
	var row strings.Builder
	// Leading blanks.
	for range startDow {
		row.WriteString("   ")
	}

	for day := 1; day <= daysInMonth; day++ {
		date := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
		label := fmt.Sprintf("%2d", day)

		isCursor := sameDay(date, cursor)
		isSelected := cal.HasValue && sameDay(date, cal.Selected)
		isToday := sameDay(date, time.Now())

		var style lipgloss.Style
		switch {
		case isCursor:
			style = appStyles.CalCursor()
		case isSelected:
			style = appStyles.CalSelected()
		case isToday:
			style = appStyles.CalToday()
		default:
			style = appStyles.Base()
		}

		row.WriteString(style.Render(label))

		dow := (startDow + day - 1) % 7
		if dow == 6 && day < daysInMonth {
			gridRows = append(gridRows, row.String())
			row.Reset()
		} else if day < daysInMonth {
			row.WriteString(" ")
		}
	}
	// Flush the last partial row.
	if row.Len() > 0 {
		gridRows = append(gridRows, row.String())
	}
	// Pad to fixed height so the overlay never resizes.
	for len(gridRows) < calendarMaxRows {
		gridRows = append(gridRows, "")
	}

	gridBlock := padLines(strings.Join(gridRows, "\n"), calW)

	// Right panel: centered header + day labels + fixed-height grid.
	rightPanel := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.PlaceHorizontal(calW, lipgloss.Center, header),
		"",
		dayLabels,
		gridBlock,
	)

	// Left panel: key hints stacked vertically, right-aligned keys.
	hints := calendarHints()
	hintsH := lipgloss.Height(hints)
	rightH := lipgloss.Height(rightPanel)
	// Vertically center hints against the right panel.
	topPad := max((rightH-hintsH)/2, 0)
	paddedHints := strings.Repeat("\n", topPad) + hints

	return lipgloss.JoinHorizontal(lipgloss.Top, paddedHints, "   ", rightPanel)
}

// calendarHints renders the key legend as a two-column vertical list.
func calendarHints() string {
	dim := appStyles.CalDayLabel()
	key := appStyles.KeycapLight()

	type hint struct{ k, v string }
	items := []hint{
		{keyH + "/" + keyL, "day"},
		{keyJ + "/" + keyK, "week"},
		{keyShiftH + "/" + keyShiftL, "month"},
		{keyLBracket + "/" + keyRBracket, "year"},
		{keyT, "today"},
		{symReturn, "pick"},
		{keyEsc, "cancel"},
	}

	// Right-align the key column.
	maxKeyW := 0
	for _, h := range items {
		if len(h.k) > maxKeyW {
			maxKeyW = len(h.k)
		}
	}

	lines := make([]string, len(items))
	for i, h := range items {
		k := fmt.Sprintf("%*s", maxKeyW, h.k)
		lines[i] = key.Render(k) + " " + dim.Render(h.v)
	}
	return strings.Join(lines, "\n")
}

func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// padLines right-pads each line in s so every line is exactly width visible
// columns. This makes the block rectangular so uniform indentation preserves
// internal column alignment.
func padLines(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if w := lipgloss.Width(line); w < width {
			lines[i] = line + strings.Repeat(" ", width-w)
		}
	}
	return strings.Join(lines, "\n")
}

// calendarToday jumps the calendar cursor to today's date.
func calendarToday(cal *calendarState) {
	now := time.Now()
	cal.Cursor = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
}

// calendarMove adjusts the calendar cursor by the given number of days.
func calendarMove(cal *calendarState, days int) {
	cal.Cursor = cal.Cursor.AddDate(0, 0, days)
}

// calendarMoveMonth adjusts the calendar cursor by the given number of months,
// clamping the day to the last day of the target month to avoid the
// time.AddDate overflow (e.g. Jan 31 + 1 month = March 3).
func calendarMoveMonth(cal *calendarState, months int) {
	cal.Cursor = data.AddMonths(cal.Cursor, months)
}

// calendarMoveYear adjusts the calendar cursor by the given number of years,
// clamping the day to the last day of the target month (handles Feb 29 in
// non-leap years).
func calendarMoveYear(cal *calendarState, years int) {
	cal.Cursor = data.AddMonths(cal.Cursor, years*12)
}
