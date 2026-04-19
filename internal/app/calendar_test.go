// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/micasa-dev/micasa/internal/data"
)

const testDate = "2026-02-15"

func TestCalendarGridRendersMonth(t *testing.T) {
	t.Parallel()
	cal := calendarState{
		Cursor:   time.Date(2026, 2, 15, 0, 0, 0, 0, time.Local),
		HasValue: false,
	}
	grid := calendarGrid(cal)
	assert.Contains(t, grid, "February 2026")
	assert.Contains(t, grid, "Su Mo Tu We Th Fr Sa")
	// Feb 2026 has 28 days.
	assert.Contains(t, grid, "28")
}

func TestCalendarMoveDay(t *testing.T) {
	t.Parallel()
	cal := &calendarState{
		Cursor: time.Date(2026, 2, 15, 0, 0, 0, 0, time.Local),
	}
	calendarMove(cal, 1)
	assert.Equal(t, 16, cal.Cursor.Day())
	calendarMove(cal, -2)
	assert.Equal(t, 14, cal.Cursor.Day())
}

func TestCalendarMoveWeek(t *testing.T) {
	t.Parallel()
	cal := &calendarState{
		Cursor: time.Date(2026, 2, 15, 0, 0, 0, 0, time.Local),
	}
	calendarMove(cal, 7)
	assert.Equal(t, 22, cal.Cursor.Day())
}

func TestCalendarMoveMonth(t *testing.T) {
	t.Parallel()
	cal := &calendarState{
		Cursor: time.Date(2026, 2, 15, 0, 0, 0, 0, time.Local),
	}
	calendarMoveMonth(cal, 1)
	assert.Equal(t, time.March, cal.Cursor.Month())
	calendarMoveMonth(cal, -2)
	assert.Equal(t, time.January, cal.Cursor.Month())
}

func TestCalendarMoveCrossesMonthBoundary(t *testing.T) {
	t.Parallel()
	cal := &calendarState{
		Cursor: time.Date(2026, 1, 31, 0, 0, 0, 0, time.Local),
	}
	calendarMove(cal, 1)
	assert.Equal(t, time.February, cal.Cursor.Month())
	assert.Equal(t, 1, cal.Cursor.Day())
}

func TestDaysIn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		year  int
		month time.Month
		want  int
	}{
		{2026, time.February, 28},
		{2024, time.February, 29}, // leap year
		{2026, time.January, 31},
		{2026, time.April, 30},
	}
	for _, tt := range tests {
		assert.Equalf(t, tt.want, daysIn(tt.year, tt.month),
			"daysIn(%d, %v)", tt.year, tt.month)
	}
}

func TestSameDay(t *testing.T) {
	t.Parallel()
	a := time.Date(2026, 2, 10, 9, 30, 0, 0, time.UTC)
	b := time.Date(2026, 2, 10, 18, 0, 0, 0, time.UTC)
	c := time.Date(2026, 2, 11, 9, 30, 0, 0, time.UTC)
	assert.True(t, sameDay(a, b))
	assert.False(t, sameDay(a, c))
}

func TestCalendarKeyNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	dateVal := testDate
	m.openCalendar(&dateVal, nil)
	require.NotNil(t, m.calendar)
	require.Contains(t, m.buildView(), "February 2026", "calendar should be visible")
	assert.Equal(t, 15, m.calendar.Cursor.Day())

	// Move right (l).
	sendKey(m, "l")
	assert.Equal(t, 16, m.calendar.Cursor.Day())

	// Move down (j) = +7 days.
	sendKey(m, "j")
	assert.Equal(t, 23, m.calendar.Cursor.Day())

	// Move left (h).
	sendKey(m, "h")
	assert.Equal(t, 22, m.calendar.Cursor.Day())

	// Move up (k) = -7 days.
	sendKey(m, "k")
	assert.Equal(t, 15, m.calendar.Cursor.Day())

	grid := calendarGrid(*m.calendar)
	assert.Contains(t, grid, "February 2026", "should still show February after navigation")
}

