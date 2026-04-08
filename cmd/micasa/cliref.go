// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// CLI reference generator. Run via `go generate ./cmd/micasa/`.
//
// Walks the cobra command tree and writes a single Hugo-flavored markdown
// file containing every visible command, its flags, examples, and
// subcommand links. Output goes to docs/content/docs/reference/cli.md and
// is committed to the repo. The generator lives next to the commands so
// docs stay in sync without an external build step.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

//go:generate go run . _gen-cli-ref ../../docs/content/docs/reference/cli.md

// newGenCLIRefCmd returns the hidden command that emits the CLI reference
// markdown. The leading underscore in the name plus Hidden:true keep it
// out of `--help` listings; it exists only as a `go generate` target.
func newGenCLIRefCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "_gen-cli-ref <output-file>",
		Short:         "Generate the Hugo CLI reference (internal)",
		Hidden:        true,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateCLIRef(cmd.Root(), args[0])
		},
	}
}

// generateCLIRef writes the rendered reference to outPath atomically.
// Atomic writes (temp + rename) prevent partial files from confusing the
// pre-commit `go-generate-check` hook when generation is interrupted.
func generateCLIRef(root *cobra.Command, outPath string) error {
	body := renderCLIReference(root)

	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(outPath), ".cli-ref-*.md")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("rename to %s: %w", outPath, err)
	}
	cleanup = false
	return nil
}

// renderCLIReference walks the command tree and returns the full markdown
// document, including TOML frontmatter.
func renderCLIReference(root *cobra.Command) string {
	var buf bytes.Buffer
	writeFrontmatter(&buf, root)

	for _, cmd := range visibleCommandsDFS(root) {
		renderCommandSection(&buf, cmd)
	}
	return buf.String()
}

// writeFrontmatter emits the TOML frontmatter block plus the
// auto-generated banner.
func writeFrontmatter(buf *bytes.Buffer, root *cobra.Command) {
	fmt.Fprintln(buf, "+++")
	fmt.Fprintln(buf, `title = "CLI Reference"`)
	fmt.Fprintln(buf, `linkTitle = "CLI"`)
	fmt.Fprintln(buf, `weight = 3`)
	fmt.Fprintln(
		buf,
		`description = "Auto-generated reference for every micasa command, flag, and subcommand."`,
	)
	fmt.Fprintln(buf, "+++")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "<!-- AUTO-GENERATED. Do not edit by hand. -->")
	fmt.Fprintln(buf, "<!-- Source: cobra command tree in cmd/micasa. -->")
	fmt.Fprintln(buf, "<!-- Regenerate: `go generate ./cmd/micasa/` -->")
	fmt.Fprintln(buf)
	if root.Short != "" {
		fmt.Fprintln(buf, escapeMarkdownProse(root.Short)+".")
		fmt.Fprintln(buf)
	}
}

// renderCommandSection emits one h2 section for cmd, with subsections for
// usage, examples, flags, subcommand links, and a back-link to the parent.
func renderCommandSection(buf *bytes.Buffer, cmd *cobra.Command) {
	path := cmd.CommandPath()
	fmt.Fprintf(buf, "## %s\n\n", path)

	if cmd.Long != "" {
		fmt.Fprintln(buf, escapeMarkdownProse(strings.TrimSpace(cmd.Long)))
		fmt.Fprintln(buf)
	} else if cmd.Short != "" {
		fmt.Fprintln(buf, escapeMarkdownProse(strings.TrimSpace(cmd.Short))+".")
		fmt.Fprintln(buf)
	}

	if cmd.Runnable() {
		fmt.Fprintln(buf, "### Usage")
		fmt.Fprintln(buf)
		fmt.Fprintln(buf, "```")
		fmt.Fprintln(buf, cmd.UseLine())
		fmt.Fprintln(buf, "```")
		fmt.Fprintln(buf)
	}

	if cmd.Example != "" {
		fmt.Fprintln(buf, "### Examples")
		fmt.Fprintln(buf)
		fmt.Fprintln(buf, "```")
		fmt.Fprintln(buf, strings.TrimRight(cmd.Example, "\n"))
		fmt.Fprintln(buf, "```")
		fmt.Fprintln(buf)
	}

	if table := flagsTable(cmd.NonInheritedFlags()); table != "" {
		fmt.Fprintln(buf, "### Flags")
		fmt.Fprintln(buf)
		buf.WriteString(table)
		fmt.Fprintln(buf)
	}
	if table := flagsTable(cmd.InheritedFlags()); table != "" {
		fmt.Fprintln(buf, "### Inherited flags")
		fmt.Fprintln(buf)
		buf.WriteString(table)
		fmt.Fprintln(buf)
	}

	if children := visibleChildren(cmd); len(children) > 0 {
		fmt.Fprintln(buf, "### Subcommands")
		fmt.Fprintln(buf)
		for _, child := range children {
			fmt.Fprintf(
				buf,
				"- [`%s`](#%s) -- %s\n",
				child.CommandPath(),
				headingAnchor(child.CommandPath()),
				escapeMarkdownProse(child.Short),
			)
		}
		fmt.Fprintln(buf)
	}

	if parent := cmd.Parent(); parent != nil {
		fmt.Fprintln(buf, "### See also")
		fmt.Fprintln(buf)
		fmt.Fprintf(
			buf,
			"- [`%s`](#%s) -- %s\n",
			parent.CommandPath(),
			headingAnchor(parent.CommandPath()),
			escapeMarkdownProse(parent.Short),
		)
		fmt.Fprintln(buf)
	}
}

