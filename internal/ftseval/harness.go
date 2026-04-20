// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package ftseval

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/llm"
)

// errorKinds in ArmResult.ErrorKind.
const (
	errorKindStage1 = "stage1_error"
	errorKindStage2 = "stage2_error"
	errorKindSQL    = "sql_error"
)

// runner wires the harness together so dependencies are explicit in tests.
type runner struct {
	cfg    Config
	store  *data.Store
	client llmStreamer
	judge  llmStreamer
	now    func() time.Time
}

// llmStreamer is the minimal contract the harness needs from an LLM
// client. The production implementation is *llm.Client; tests can drop in
// a canned-response fake.
type llmStreamer interface {
	ChatStream(ctx context.Context, messages []llm.Message) (<-chan llm.StreamChunk, error)
}

// Run executes every question in cfg.Questions (or DefaultQuestions if
// empty), returns the per-question RunResults, and leaves any reporting
// to the caller.
func Run(
	ctx context.Context,
	cfg Config,
	store *data.Store,
	fixture SeededFixture,
	client llmStreamer,
	judge llmStreamer,
) ([]RunResult, error) {
	r := &runner{
		cfg:    cfg,
		store:  store,
		client: client,
		judge:  judge,
		now:    time.Now,
	}
	if r.judge == nil {
		r.judge = client // fallback: use the same client for grading
	}

	all := DefaultQuestions(fixture)
	questions := FilterQuestions(all, cfg.Questions)
	if len(questions) == 0 {
		return nil, fmt.Errorf(
			"no questions to run; DefaultQuestions returned %d, --questions=%v matched none",
			len(all),
			cfg.Questions,
		)
	}

	out := make([]RunResult, 0, len(questions))
	for _, q := range questions {
		res := r.runQuestion(ctx, q)
		out = append(out, res)
	}
	return out, nil
}

// runQuestion drives one question through both FTS arms (or just one when
// NoAB is set). Stage errors are recorded on the arm rather than aborting
// the whole run.
func (r *runner) runQuestion(ctx context.Context, q Question) RunResult {
	res := RunResult{Question: q}
	res.FTSOn = r.runArm(ctx, q, true /*useFTS*/)
	if !r.cfg.NoAB {
		res.FTSOff = r.runArm(ctx, q, false /*useFTS*/)
	}
	return res
}

// runArm produces the SQL + summary for one arm and grades them.
func (r *runner) runArm(ctx context.Context, q Question, useFTS bool) ArmResult {
	start := time.Now()
	arm := ArmResult{Grade: GradeResult{JudgeScore: -1}}
	defer func() { arm.DurationMS = time.Since(start).Milliseconds() }()

	tables := llm.BuildTableInfo(r.store)
	columnHints := ""
	if r.store != nil {
		columnHints = r.store.ColumnHints()
	}

	ftsContext := ""
	if useFTS {
		ftsContext = llm.BuildFTSContextFromStore(r.store, q.Query)
	}

	// Stage 1: NL -> SQL.
	sqlPrompt := llm.BuildSQLPrompt(tables, r.now(), columnHints, ftsContext, "")
	messages := []llm.Message{
		{Role: "system", Content: sqlPrompt},
		{Role: "user", Content: q.Query},
	}
	sqlText, err := r.stream(ctx, r.client, messages)
	if err != nil {
		arm.ErrorKind = errorKindStage1
		arm.ErrorMsg = err.Error()
		r.applyRubrics(&arm, q)
		r.applyEntityHit(&arm, q, ftsContext)
		return arm
	}
	arm.GeneratedSQL = strings.TrimSpace(stripFences(sqlText))

	// SQL execution.
	summaryInput := arm.GeneratedSQL
	var resultsTable string
	if arm.GeneratedSQL == "" {
		arm.ErrorKind = errorKindSQL
		arm.ErrorMsg = "empty SQL from stage 1"
		resultsTable = "no SQL produced"
	} else {
		cols, rows, execErr := r.store.ReadOnlyQuery(ctx, arm.GeneratedSQL)
		if execErr != nil {
			arm.ErrorKind = errorKindSQL
			arm.ErrorMsg = execErr.Error()
			resultsTable = "query error: " + execErr.Error()
		} else {
			resultsTable = llm.FormatResultsTable(cols, rows)
		}
	}

	// Stage 2: summary.
	summaryPrompt := llm.BuildSummaryPrompt(
		q.Query,
		summaryInput,
		resultsTable,
		r.now(),
		ftsContext,
		"",
	)
	sumMessages := []llm.Message{
		{Role: "system", Content: summaryPrompt},
		{Role: "user", Content: q.Query},
	}
	sumText, err := r.stream(ctx, r.client, sumMessages)
	if err != nil {
		arm.ErrorKind = errorKindStage2
		arm.ErrorMsg = err.Error()
		r.applyRubrics(&arm, q)
		r.applyEntityHit(&arm, q, ftsContext)
		return arm
	}
	arm.SummaryText = strings.TrimSpace(sumText)

	r.applyRubrics(&arm, q)
	r.applyEntityHit(&arm, q, ftsContext)

	// Judge: only runs when a non-empty summary is available and --skip-judge
	// is off.
	if !r.cfg.SkipJudge && arm.SummaryText != "" {
		r.runJudge(ctx, &arm, q)
	}

	return arm
}