func TestCalendarConfirmWritesDate(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	dateVal := ""
	confirmed := false
	m.openCalendar(&dateVal, func() { confirmed = true })

	// Navigate to a specific date.
	m.calendar.Cursor = time.Date(
		2026, 3, 20, 0, 0, 0, 0, time.Local,
	)
	sendKey(m, "enter")

	assert.Equal(t, "2026-03-20", dateVal)
	assert.True(t, confirmed)
	assert.Nil(t, m.calendar)
	// Calendar should be dismissed -- tab hints visible again.
	assert.Contains(t, m.statusView(), "NAV")
}

func TestCalendarEscCancels(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	dateVal := testDate
	m.openCalendar(&dateVal, nil)
	sendKey(m, "esc")
	assert.Nil(t, m.calendar)
	// Calendar should be dismissed -- tab hints visible again.
	assert.Contains(t, m.statusView(), "NAV")
	assert.Equal(t, testDate, dateVal)
}

func TestDatePickerEscClearsFormState(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	dateVal := testDate
	id := "01JTEST00000000000000042"

	// Simulate what openDatePicker does: set form state then open calendar.
	m.fs.editID = &id
	m.fs.formData = &maintenanceFormData{}
	m.openCalendar(&dateVal, nil)
	require.NotNil(t, m.calendar)

	// Preconditions: form state is set.
	require.NotNil(t, m.fs.editID)
	require.Equal(t, formMaintenance, m.fs.formKind())
	require.NotNil(t, m.fs.formData)

	// Press ESC to cancel.
	sendKey(m, "esc")

	assert.Nil(t, m.calendar, "calendar should be dismissed")
	assert.Equal(t, formNone, m.fs.formKind(), "formKind should be reset after ESC")
	assert.Nil(t, m.fs.formData, "formData should be cleared after ESC")
	assert.Nil(t, m.fs.editID, "editID should be cleared after ESC")
	assert.Equal(t, testDate, dateVal, "date value should be unchanged on cancel")
}

func TestCalendarRendersInView(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	m.width = 120
	m.height = 40
	dateVal := testDate
	m.openCalendar(&dateVal, nil)

	view := m.buildView()
	assert.Contains(t, view, "February 2026")
	assert.Contains(t, view, "pick")
}

func TestCalendarMonthNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	dateVal := testDate
	m.openCalendar(&dateVal, nil)

	// H = previous month -- grid should show January.
	sendKey(m, "H")
	assert.Equal(t, time.January, m.calendar.Cursor.Month())
	grid := calendarGrid(*m.calendar)
	assert.Contains(t, grid, "January 2026")

	// L = next month twice -- grid should show March.
	sendKey(m, "L")
	sendKey(m, "L")
	assert.Equal(t, time.March, m.calendar.Cursor.Month())
	grid = calendarGrid(*m.calendar)
	assert.Contains(t, grid, "March 2026")
}

func TestCalendarYearNavigation(t *testing.T) {
	t.Parallel()
	cal := &calendarState{
		Cursor: time.Date(2026, 2, 15, 0, 0, 0, 0, time.Local),
	}
	calendarMoveYear(cal, 1)
	assert.Equal(t, 2027, cal.Cursor.Year())
	assert.Equal(t, time.February, cal.Cursor.Month())
	assert.Equal(t, 15, cal.Cursor.Day())
	calendarMoveYear(cal, -2)
	assert.Equal(t, 2025, cal.Cursor.Year())
}

