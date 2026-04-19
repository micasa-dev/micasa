// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Package ftseval drives a chat-quality benchmark against either the
// embedded fixture database or a user-supplied micasa SQLite file. It
// reproduces the chat pipeline's prompt-building and LLM invocation
// outside the TUI, grades each run with a deterministic rubric (and
// optionally an LLM judge), and reports FTS-on vs FTS-off deltas.
package ftseval

import (
	"regexp"
	"time"
)

// Config holds every knob the harness exposes. Zero values are the
// defaults used when the CLI flag isn't supplied.
type Config struct {
	// DBPath selects the database to evaluate against. Empty means the
	// embedded fixture; any other value is treated as an existing
	// micasa SQLite file to open read-only.
	DBPath string

	Provider   string
	Model      string
	JudgeModel string
	APIKey     string

	// Timeout applies to every LLM call (per stage, per arm, per question).
	Timeout time.Duration

	// Questions restricts the run to the named questions. Empty means
	// "run all questions in DefaultQuestions()".
	Questions []string

	SkipJudge bool
	NoAB      bool

	Format string // "markdown" or "json"
	Strict bool
}

// Question describes one NL-question benchmark case. Rubrics are regexps
// that must all match the corresponding text; partial misses cost one
// rubric point each.
type Question struct {
	Name              string
	Query             string
	RubricSQL         []*regexp.Regexp
	RubricSummary     []*regexp.Regexp
	ExpectedEntityIDs []string
	JudgePrompt       string
}

// ArmResult captures everything produced by running one arm (FTS-on or
// FTS-off) of a question: the generated SQL, the summary, any error, and
// the per-question grading.
type ArmResult struct {
	GeneratedSQL string
	SummaryText  string
	ErrorKind    string // empty when the arm ran to completion
	ErrorMsg     string // human-readable error detail
	DurationMS   int64
	Grade        GradeResult
}

// GradeResult combines rubric and judge signals for one arm.
type GradeResult struct {
	Rubric        int
	RubricTotal   int
	JudgeScore    int    // 0-5 when the judge ran; -1 when it did not.
	JudgeReason   string // one-line rationale; empty when not run.
	EntitiesHit   int    // how many ExpectedEntityIDs surfaced via FTS.
	EntitiesTotal int
}

// RunResult is one question's complete result across both arms.
type RunResult struct {
	Question Question
	FTSOn    ArmResult
	FTSOff   ArmResult // zero-value if NoAB was requested.
}

// Completed reports whether both arms produced a Stage-2 summary (the
// definition used for aggregate and --strict gating).
func (r *RunResult) Completed(noAB bool) bool {
	if !r.completeArm(r.FTSOn) {
		return false
	}
	if noAB {
		return true
	}
	return r.completeArm(r.FTSOff)
}

func (r *RunResult) completeArm(a ArmResult) bool {
	return a.ErrorKind == "" || a.ErrorKind == "sql_error"
}
