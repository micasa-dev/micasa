// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package ftseval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/micasa-dev/micasa/internal/termio"
)

// WriteReport renders the run results according to cfg.Format:
//
//   - "json"     — machine-readable output with APIKey stripped.
//   - "markdown" — GitHub-flavored markdown; good for piping to files
//     or pasting into PR descriptions.
//   - "table"    — styled terminal table using lipgloss. Default when
//     stdout is a TTY. Use this when reading the report live.
//
// Unknown values fall back to "table".
func WriteReport(w io.Writer, cfg Config, results []RunResult) error {
	switch strings.ToLower(cfg.Format) {
	case "json":
		return writeJSONReport(w, cfg, results)
	case "markdown", "md":
		return writeMarkdownReport(w, cfg, results)
	default:
		return writeTableReport(w, cfg, results)
	}
}

// redactedConfig mirrors Config for JSON output with sensitive fields
// stripped. `APIKey` in particular must never appear in reports — the
// report may be printed to stdout, written to --output, or committed as
// a CI artifact.
type redactedConfig struct {
	DBPath     string        `json:"db_path"`
	Provider   string        `json:"provider"`
	Model      string        `json:"model"`
	JudgeModel string        `json:"judge_model"`
	Timeout    time.Duration `json:"timeout"`
	Questions  []string      `json:"questions,omitempty"`
	SkipJudge  bool          `json:"skip_judge"`
	NoAB       bool          `json:"no_ab"`
	Format     string        `json:"format"`
	Strict     bool          `json:"strict"`
}

func redact(cfg Config) redactedConfig {
	return redactedConfig{
		DBPath:     cfg.DBPath,
		Provider:   cfg.Provider,
		Model:      cfg.Model,
		JudgeModel: cfg.JudgeModel,
		Timeout:    cfg.Timeout,
		Questions:  cfg.Questions,
		SkipJudge:  cfg.SkipJudge,
		NoAB:       cfg.NoAB,
		Format:     cfg.Format,
		Strict:     cfg.Strict,
	}
}

func writeJSONReport(w io.Writer, cfg Config, results []RunResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"config":  redact(cfg),
		"results": results,
	}); err != nil {
		return fmt.Errorf("encode json report: %w", err)
	}
	return nil
}

