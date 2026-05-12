// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"math"
	"time"
)

// Compressed-duration labels: durationNow for sub-minute spans, durationToday
// for a zero-day span.
const (
	durationNow   = "now"
	durationToday = "today"
)

// DateDiffDays returns the number of calendar days from now to target,
// using each time's local Y/M/D. Positive means target is in the future.
func DateDiffDays(now, target time.Time) int {
	nowDate := time.Date(
		now.Year(), now.Month(), now.Day(),
		0, 0, 0, 0, time.UTC,
	)
	tgtDate := time.Date(
		target.Year(), target.Month(), target.Day(),
		0, 0, 0, 0, time.UTC,
	)
	return int(math.Round(tgtDate.Sub(nowDate).Hours() / 24))
}

// ShortDur returns a compressed duration string like "3d", "2mo", "1y".
func ShortDur(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return durationNow
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

// DaysText returns a bare compressed duration like "5d" or "today".
func DaysText(days int) string {
	if days == 0 {
		return durationToday
	}
	abs := days
	if abs < 0 {
		abs = -abs
	}
	return ShortDur(time.Duration(abs) * 24 * time.Hour)
}

// PastDur returns a compressed past-duration string. Sub-minute is "<1m".
func PastDur(d time.Duration) string {
	s := ShortDur(d)
	if s == durationNow {
		return "<1m"
	}
	return s
}