func TestCalendarGridColumnAlignment(t *testing.T) {
	t.Parallel()
	cal := calendarState{
		Cursor:   time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local),
		HasValue: false,
	}
	grid := calendarGrid(cal)

	lines := strings.Split(grid, "\n")
	labelIdx := -1
	lastDayIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Su Mo Tu We Th Fr Sa") {
			labelIdx = i
		}
		if strings.Contains(line, "29") && strings.Contains(line, "30") {
			lastDayIdx = i
		}
	}
	require.NotEqual(t, -1, labelIdx, "day-label line not found")
	require.NotEqual(t, -1, lastDayIdx, "line with 29 and 30 not found")

	// Strip ANSI escape sequences before comparing visual column positions.
	// Different styled spans produce different-length escapes, so raw byte
	// positions are unreliable for alignment checks.
	labelPlain := ansi.Strip(lines[labelIdx])
	lastPlain := ansi.Strip(lines[lastDayIdx])

	suPos := strings.Index(labelPlain, "Su")
	moPos := strings.Index(labelPlain, "Mo")
	pos29 := strings.Index(lastPlain, "29")
	pos30 := strings.Index(lastPlain, "30")
	require.GreaterOrEqual(t, suPos, 0)
	require.GreaterOrEqual(t, pos29, 0)
	assert.Equalf(t, suPos, pos29,
		"column misalignment: Su at col %d but 29 at col %d\nlabels: %q\nlast:   %q",
		suPos, pos29, labelPlain, lastPlain)
	require.GreaterOrEqual(t, moPos, 0)
	require.GreaterOrEqual(t, pos30, 0)
	assert.Equalf(t, moPos, pos30,
		"column misalignment: Mo at col %d but 30 at col %d\nlabels: %q\nlast:   %q",
		moPos, pos30, labelPlain, lastPlain)
}

func TestCalendarFixedHeight(t *testing.T) {
	t.Parallel()
	feb := calendarGrid(calendarState{
		Cursor: time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local),
	})
	aug := calendarGrid(calendarState{
		Cursor: time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local),
	})

	assert.Equal(t, lipgloss.Height(feb), lipgloss.Height(aug), "calendar height should be fixed")
}

func TestCalendarHintsOnLeft(t *testing.T) {
	t.Parallel()
	grid := calendarGrid(calendarState{
		Cursor: time.Date(2026, 2, 15, 0, 0, 0, 0, time.Local),
	})

	lines := strings.Split(grid, "\n")
	foundHint := false
	foundDays := false
	for _, line := range lines {
		hintIdx := strings.Index(line, "h/l")
		daysIdx := strings.Index(line, "Su Mo")
		if hintIdx >= 0 {
			foundHint = true
		}
		if daysIdx >= 0 {
			foundDays = true
		}
		// If both appear on the same line, hint must be left of days.
		if hintIdx >= 0 && daysIdx >= 0 {
			assert.Less(t, hintIdx, daysIdx, "hints should be left of day grid")
		}
	}
	assert.True(t, foundHint, "expected hint keys in calendar output")
	assert.True(t, foundDays, "expected day labels in calendar output")
}

func TestCalendarMoveMonthFromJan31ClampsFeb(t *testing.T) {
	t.Parallel()
	// User scenario: cursor is on Jan 31, user presses L (next month).
	// Should land on Feb 28 (not March 3 from time.AddDate overflow).
	cal := &calendarState{
		Cursor: time.Date(2025, 1, 31, 0, 0, 0, 0, time.Local),
	}
	calendarMoveMonth(cal, 1)
	assert.Equal(t, time.February, cal.Cursor.Month())
	assert.Equal(t, 28, cal.Cursor.Day())
}

func TestCalendarMoveMonthFromJan31LeapYear(t *testing.T) {
	t.Parallel()
	cal := &calendarState{
		Cursor: time.Date(2024, 1, 31, 0, 0, 0, 0, time.Local),
	}
	calendarMoveMonth(cal, 1)
	assert.Equal(t, time.February, cal.Cursor.Month())
	assert.Equal(t, 29, cal.Cursor.Day())
}

func TestCalendarMoveYearFromFeb29ClampsFeb28(t *testing.T) {
	t.Parallel()
	// User scenario: cursor is on Feb 29 in a leap year, user presses ]
	// (next year). Should land on Feb 28, not March 1.
	cal := &calendarState{
		Cursor: time.Date(2024, 2, 29, 0, 0, 0, 0, time.Local),
	}
	calendarMoveYear(cal, 1)
	assert.Equal(t, 2025, cal.Cursor.Year())
	assert.Equal(t, time.February, cal.Cursor.Month())
	assert.Equal(t, 28, cal.Cursor.Day())
}