func writeMarkdownReport(w io.Writer, cfg Config, results []RunResult) error {
	var b strings.Builder
	b.WriteString("# FTS Eval Report\n\n")
	fmt.Fprintf(&b, "Provider: `%s`  Model: `%s`  Judge: `%s`  SkipJudge: %v  NoAB: %v\n\n",
		defaultStr(cfg.Provider, "(default)"),
		defaultStr(cfg.Model, "(default)"),
		defaultStr(cfg.JudgeModel, "(same as model)"),
		cfg.SkipJudge, cfg.NoAB,
	)

	b.WriteString(
		"| Question | FTS off rubric | FTS off judge | FTS on rubric | FTS on judge | Δ judge | Notes |\n",
	)
	b.WriteString(
		"|----------|----------------|---------------|---------------|--------------|--------|-------|\n",
	)

	var sumRubricOn, sumRubricOff, sumRubricTotalOn, sumRubricTotalOff int
	var judgeSumOn, judgeSumOff, judgeCountOn, judgeCountOff int
	incomplete := []string{}

	for _, r := range results {
		if !r.Completed(cfg.NoAB) {
			incomplete = append(incomplete, r.Question.Name)
		}
		offRubric := formatRubric(r.FTSOff, cfg.NoAB)
		onRubric := formatRubric(r.FTSOn, false)
		offJudge := formatJudge(r.FTSOff, cfg.NoAB)
		onJudge := formatJudge(r.FTSOn, false)
		deltaJudge := formatJudgeDelta(r, cfg.NoAB)
		notes := formatNotes(r, cfg.NoAB)
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			r.Question.Name, offRubric, offJudge, onRubric, onJudge, deltaJudge, notes,
		)

		sumRubricOn += r.FTSOn.Grade.Rubric
		sumRubricTotalOn += r.FTSOn.Grade.RubricTotal
		if !cfg.NoAB {
			sumRubricOff += r.FTSOff.Grade.Rubric
			sumRubricTotalOff += r.FTSOff.Grade.RubricTotal
		}
		if r.FTSOn.Grade.JudgeScore >= 0 {
			judgeSumOn += r.FTSOn.Grade.JudgeScore
			judgeCountOn++
		}
		if !cfg.NoAB && r.FTSOff.Grade.JudgeScore >= 0 {
			judgeSumOff += r.FTSOff.Grade.JudgeScore
			judgeCountOff++
		}
	}

	b.WriteString("\n## Aggregate\n\n")
	if sumRubricTotalOn > 0 {
		fmt.Fprintf(&b, "- FTS-on rubric pass rate: %d/%d\n", sumRubricOn, sumRubricTotalOn)
	}
	if !cfg.NoAB && sumRubricTotalOff > 0 {
		fmt.Fprintf(&b, "- FTS-off rubric pass rate: %d/%d\n", sumRubricOff, sumRubricTotalOff)
	}
	if judgeCountOn > 0 {
		fmt.Fprintf(&b, "- FTS-on mean judge score: %.2f (n=%d)\n",
			float64(judgeSumOn)/float64(judgeCountOn), judgeCountOn)
	}
	if !cfg.NoAB && judgeCountOff > 0 {
		fmt.Fprintf(&b, "- FTS-off mean judge score: %.2f (n=%d)\n",
			float64(judgeSumOff)/float64(judgeCountOff), judgeCountOff)
	}
	if len(incomplete) > 0 {
		fmt.Fprintf(&b, "- Incomplete questions (excluded from deltas): %s\n",
			strings.Join(incomplete, ", "))
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func formatRubric(a ArmResult, skipped bool) string {
	if skipped {
		return "-"
	}
	return fmt.Sprintf("%d/%d", a.Grade.Rubric, a.Grade.RubricTotal)
}

func formatJudge(a ArmResult, skipped bool) string {
	if skipped || a.Grade.JudgeScore < 0 {
		return "-"
	}
	return fmt.Sprintf("%d/5", a.Grade.JudgeScore)
}

func formatJudgeDelta(r RunResult, noAB bool) string {
	if noAB || r.FTSOn.Grade.JudgeScore < 0 || r.FTSOff.Grade.JudgeScore < 0 {
		return "-"
	}
	d := r.FTSOn.Grade.JudgeScore - r.FTSOff.Grade.JudgeScore
	return fmt.Sprintf("%+d", d)
}

func formatNotes(r RunResult, noAB bool) string {
	var parts []string
	if r.FTSOn.ErrorKind != "" {
		parts = append(parts, "on="+r.FTSOn.ErrorKind)
	}
	if !noAB && r.FTSOff.ErrorKind != "" {
		parts = append(parts, "off="+r.FTSOff.ErrorKind)
	}
	if r.Question.ExpectedEntityIDs != nil {
		parts = append(parts, fmt.Sprintf("entities=%d/%d",
			r.FTSOn.Grade.EntitiesHit, r.FTSOn.Grade.EntitiesTotal))
	}
	// Surface the first judge_reason we have so --skip-judge=false runs
	// with judge==- in the table aren't mystery failures. Prefer FTS-on
	// since FTS-off without --no-ab usually has the same root cause.
	if msg := firstJudgeIssue(r, noAB); msg != "" {
		parts = append(parts, msg)
	}
	return strings.Join(parts, "; ")
}

// firstJudgeIssue returns a short tag describing why the judge didn't
// produce a score, or "" when the judge ran normally (or --skip-judge
// was effectively in force on both arms). Only diagnostic messages are
// surfaced — successful judge reasons live in the JSON report and
// would clutter the table.
func firstJudgeIssue(r RunResult, noAB bool) string {
	pick := func(a ArmResult, arm string) string {
		if a.Grade.JudgeScore >= 0 {
			return ""
		}
		reason := a.Grade.JudgeReason
		if reason == "" {
			return ""
		}
		// Snip to a short tag so the notes column stays readable.
		const maxLen = 50
		if len(reason) > maxLen {
			reason = reason[:maxLen] + "..."
		}
		return arm + ": " + reason
	}
	if msg := pick(r.FTSOn, "on"); msg != "" {
		return msg
	}
	if !noAB {
		if msg := pick(r.FTSOff, "off"); msg != "" {
			return msg
		}
	}
	return ""
}

func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// detectDarkBG asks lipgloss whether the terminal has a dark background,
// but only when the report is actually being written to a terminal. Tests,
// pipes, and files use a dark default without hitting lipgloss -- its
// terminal query hangs or panics on Windows CI when stdin is redirected.
// The recover() guards against any remaining panic from a live TTY on a
// hostile platform.
func detectDarkBG(w io.Writer) (dark bool) {
	dark = true
	if !termio.IsTerminal(w) {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			dark = true
		}
	}()
	dark = lipgloss.HasDarkBackground(os.Stdin, os.Stderr)
	return
}

