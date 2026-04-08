// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenCLIRef_WritesFile drives the hidden command end-to-end and
// asserts the output file exists with valid TOML frontmatter and the
// auto-generated banner.
func TestGenCLIRef_WritesFile(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "cli.md")

	_, err := executeCLI("_gen-cli-ref", out)
	require.NoError(t, err)

	body, err := os.ReadFile(out) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)

	got := string(body)
	assert.True(t, strings.HasPrefix(got, "+++\n"), "missing TOML frontmatter")
	assert.Contains(t, got, `title = "CLI Reference"`)
	assert.Contains(t, got, "<!-- AUTO-GENERATED. Do not edit by hand. -->")
}

// TestGenCLIRef_ContainsAllVisibleCommands asserts every non-hidden
// command (root + nested + pro/show/config children) appears as an h2
// section.
func TestGenCLIRef_ContainsAllVisibleCommands(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "cli.md")
	_, err := executeCLI("_gen-cli-ref", out)
	require.NoError(t, err)

	body, err := os.ReadFile(out) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	got := string(body)

	for _, want := range []string{
		"## micasa",
		"## micasa demo",
		"## micasa backup",
		"## micasa config",
		"## micasa config get",
		"## micasa config edit",
		"## micasa pro",
		"## micasa pro init",
		"## micasa pro invite",
		"## micasa pro join",
		"## micasa pro sync",
		"## micasa show",
		"## micasa show appliances",
		"## micasa show maintenance-categories",
		"## micasa query",
		"## micasa mcp",
	} {
		assert.Contains(t, got, want+"\n", "missing section: %s", want)
	}
}

// TestGenCLIRef_ExcludesHidden asserts the hidden `_gen-cli-ref`,
// auto-injected `help`, and `completion` commands stay out of the
// output.
func TestGenCLIRef_ExcludesHidden(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "cli.md")
	_, err := executeCLI("_gen-cli-ref", out)
	require.NoError(t, err)

	body, err := os.ReadFile(out) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	got := string(body)

	for _, hidden := range []string{
		"## micasa _gen-cli-ref",
		"## micasa help",
		"## micasa completion",
	} {
		assert.NotContains(t, got, hidden, "hidden command leaked: %s", hidden)
	}
}

// TestGenCLIRef_RendersFlags asserts flags from a known command appear
// in the markdown table with their defaults rendered.
func TestGenCLIRef_RendersFlags(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "cli.md")
	_, err := executeCLI("_gen-cli-ref", out)
	require.NoError(t, err)

	body, err := os.ReadFile(out) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	got := string(body)

	// `micasa demo --years` exists with default 0.
	assert.Regexp(t, "`--years`.*`0`", got)
	// `micasa demo --seed-only` is a bool with default false (rendered as `-`).
	assert.Contains(t, got, "`--seed-only`")
	// `micasa pro init --relay-url` defaults to the production URL.
	assert.Contains(t, got, "`--relay-url`")
	assert.Contains(t, got, "`https://relay.micasa.dev`")
}

// TestGenCLIRef_RendersFrameworkInjectedFlags asserts that flags cobra
// and fang inject lazily during execute() -- `--help` on every command
// and `--version` on the root -- are present in the generated reference.
// Without this guard the docs claim to cover "every command, flag, and
// subcommand" but quietly omit the framework-injected ones. The fix
// pre-initializes default flags before walking the tree.
func TestGenCLIRef_RendersFrameworkInjectedFlags(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "cli.md")
	_, err := executeCLI("_gen-cli-ref", out)
	require.NoError(t, err)

	body, err := os.ReadFile(out) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	got := string(body)

	// `--version` is set on the root by fang.WithVersion. It must
	// appear in the root section's flag table.
	rootSection := sliceSection(t, got, "## micasa\n", "## micasa ")
	assert.Contains(t, rootSection, "`-v`, `--version`",
		"root section missing --version flag")

	// `--help` is added by cobra on every command. Sample two: root
	// and a leaf subcommand.
	assert.Contains(t, rootSection, "`-h`, `--help`",
		"root section missing --help flag")
	demoSection := sliceSection(t, got, "## micasa demo\n", "## micasa ")
	assert.Contains(t, demoSection, "`-h`, `--help`",
		"demo section missing --help flag")
}