// escapeMarkdownProse escapes characters in cobra Short/Long text that
// goldmark would otherwise interpret as raw HTML or markdown syntax.
// Cobra descriptions are plain prose meant for terminal display, so
// literal `<placeholder>` patterns must not become HTML elements when
// the markdown is rendered. Only `<` and `>` are escaped: `&` is
// usually a literal ampersand in prose and goldmark passes it through.
func escapeMarkdownProse(s string) string {
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// flagsTable renders the given flag set as a markdown table. Returns the
// empty string when there are no visible flags. Pipe characters in
// descriptions are escaped so they don't break table parsing.
func flagsTable(fs *pflag.FlagSet) string {
	type row struct {
		flag  string
		def   string
		usage string
	}
	var rows []row
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		var name strings.Builder
		if f.Shorthand != "" {
			fmt.Fprintf(&name, "`-%s`, `--%s`", f.Shorthand, f.Name)
		} else {
			fmt.Fprintf(&name, "`--%s`", f.Name)
		}
		def := f.DefValue
		if def == "" || def == "false" {
			def = "-"
		} else {
			def = "`" + def + "`"
		}
		rows = append(rows, row{
			flag:  name.String(),
			def:   def,
			usage: escapeTableCell(f.Usage),
		})
	})
	if len(rows) == 0 {
		return ""
	}
	var buf strings.Builder
	fmt.Fprintln(&buf, "| Flag | Default | Description |")
	fmt.Fprintln(&buf, "|------|---------|-------------|")
	for _, r := range rows {
		fmt.Fprintf(&buf, "| %s | %s | %s |\n", r.flag, r.def, r.usage)
	}
	return buf.String()
}

// escapeTableCell makes a string safe for inclusion in a markdown table
// cell: collapses whitespace, replaces newlines with spaces, and escapes
// pipe characters.
func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// headingAnchor mirrors Hugo's default goldmark anchor algorithm closely
// enough for command paths: lowercase, replace whitespace runs with `-`,
// preserve `-` and `_`, drop everything else. Command paths are
// space-separated identifiers that may contain dashes (e.g.
// `maintenance-categories`), so dashes must round-trip.
func headingAnchor(text string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ' || r == '\t':
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		case r == '-' || r == '_':
			b.WriteRune(r)
			prevDash = (r == '-')
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		}
	}
	return b.String()
}

// visibleCommandsDFS returns the root and all visible descendants in
// depth-first preorder, with siblings sorted alphabetically. Hidden
// commands and the auto-generated `help`/`completion` commands are
// excluded.
func visibleCommandsDFS(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		out = append(out, c)
		for _, child := range visibleChildren(c) {
			walk(child)
		}
	}
	walk(root)
	return out
}

// visibleChildren returns cmd's direct subcommands minus hidden ones,
// minus the cobra-injected help and completion commands. Sorted by name
// for deterministic output.
func visibleChildren(cmd *cobra.Command) []*cobra.Command {
	children := cmd.Commands()
	out := make([]*cobra.Command, 0, len(children))
	for _, c := range children {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		if !c.IsAvailableCommand() {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