func TestCalendarMoveMonthViaKeyboardClamps(t *testing.T) {
	t.Parallel()
	// End-to-end user flow: open calendar on Jan 31, press L (next month).
	m := newTestModel(t)
	dateVal := "2025-01-31"
	m.openCalendar(&dateVal, nil)
	require.NotNil(t, m.calendar)
	require.Contains(t, m.buildView(), "January 2025", "calendar should show January")
	assert.Equal(t, 31, m.calendar.Cursor.Day())

	sendKey(m, "L") // next month
	assert.Equal(t, time.February, m.calendar.Cursor.Month())
	assert.Equal(t, 28, m.calendar.Cursor.Day())
	grid := calendarGrid(*m.calendar)
	assert.Contains(t, grid, "February 2025",
		"navigating forward from Jan 31 should show February, not overflow to March")
}

func TestCalendarToday(t *testing.T) {
	t.Parallel()
	cal := &calendarState{
		Cursor: time.Date(2020, 6, 15, 0, 0, 0, 0, time.Local),
	}
	calendarToday(cal)
	assert.True(t, sameDay(cal.Cursor, time.Now()))
}

func TestCalendarTodayKeyNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	dateVal := "2020-06-15"
	confirmed := false
	m.openCalendar(&dateVal, func() { confirmed = true })
	require.NotNil(t, m.calendar)
	assert.Equal(t, 2020, m.calendar.Cursor.Year())

	// Capture now before sending keys to avoid midnight-boundary flakes.
	now := time.Now()

	// Press t to jump to today, then enter to confirm.
	sendKey(m, "t")
	assert.True(t, sameDay(m.calendar.Cursor, now),
		"pressing t should jump cursor to today")

	sendKey(m, "enter")
	assert.True(t, confirmed, "OnConfirm should have been called")
	assert.Nil(t, m.calendar, "calendar should be dismissed")
	assert.Equal(t, now.Format(data.DateLayout), dateVal,
		"confirmed date should be today")
}

func TestCalendarTodayFromParsedCursor(t *testing.T) {
	t.Parallel()
	// openCalendar parses dates via time.ParseInLocation (local).
	// Pressing "t" must land on the correct local day.
	m := newTestModel(t)
	dateVal := "2020-06-15"
	m.openCalendar(&dateVal, nil)
	require.NotNil(t, m.calendar)

	// Parsed cursor should be in local timezone.
	require.Equal(t, time.Local, m.calendar.Cursor.Location(),
		"precondition: parsed date should be local")

	sendKey(m, "t")
	now := time.Now()
	assert.Equal(t, now.Year(), m.calendar.Cursor.Year())
	assert.Equal(t, now.Month(), m.calendar.Cursor.Month())
	assert.Equal(t, now.Day(), m.calendar.Cursor.Day())
	assert.Equal(t, time.Local, m.calendar.Cursor.Location(),
		"cursor should be in local timezone after pressing t")
}

func TestOpenCalendarParsesInLocalTimezone(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	dateVal := "2026-02-15"
	m.openCalendar(&dateVal, nil)
	require.NotNil(t, m.calendar)

	assert.Equal(t, time.Local, m.calendar.Cursor.Location(),
		"parsed cursor should use local timezone")
	assert.Equal(t, time.Local, m.calendar.Selected.Location(),
		"parsed selected should use local timezone")
}

func TestOpenCalendarWithEmptyValue(t *testing.T) {
	t.Parallel()
	m := newTestModel(t)
	dateVal := ""
	m.openCalendar(&dateVal, nil)
	require.NotNil(t, m.calendar)
	assert.True(t, sameDay(m.calendar.Cursor, time.Now()))
	assert.False(t, m.calendar.HasValue)
	// Calendar overlay should be visible with the current month.
	now := time.Now()
	view := m.buildView()
	assert.Contains(t, view, now.Month().String(),
		"calendar should open to the current month")
}