// stream consumes a single ChatStream and concatenates content chunks into
// one text blob. Errors from the stream surface as the final error.
func (r *runner) stream(
	ctx context.Context,
	c llmStreamer,
	messages []llm.Message,
) (string, error) {
	stageCtx, cancel := context.WithTimeout(ctx, r.effectiveTimeout())
	defer cancel()

	ch, err := c.ChatStream(stageCtx, messages)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return b.String(), chunk.Err
		}
		b.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}
	return b.String(), nil
}

func (r *runner) effectiveTimeout() time.Duration {
	if r.cfg.Timeout > 0 {
		return r.cfg.Timeout
	}
	return 60 * time.Second
}

// stripFences removes ```sql ... ``` or ``` ... ``` wrappers the model
// often adds around generated SQL.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```sql")
	s = strings.TrimPrefix(s, "```SQL")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// applyRubrics scores the regex-based portion of the grade.
func (r *runner) applyRubrics(arm *ArmResult, q Question) {
	total := len(q.RubricSQL) + len(q.RubricSummary)
	hit := 0
	for _, re := range q.RubricSQL {
		if re.MatchString(arm.GeneratedSQL) {
			hit++
		}
	}
	for _, re := range q.RubricSummary {
		if re.MatchString(arm.SummaryText) {
			hit++
		}
	}
	arm.Grade.Rubric = hit
	arm.Grade.RubricTotal = total
}

// applyEntityHit checks which ExpectedEntityIDs surfaced in the injected
// FTS context. The context format is "Project \"Kitchen Remodel\" (id: <ID>)",
// so a literal substring match on "id: <ID>" is precise enough without
// coupling to the exact summary format. Empty IDs are skipped — they show
// up when --db is used (so the SeededFixture is zero-valued) and would
// otherwise false-positive against any "id: " substring.
func (r *runner) applyEntityHit(arm *ArmResult, q Question, ftsContext string) {
	var nonEmpty []string
	for _, id := range q.ExpectedEntityIDs {
		if id != "" {
			nonEmpty = append(nonEmpty, id)
		}
	}
	arm.Grade.EntitiesTotal = len(nonEmpty)
	if ftsContext == "" || len(nonEmpty) == 0 {
		return
	}
	hit := 0
	for _, id := range nonEmpty {
		if strings.Contains(ftsContext, "id: "+id) {
			hit++
		}
	}
	arm.Grade.EntitiesHit = hit
}

// runJudge makes one LLM call asking the judge model to score C1..C5 on a
// 0/1 each, summed to 0-5.
func (r *runner) runJudge(ctx context.Context, arm *ArmResult, q Question) {
	judgePrompt := fmt.Sprintf(
		`You are grading an AI assistant's answer to a user's question about their home-management database.

Score the answer on FIVE criteria, each 0 or 1:
C1: Does the answer directly address the question?
C2: Is the answer grounded in the SQL result (no hallucinated facts)?
C3: Are entity names correct and disambiguated?
C4: Is the SQL a reasonable query for the question?
C5: Is the answer free of irrelevant content?

Extra context for grading: %s

Reply with each score on its own line in the format "Cn=0" or "Cn=1"
(n from 1 to 5), followed by a single line "Reason: <one short sentence>".
Think first if you want, but the grading lines must be present.`,
		q.JudgePrompt,
	)

	user := fmt.Sprintf("Question: %s\n\nGenerated SQL:\n%s\n\nAnswer:\n%s",
		q.Query, arm.GeneratedSQL, arm.SummaryText)

	messages := []llm.Message{
		{Role: "system", Content: judgePrompt},
		{Role: "user", Content: user},
	}
	reply, err := r.stream(ctx, r.judge, messages)
	if err != nil {
		arm.Grade.JudgeReason = "judge_error: " + err.Error()
		return
	}
	score, reason := parseJudgeReply(reply)
	if score < 0 {
		arm.Grade.JudgeReason = "judge_parse_failed: " + truncateReply(reply)
		return
	}
	arm.Grade.JudgeScore = score
	arm.Grade.JudgeReason = reason
}

