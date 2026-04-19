// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package ftseval

import "regexp"

// DefaultQuestions returns the benchmark question set rooted in the
// SeededFixture. Each question has rigid rubric regexps plus a free-form
// judge prompt the (optional) LLM judge uses for semantic grading.
//
// The question names are stable identifiers used by --questions.
func DefaultQuestions(f SeededFixture) []Question {
	mustRE := func(pat string) *regexp.Regexp {
		return regexp.MustCompile(`(?i)` + pat)
	}

	return []Question{
		{
			Name:  "kitchen-status",
			Query: "what's the status of the kitchen project?",
			RubricSQL: []*regexp.Regexp{
				mustRE(`from\s+projects`),
			},
			RubricSummary: []*regexp.Regexp{
				mustRE(`kitchen\s+remodel`),
				mustRE(`in[-\s_]?progress`),
			},
			ExpectedEntityIDs: []string{f.ProjectKitchenID},
			JudgePrompt: "Does the answer correctly identify the status of the " +
				"'Kitchen Remodel' project as in_progress, without confusing it " +
				"with the 'Kitchen Supplies Co' vendor?",
		},
		{
			Name:  "plumber-quote",
			Query: "how much was the plumber's quote for the kitchen?",
			RubricSQL: []*regexp.Regexp{
				mustRE(`from\s+quotes`),
			},
			RubricSummary: []*regexp.Regexp{
				mustRE(`\$?4,?500`),
				mustRE(`pacific\s+plumbing|plumber`),
			},
			ExpectedEntityIDs: []string{
				f.QuoteKitchenID,
				f.VendorPacificID,
				f.ProjectKitchenID,
			},
			JudgePrompt: "Does the answer name the Pacific Plumbing quote of " +
				"$4,500 for the Kitchen Remodel project?",
		},
		{
			Name:  "hvac-last-service",
			Query: "when was the hvac filter last changed?",
			RubricSQL: []*regexp.Regexp{
				mustRE(`from\s+service_log_entries`),
			},
			RubricSummary: []*regexp.Regexp{
				mustRE(`hvac|filter`),
			},
			ExpectedEntityIDs: []string{f.ServiceLogHVACID, f.MaintHVACID},
			JudgePrompt: "Does the answer report the most recent service date " +
				"for the 'HVAC Filter Change' maintenance item (about one month ago)?",
		},
		{
			Name:  "total-project-spend",
			Query: "what's the total actual spend across all projects?",
			RubricSQL: []*regexp.Regexp{
				mustRE(`sum\s*\(`),
				mustRE(`actual_cents|spent`),
			},
			RubricSummary: []*regexp.Regexp{
				mustRE(`\$?3,?000|\$?300|total`),
			},
			JudgePrompt: "Is the total actual spend computed from the projects " +
				"table and reported in dollars (the fixture totals $3,000)? " +
				"FTS context is not expected to help this aggregate query.",
		},
		{
			Name:  "basement-incidents",
			Query: "any issues in the basement?",
			RubricSQL: []*regexp.Regexp{
				mustRE(`from\s+incidents|location`),
			},
			RubricSummary: []*regexp.Regexp{
				mustRE(`leak|basement`),
			},
			ExpectedEntityIDs: []string{f.IncidentLeakID},
			JudgePrompt:       "Does the answer mention the open basement leak incident?",
		},
		{
			Name:  "nonexistent-project",
			Query: "what's the status of the attic project?",
			RubricSummary: []*regexp.Regexp{
				mustRE(`no\s+attic|not\s+found|don't\s+have|no\s+(such\s+)?project`),
			},
			JudgePrompt: "Does the answer correctly say no attic project exists, " +
				"without hallucinating one?",
		},
		{
			Name:  "long-tail-note",
			Query: "which vendor mentioned permit delays?",
			RubricSQL: []*regexp.Regexp{
				mustRE(`from\s+vendors`),
			},
			RubricSummary: []*regexp.Regexp{
				mustRE(`pacific\s+plumbing`),
			},
			ExpectedEntityIDs: []string{f.VendorPacificID},
			JudgePrompt: "Does the answer identify Pacific Plumbing as the vendor " +
				"whose notes mention permit delays?",
		},
		{
			Name:  "appliance-by-brand",
			Query: "list my maytag appliances",
			RubricSQL: []*regexp.Regexp{
				mustRE(`from\s+appliances`),
				mustRE(`brand|maytag`),
			},
			RubricSummary: []*regexp.Regexp{
				mustRE(`maytag\s+dishwasher|dishwasher`),
			},
			ExpectedEntityIDs: []string{f.ApplianceMaytagID},
			JudgePrompt: "Does the answer list the Maytag Dishwasher and no other " +
				"(non-Maytag) appliances?",
		},
	}
}

// FilterQuestions returns the subset of `all` whose Name is in `names`.
// A nil or empty `names` slice returns `all` unchanged.
func FilterQuestions(all []Question, names []string) []Question {
	if len(names) == 0 {
		return all
	}
	keep := make(map[string]bool, len(names))
	for _, n := range names {
		keep[n] = true
	}
	var out []Question
	for _, q := range all {
		if keep[q.Name] {
			out = append(out, q)
		}
	}
	return out
}