// sliceSection returns the substring of body that begins at the first
// occurrence of start and ends at the next occurrence of end after that
// (or the end of the document). Used to scope assertions to a single
// command's h2 section.
func sliceSection(t *testing.T, body, start, end string) string {
	t.Helper()
	_, rest, ok := strings.Cut(body, start)
	require.True(t, ok, "section start %q not found", start)
	before, _, _ := strings.Cut(rest, end)
	return before
}

// TestGenCLIRef_RendersSubcommandLinks asserts the root section links
// to `pro`, and `pro` links to its children via in-page anchors that
// match the headings rendered later in the document.
func TestGenCLIRef_RendersSubcommandLinks(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "cli.md")
	_, err := executeCLI("_gen-cli-ref", out)
	require.NoError(t, err)

	body, err := os.ReadFile(out) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	got := string(body)

	assert.Contains(t, got, "[`micasa pro`](#micasa-pro)")
	assert.Contains(t, got, "[`micasa pro init`](#micasa-pro-init)")
	// Anchor preserves dashes inside command names.
	assert.Contains(
		t,
		got,
		"[`micasa show maintenance-categories`](#micasa-show-maintenance-categories)",
	)
	// Children link back via the See also block.
	assert.Contains(t, got, "[`micasa`](#micasa) -- A terminal UI")
}

// TestGenCLIRef_Deterministic guarantees byte-identical output across
// repeated runs so the pre-commit go-generate-check stays stable.
func TestGenCLIRef_Deterministic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out1 := filepath.Join(dir, "cli1.md")
	out2 := filepath.Join(dir, "cli2.md")

	_, err := executeCLI("_gen-cli-ref", out1)
	require.NoError(t, err)
	_, err = executeCLI("_gen-cli-ref", out2)
	require.NoError(t, err)

	a, err := os.ReadFile(out1) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	b, err := os.ReadFile(out2) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	assert.Equal(t, string(a), string(b))
}

// TestGenCLIRef_RequiresOutputArg asserts cobra rejects the missing
// argument case.
func TestGenCLIRef_RequiresOutputArg(t *testing.T) {
	t.Parallel()
	_, err := executeCLI("_gen-cli-ref")
	assert.Error(t, err)
}

// TestHeadingAnchor pins the slugifier behavior so anchor mismatches
// surface as test failures rather than broken in-page links.
func TestHeadingAnchor(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"micasa":                             "micasa",
		"micasa pro":                         "micasa-pro",
		"micasa pro init":                    "micasa-pro-init",
		"micasa show maintenance-categories": "micasa-show-maintenance-categories",
		"micasa show service-log":            "micasa-show-service-log",
	}
	for in, want := range cases {
		assert.Equal(t, want, headingAnchor(in), "headingAnchor(%q)", in)
	}
}

// TestEscapeTableCell asserts pipes, newlines, and runs of whitespace
// are normalized so flag descriptions never break markdown tables.
func TestEscapeTableCell(t *testing.T) {
	t.Parallel()
	got := escapeTableCell("first | second\nline\twith\r\nspaces")
	assert.Equal(t, `first \| second line with spaces`, got)
}

// TestEscapeMarkdownProse_PlaceholdersAreEscaped guards against the
// `<placeholder>` text in cobra Long descriptions being parsed as
// raw HTML by Hugo's goldmark renderer.
func TestEscapeMarkdownProse_PlaceholdersAreEscaped(t *testing.T) {
	t.Parallel()
	in := "Run with micasa pro join <code> to bootstrap"
	got := escapeMarkdownProse(in)
	assert.Equal(t, "Run with micasa pro join &lt;code&gt; to bootstrap", got)
}

// TestGenCLIRef_LongDescriptionEscaped verifies the end-to-end output
// escapes `<placeholder>` in Long text so it never reaches Hugo as raw
// HTML. The `pro` command's Long contains literal `<code>` and was the
// real-world regression that motivated this guard.
func TestGenCLIRef_LongDescriptionEscaped(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "cli.md")
	_, err := executeCLI("_gen-cli-ref", out)
	require.NoError(t, err)

	body, err := os.ReadFile(out) //nolint:gosec // test reads its own temp file
	require.NoError(t, err)
	got := string(body)

	// The Long text contains `micasa pro join <code>` -- the angle
	// brackets must be HTML-entity escaped where they appear in prose.
	assert.Contains(t, got, "micasa pro join &lt;code&gt;")
	// The UseLine inside the fenced ``` block must stay literal.
	assert.Contains(t, got, "micasa pro join <code> [database-path] [flags]")
}