// judgeScoreRE matches one criterion line. Tolerates:
//   - separator variants: `=`, `:`, `  =  `, ` : `
//   - markdown decoration around the criterion name or score:
//     `**C1**=1`, `- C1 = 1`, `**C1**=**1**`
//   - case: `c1`, `C1`
//
// The `\** \s*` pair on each side of the separator absorbs `**`
// (markdown bold), leading/trailing whitespace, and stray underscores
// common in model output.
var judgeScoreRE = regexp.MustCompile(`(?i)\bc\s*([1-5])\s*\**\s*[:=]\s*\**\s*([01])\b`)

// judgeReasonRE matches the reason line. Accepts "Reason:", "reason =",
// "Rationale:" etc. (?i) for case; (?s) deliberately NOT set so the
// capture stops at the first newline.
var judgeReasonRE = regexp.MustCompile(`(?i)\b(?:reason|rationale)\b\s*[:=]\s*(.+)`)

// judgeThinkREs strip reasoning-model preambles. Qwen3, DeepSeek-R1,
// and similar models emit `<think>…</think>`, `<thinking>…</thinking>`,
// or `<reasoning>…</reasoning>` before the actual answer. RE2 has no
// backreferences, so we keep one regex per tag name.
var judgeThinkREs = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<think>.*?</think>`),
	regexp.MustCompile(`(?is)<thinking>.*?</thinking>`),
	regexp.MustCompile(`(?is)<reasoning>.*?</reasoning>`),
}

// parseJudgeReply extracts C1..C5 scores and an optional reason from a
// judge-model reply. Returns (-1, "") when any of the five criteria are
// missing. Tolerant of formatting variation: reasoning preambles,
// markdown decoration, `:` vs `=` separators, mixed case, and stray
// whitespace all work.
func parseJudgeReply(reply string) (int, string) {
	for _, re := range judgeThinkREs {
		reply = re.ReplaceAllString(reply, "")
	}

	matches := judgeScoreRE.FindAllStringSubmatch(reply, -1)
	byCriterion := map[int]int{}
	for _, m := range matches {
		idx, err := strconv.Atoi(m[1])
		if err != nil || idx < 1 || idx > 5 {
			continue
		}
		val, err := strconv.Atoi(m[2])
		if err != nil || (val != 0 && val != 1) {
			continue
		}
		// First match wins so a model restating the criteria mid-reply
		// can't override its final verdict.
		if _, seen := byCriterion[idx]; !seen {
			byCriterion[idx] = val
		}
	}
	if len(byCriterion) != 5 {
		return -1, ""
	}
	score := 0
	for i := 1; i <= 5; i++ {
		score += byCriterion[i]
	}

	reason := ""
	if m := judgeReasonRE.FindStringSubmatch(reply); len(m) >= 2 {
		// Strip markdown decoration (** _ *) and whitespace so "Reason:
		// **mostly there**" normalizes to "mostly there".
		reason = strings.Trim(m[1], " \t*_")
	}
	return score, reason
}

// truncateReply keeps the last chunk of a judge reply so parse-failure
// reasons surface something recognizable in the report without flooding
// it. Prefers the tail because reasoning models put the verdict at the
// end; the leading <think> block is rarely informative about why the
// parse failed.
func truncateReply(reply string) string {
	reply = strings.TrimSpace(reply)
	const maxLen = 200
	if len(reply) <= maxLen {
		return reply
	}
	return "..." + reply[len(reply)-maxLen:]
}

// ExitCode derives a process exit code from the results under cfg.Strict.
// Returns 0 when nothing regressed and 1 on a per-question rubric
// regression (FTS-on rubric strictly less than FTS-off rubric) among
// questions that completed on both arms.
func ExitCode(cfg Config, results []RunResult) int {
	if !cfg.Strict || cfg.NoAB {
		return 0
	}
	for _, r := range results {
		if !r.Completed(cfg.NoAB) {
			continue
		}
		if r.FTSOn.Grade.Rubric < r.FTSOff.Grade.Rubric {
			return 1
		}
	}
	return 0
}
