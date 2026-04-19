// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package ftseval

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseJudgeReplyHappyPath(t *testing.T) {
	t.Parallel()
	score, reason := parseJudgeReply("C1=1 C2=1 C3=0 C4=1 C5=1\nReason: looks good")
	assert.Equal(t, 4, score)
	assert.Equal(t, "looks good", reason)
}

func TestParseJudgeReplyMissingTag(t *testing.T) {
	t.Parallel()
	score, _ := parseJudgeReply("C1=1 C2=1 C3=0")
	assert.Equal(t, -1, score, "missing C4/C5 must produce sentinel -1")
}

func TestParseJudgeReplyInvalidDigit(t *testing.T) {
	t.Parallel()
	// 9 isn't a valid criterion score; the regex rejects it via [01].
	// The remaining four criteria aren't enough to reach 5, so sentinel.
	score, _ := parseJudgeReply("C1=9 C2=1 C3=0 C4=1 C5=1")
	assert.Equal(t, -1, score, "non-binary digit must produce sentinel -1")
}

func TestParseJudgeReplyColonSeparator(t *testing.T) {
	t.Parallel()
	score, reason := parseJudgeReply("C1: 1\nC2: 1\nC3: 0\nC4: 1\nC5: 0\nReason: mostly there")
	assert.Equal(t, 3, score)
	assert.Equal(t, "mostly there", reason)
}

func TestParseJudgeReplyMarkdownDecoration(t *testing.T) {
	t.Parallel()
	score, reason := parseJudgeReply(`- **C1** = 1
- **C2** = 0
- **C3** = 1
- **C4** = 1
- **C5** = 0

**Reason:** entity naming was off`)
	assert.Equal(t, 3, score)
	assert.Equal(t, "entity naming was off", reason)
}

func TestParseJudgeReplyStripsThinkBlock(t *testing.T) {
	t.Parallel()
	// qwen3 / deepseek-r1 emit <think>...</think> preambles. The
	// parser must ignore them even when they contain strings that
	// look like scores (the model might analyze "C1").
	reply := `<think>
Let me analyze C1: the answer addresses the question. I should give it 1.
Similarly C2 through C5... I'll check each.
</think>

C1=1 C2=1 C3=1 C4=0 C5=1
Reason: SQL was awkward but correct`
	score, reason := parseJudgeReply(reply)
	assert.Equal(t, 4, score)
	assert.Equal(t, "SQL was awkward but correct", reason)
}

func TestParseJudgeReplyFirstMatchWins(t *testing.T) {
	t.Parallel()
	// When a model restates the criteria list before scoring (common
	// pattern), the SECOND mention is the verdict. The parser should
	// take the first match per criterion instead — restated criteria
	// often come with no score attached.
	reply := `Restating criteria: C1: ?, C2: ?, C3: ?, C4: ?, C5: ?
Grading:
C1=1 C2=1 C3=0 C4=0 C5=1
Reason: partial credit`
	score, _ := parseJudgeReply(reply)
	// The "?" lines don't match [01] so they're ignored; only the
	// grading lines count. Score = 1+1+0+0+1 = 3.
	assert.Equal(t, 3, score)
}

func TestParseJudgeReplyRationaleKeyword(t *testing.T) {
	t.Parallel()
	score, reason := parseJudgeReply("C1=1 C2=1 C3=1 C4=1 C5=1\nRationale: perfect")
	assert.Equal(t, 5, score)
	assert.Equal(t, "perfect", reason)
}

func TestParseJudgeReplyWithReasonButNoScores(t *testing.T) {
	t.Parallel()
	// Only partial scores — must be treated as unparseable even
	// though a reason is present.
	score, reason := parseJudgeReply("C1=1 C2=1\nReason: incomplete")
	assert.Equal(t, -1, score)
	assert.Equal(t, "", reason)
}

func TestTruncateReplyShort(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "short reply", truncateReply("  short reply  "))
}

func TestTruncateReplyLong(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 250) + "THE VERDICT"
	got := truncateReply(long)
	assert.True(t, strings.HasSuffix(got, "THE VERDICT"),
		"truncation must keep the tail (where the verdict lives)")
	assert.True(t, strings.HasPrefix(got, "..."),
		"truncation must mark the leading cut")
}