// writeTableReport renders the run results as a styled terminal table.
// Colors follow the Wong palette used across the project so the eval's
// output reads the same as `micasa status`.
func writeTableReport(w io.Writer, cfg Config, results []RunResult) error {
	styles := evalStyles(detectDarkBG(w))
	var b strings.Builder

	b.WriteString(styles.heading.Render("FTS EVAL RESULTS"))
	b.WriteString("\n\n")

	// Config banner.
	kv := func(k, v string) string {
		return styles.key.Render(k) + " " + styles.value.Render(v)
	}
	b.WriteString(kv("provider", defaultStr(cfg.Provider, "(default)")))
	b.WriteString("   ")
	b.WriteString(kv("model", defaultStr(cfg.Model, "(default)")))
	b.WriteString("   ")
	b.WriteString(kv("judge", defaultStr(cfg.JudgeModel, "(same as model)")))
	if cfg.SkipJudge {
		b.WriteString("   ")
		b.WriteString(styles.muted.Render("[skip-judge]"))
	}
	if cfg.NoAB {
		b.WriteString("   ")
		b.WriteString(styles.muted.Render("[no-ab]"))
	}
	b.WriteString("\n\n")

	tbl := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(styles.border).
		Headers(tableHeaders(cfg.NoAB)...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return styles.tableHeader
			}
			// Colorize the delta column.
			deltaCol := deltaColumnIndex(cfg.NoAB)
			if col == deltaCol && row >= 0 && row < len(results) {
				d := judgeDelta(results[row], cfg.NoAB)
				switch {
				case d > 0:
					return styles.success
				case d < 0:
					return styles.danger
				}
			}
			// Error notes in rose.
			if col == len(tableHeaders(cfg.NoAB))-1 && row >= 0 && row < len(results) {
				r := results[row]
				if r.FTSOn.ErrorKind != "" || (!cfg.NoAB && r.FTSOff.ErrorKind != "") {
					return styles.warning
				}
			}
			return lipgloss.NewStyle()
		})

	var sumRubricOn, sumRubricOff, sumRubricTotalOn, sumRubricTotalOff int
	var judgeSumOn, judgeSumOff, judgeCountOn, judgeCountOff int
	incomplete := []string{}

	for _, r := range results {
		if !r.Completed(cfg.NoAB) {
			incomplete = append(incomplete, r.Question.Name)
		}
		tbl.Row(tableRow(r, cfg.NoAB)...)

		sumRubricOn += r.FTSOn.Grade.Rubric
		sumRubricTotalOn += r.FTSOn.Grade.RubricTotal
		if !cfg.NoAB {
			sumRubricOff += r.FTSOff.Grade.Rubric
			sumRubricTotalOff += r.FTSOff.Grade.RubricTotal
		}
		if r.FTSOn.Grade.JudgeScore >= 0 {
			judgeSumOn += r.FTSOn.Grade.JudgeScore
			judgeCountOn++
		}
		if !cfg.NoAB && r.FTSOff.Grade.JudgeScore >= 0 {
			judgeSumOff += r.FTSOff.Grade.JudgeScore
			judgeCountOff++
		}
	}

	b.WriteString(tbl.String())
	b.WriteString("\n\n")

	// Aggregate block.
	b.WriteString(styles.heading.Render("AGGREGATE"))
	b.WriteString("\n")
	if sumRubricTotalOn > 0 {
		b.WriteString("  " + kv("FTS on  rubric",
			fmt.Sprintf("%d/%d", sumRubricOn, sumRubricTotalOn)))
		b.WriteString("\n")
	}
	if !cfg.NoAB && sumRubricTotalOff > 0 {
		b.WriteString("  " + kv("FTS off rubric",
			fmt.Sprintf("%d/%d", sumRubricOff, sumRubricTotalOff)))
		b.WriteString("\n")
	}
	if judgeCountOn > 0 {
		b.WriteString("  " + kv("FTS on  judge mean",
			fmt.Sprintf("%.2f  (n=%d)", float64(judgeSumOn)/float64(judgeCountOn), judgeCountOn)))
		b.WriteString("\n")
	}
	if !cfg.NoAB && judgeCountOff > 0 {
		b.WriteString("  " + kv(
			"FTS off judge mean",
			fmt.Sprintf(
				"%.2f  (n=%d)",
				float64(judgeSumOff)/float64(judgeCountOff),
				judgeCountOff,
			),
		))
		b.WriteString("\n")
	}
	if len(incomplete) > 0 {
		b.WriteString("  " + styles.warning.Render(
			"incomplete: "+strings.Join(incomplete, ", ")))
		b.WriteString("\n")
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func tableHeaders(noAB bool) []string {
	if noAB {
		return []string{"QUESTION", "RUBRIC", "JUDGE", "NOTES"}
	}
	return []string{
		"QUESTION",
		"OFF RUBRIC", "OFF JUDGE",
		"ON RUBRIC", "ON JUDGE",
		"Δ JUDGE",
		"NOTES",
	}
}

func deltaColumnIndex(noAB bool) int {
	if noAB {
		return -1 // no delta column in no-ab mode
	}
	return 5
}

func tableRow(r RunResult, noAB bool) []string {
	if noAB {
		return []string{
			r.Question.Name,
			formatRubric(r.FTSOn, false),
			formatJudge(r.FTSOn, false),
			formatNotes(r, true),
		}
	}
	return []string{
		r.Question.Name,
		formatRubric(r.FTSOff, false),
		formatJudge(r.FTSOff, false),
		formatRubric(r.FTSOn, false),
		formatJudge(r.FTSOn, false),
		formatJudgeDelta(r, false),
		formatNotes(r, false),
	}
}

// judgeDelta returns the numeric FTS-on-minus-FTS-off delta, or 0 when
// either arm doesn't have a judge score (the delta column shows "-" in
// that case and isn't colorized).
func judgeDelta(r RunResult, noAB bool) int {
	if noAB || r.FTSOn.Grade.JudgeScore < 0 || r.FTSOff.Grade.JudgeScore < 0 {
		return 0
	}
	return r.FTSOn.Grade.JudgeScore - r.FTSOff.Grade.JudgeScore
}

// evalStyles bundles the lipgloss styles used by the table report. Kept
// in this package so ftseval can render a rich terminal report without
// pulling cmd/micasa's cliStyles in (reverse import).
type evalStylesSet struct {
	heading     lipgloss.Style
	tableHeader lipgloss.Style
	border      lipgloss.Style
	key         lipgloss.Style
	value       lipgloss.Style
	success     lipgloss.Style
	warning     lipgloss.Style
	danger      lipgloss.Style
	muted       lipgloss.Style
}

func evalStyles(isDark bool) evalStylesSet {
	c := lipgloss.LightDark(isDark)
	blue := c(lipgloss.Color("#0072B2"), lipgloss.Color("#56B4E9"))
	orange := c(lipgloss.Color("#D55E00"), lipgloss.Color("#E69F00"))
	green := c(lipgloss.Color("#007A5A"), lipgloss.Color("#009E73"))
	vermillion := c(lipgloss.Color("#CC3311"), lipgloss.Color("#D55E00"))
	rose := c(lipgloss.Color("#AA4499"), lipgloss.Color("#CC79A7"))
	dim := c(lipgloss.Color("#4B5563"), lipgloss.Color("#6B7280"))
	border := c(lipgloss.Color("#D1D5DB"), lipgloss.Color("#374151"))
	return evalStylesSet{
		heading:     lipgloss.NewStyle().Bold(true).Foreground(blue),
		tableHeader: lipgloss.NewStyle().Bold(true).Foreground(dim),
		border:      lipgloss.NewStyle().Foreground(border),
		key:         lipgloss.NewStyle().Foreground(dim),
		value:       lipgloss.NewStyle().Bold(true),
		success:     lipgloss.NewStyle().Foreground(green),
		warning:     lipgloss.NewStyle().Foreground(orange),
		danger:      lipgloss.NewStyle().Foreground(vermillion),
		muted:       lipgloss.NewStyle().Foreground(rose),
	}
}