func TestStripFences(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"SELECT * FROM t", "SELECT * FROM t"},
		{"```sql\nSELECT 1\n```", "SELECT 1"},
		{"```SELECT 1```", "SELECT 1"},
		{"  ```\nSELECT 1\n```  ", "SELECT 1"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, stripFences(tc.in), "input %q", tc.in)
	}
}

func TestFilterQuestions(t *testing.T) {
	t.Parallel()
	all := []Question{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	assert.Equal(t, all, FilterQuestions(all, nil))
	got := FilterQuestions(all, []string{"a", "c"})
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].Name)
	assert.Equal(t, "c", got[1].Name)
}

func TestExitCodeStrictRegression(t *testing.T) {
	t.Parallel()
	cfg := Config{Strict: true}
	results := []RunResult{{
		Question: Question{Name: "q"},
		FTSOn:    ArmResult{Grade: GradeResult{Rubric: 1, RubricTotal: 3, JudgeScore: -1}},
		FTSOff:   ArmResult{Grade: GradeResult{Rubric: 2, RubricTotal: 3, JudgeScore: -1}},
	}}
	assert.Equal(t, 1, ExitCode(cfg, results), "FTS-on rubric < FTS-off rubric must exit 1")
}

func TestExitCodeStrictOKWhenOnMatchesOff(t *testing.T) {
	t.Parallel()
	cfg := Config{Strict: true}
	results := []RunResult{{
		Question: Question{Name: "q"},
		FTSOn:    ArmResult{Grade: GradeResult{Rubric: 2, RubricTotal: 3, JudgeScore: -1}},
		FTSOff:   ArmResult{Grade: GradeResult{Rubric: 2, RubricTotal: 3, JudgeScore: -1}},
	}}
	assert.Equal(t, 0, ExitCode(cfg, results))
}

func TestExitCodeStrictIgnoresIncomplete(t *testing.T) {
	t.Parallel()
	cfg := Config{Strict: true}
	results := []RunResult{{
		Question: Question{Name: "q"},
		FTSOn: ArmResult{
			ErrorKind: errorKindStage1,
			Grade:     GradeResult{Rubric: 0, RubricTotal: 3, JudgeScore: -1},
		},
		FTSOff: ArmResult{Grade: GradeResult{Rubric: 3, RubricTotal: 3, JudgeScore: -1}},
	}}
	assert.Equal(t, 0, ExitCode(cfg, results), "incomplete arm must not trigger --strict")
}

func TestExitCodeNoStrictAlwaysZero(t *testing.T) {
	t.Parallel()
	results := []RunResult{{
		FTSOn:  ArmResult{Grade: GradeResult{Rubric: 0, RubricTotal: 3, JudgeScore: -1}},
		FTSOff: ArmResult{Grade: GradeResult{Rubric: 3, RubricTotal: 3, JudgeScore: -1}},
	}}
	assert.Equal(t, 0, ExitCode(Config{}, results))
}

func TestJSONReportOmitsAPIKey(t *testing.T) {
	t.Parallel()
	// Regression: writeJSONReport used to serialize the whole Config,
	// which contains APIKey. Any --format json run would have written
	// the key to stdout / the --output file / CI artifacts.
	var buf bytes.Buffer
	cfg := Config{
		Provider: "anthropic",
		Model:    "claude-3-haiku",
		APIKey:   "sk-ant-abcdef-DO-NOT-LEAK",
		Format:   "json",
	}
	require.NoError(t, WriteReport(&buf, cfg, nil))
	assert.NotContains(t, buf.String(), "sk-ant-abcdef-DO-NOT-LEAK",
		"JSON report must never contain the API key")
	assert.NotContains(t, buf.String(), "api_key",
		"JSON report must not even expose an api_key field")
}

func TestApplyEntityHitSkipsEmptyExpectedIDs(t *testing.T) {
	t.Parallel()
	// Regression: when --db is used, SeededFixture{} has empty string
	// IDs. The old applyEntityHit checked strings.Contains(ctx, "id: "+id)
	// which was true for empty id against any normal FTS context.
	r := &runner{}
	q := Question{ExpectedEntityIDs: []string{"", "", ""}}
	arm := &ArmResult{Grade: GradeResult{JudgeScore: -1}}
	r.applyEntityHit(arm, q, "Project \"Kitchen\" (id: 01JX): status=planned")
	assert.Equal(t, 0, arm.Grade.EntitiesHit)
	assert.Equal(t, 0, arm.Grade.EntitiesTotal,
		"empty expected IDs must not contribute to totals")
}

func TestApplyEntityHitCountsNonEmptyOnly(t *testing.T) {
	t.Parallel()
	r := &runner{}
	q := Question{ExpectedEntityIDs: []string{"01JX", "", "01JY"}}
	arm := &ArmResult{Grade: GradeResult{JudgeScore: -1}}
	ctx := `Project "Kitchen" (id: 01JX): status=planned
Vendor "Pacific" (id: 01JY)`
	r.applyEntityHit(arm, q, ctx)
	assert.Equal(t, 2, arm.Grade.EntitiesHit)
	assert.Equal(t, 2, arm.Grade.EntitiesTotal,
		"only non-empty expected IDs count toward totals")
}

// sampleResults returns a minimal RunResult slice for smoke-testing
// every WriteReport format path. Covers the three interesting cases
// the formatters branch on: completed with judge, incomplete (stage-1
// error, no summary), and completed with --skip-judge sentinel.
func sampleResults() []RunResult {
	return []RunResult{
		{
			Question: Question{
				Name:              "kitchen-status",
				ExpectedEntityIDs: []string{"01JX"},
			},
			FTSOn: ArmResult{
				GeneratedSQL: "SELECT 1",
				SummaryText:  "done",
				Grade: GradeResult{
					Rubric:        2,
					RubricTotal:   3,
					JudgeScore:    4,
					EntitiesHit:   1,
					EntitiesTotal: 1,
				},
			},
			FTSOff: ArmResult{
				GeneratedSQL: "SELECT 1",
				SummaryText:  "done",
				Grade: GradeResult{
					Rubric:        1,
					RubricTotal:   3,
					JudgeScore:    2,
					EntitiesHit:   0,
					EntitiesTotal: 1,
				},
			},
		},
		{
			Question: Question{Name: "stage1-fail"},
			FTSOn: ArmResult{
				ErrorKind: errorKindStage1,
				ErrorMsg:  "provider down",
				Grade:     GradeResult{Rubric: 0, RubricTotal: 2, JudgeScore: -1},
			},
			FTSOff: ArmResult{
				Grade: GradeResult{Rubric: 2, RubricTotal: 2, JudgeScore: 3},
			},
		},
		{
			Question: Question{Name: "skip-judge"},
			FTSOn:    ArmResult{Grade: GradeResult{Rubric: 3, RubricTotal: 3, JudgeScore: -1}},
			FTSOff:   ArmResult{Grade: GradeResult{Rubric: 3, RubricTotal: 3, JudgeScore: -1}},
		},
	}
}

// TestWriteReportFormatsDoNotPanic is a smoke test for every format
// WriteReport supports. Before this test landed, writeTableReport
// passed nils to lipgloss.HasDarkBackground and SIGSEGV'd at runtime;
// nothing in the package exercised the styled path. This test drives
// each format + a few NoAB permutations so any nil-dereference,
// out-of-range index, or formatter panic surfaces in CI.
func TestWriteReportFormatsDoNotPanic(t *testing.T) {
	t.Parallel()
	results := sampleResults()

	cases := []struct {
		format string
		noAB   bool
	}{
		{"table", false},
		{"table", true},
		{"markdown", false},
		{"markdown", true},
		{"json", false},
		{"", false}, // unknown/empty should fall through to table
	}
	for _, tc := range cases {
		t.Run(tc.format+"-noab-"+strconv.FormatBool(tc.noAB), func(t *testing.T) {
			var buf bytes.Buffer
			cfg := Config{
				Provider: "ollama",
				Model:    "qwen3",
				Format:   tc.format,
				NoAB:     tc.noAB,
			}
			require.NotPanics(t, func() {
				require.NoError(t, WriteReport(&buf, cfg, results))
			})
			assert.NotEmpty(t, buf.String(),
				"format=%q noAB=%v produced no output", tc.format, tc.noAB)
		})
	}
}

// TestWriteReportEmptyResults covers the zero-question case so the
// aggregate block and tables handle an empty slice without touching
// out-of-range indices.
func TestWriteReportEmptyResults(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"table", "markdown", "json"} {
		t.Run(f, func(t *testing.T) {
			var buf bytes.Buffer
			require.NotPanics(t, func() {
				require.NoError(t, WriteReport(&buf, Config{Format: f}, nil))
			})
		})
	}
}
