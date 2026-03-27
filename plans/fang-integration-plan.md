<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Fang CLI Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hand-rolled CLI help/error/completion/version handling with `charm.land/fang/v2`, using a custom Wong colorblind-safe color scheme.

**Architecture:** Single `fang.Execute()` call in `main()` replaces manual Cobra orchestration. A new `theme.go` defines the Wong color scheme. Existing `help.go` and `completion.go` are deleted entirely.

**Tech Stack:** Go, `charm.land/fang/v2`, `charm.land/lipgloss/v2` (already in use), `spf13/cobra` (already in use)

**Spec:** `plans/fang-integration.md`

---

### File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `cmd/micasa/theme.go` | Wong color scheme for fang |
| Modify | `cmd/micasa/main.go` | Replace `root.Execute()` with `fang.Execute()`, remove help/completion/version wiring |
| Modify | `cmd/micasa/main_test.go` | Update `executeCLI` for fang, fix completion and version tests |
| Delete | `cmd/micasa/help.go` | Replaced by fang's help renderer |
| Delete | `cmd/micasa/completion.go` | Replaced by fang's built-in completion |
| Modify | `go.mod` / `go.sum` | Add `charm.land/fang/v2` |

---

### Task 1: Add fang dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add fang to go.mod**

```bash
go get charm.land/fang/v2@latest
```

- [ ] **Step 2: Tidy**

```bash
go mod tidy
```

- [ ] **Step 3: Verify it resolves**

```bash
go list -m charm.land/fang/v2
```

Expected: `charm.land/fang/v2 v2.x.x` (latest version)

- [ ] **Step 4: Commit**

```
chore(deps): add charm.land/fang/v2
```

---

### Task 2: Create Wong color scheme

**Files:**
- Create: `cmd/micasa/theme.go`

- [ ] **Step 1: Write theme.go**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"image/color"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
)

// Wong palette hex values used for CLI theming. These duplicate the values in
// internal/app/styles.go intentionally so the CLI binary does not import the
// full TUI package.
//
// Light/dark adaptive pairs: the first value is for light backgrounds, the
// second for dark backgrounds.

func wongColorScheme(c lipgloss.LightDarkFunc) fang.ColorScheme {
	blue := c(lipgloss.Color("#0072B2"), lipgloss.Color("#56B4E9"))
	orange := c(lipgloss.Color("#D55E00"), lipgloss.Color("#E69F00"))
	green := c(lipgloss.Color("#009E73"), lipgloss.Color("#009E73"))
	purple := c(lipgloss.Color("#CC79A7"), lipgloss.Color("#CC79A7"))
	vermillion := c(lipgloss.Color("#D55E00"), lipgloss.Color("#D55E00"))
	base := c(lipgloss.Color("#4B5563"), lipgloss.Color("#9CA3AF"))
	dim := c(lipgloss.Color("#4B5563"), lipgloss.Color("#6B7280"))
	cream := lipgloss.Color("#FFFAF1")
	codeblockBg := c(lipgloss.Color("#F1EFEF"), lipgloss.Color("#2F2E36"))

	return fang.ColorScheme{
		Base:           base,
		Title:          blue,
		Description:    base,
		Codeblock:      codeblockBg,
		Program:        blue,
		DimmedArgument: dim,
		Comment:        dim,
		Flag:           green,
		FlagDefault:    dim,
		Command:        orange,
		QuotedString:   purple,
		Argument:       base,
		ErrorHeader:    [2]color.Color{cream, vermillion},
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./cmd/micasa/
```

Expected: builds successfully (theme.go is unused yet, but compiles as part of the package)

- [ ] **Step 3: Commit**

```
feat(cli): add Wong colorblind-safe theme for fang
```

---

### Task 3: Wire fang.Execute() and remove old wiring

**Files:**
- Modify: `cmd/micasa/main.go`

- [ ] **Step 1: Update imports in main.go**

Add `"charm.land/fang/v2"` to the import block. `context` is already imported.

- [ ] **Step 2: Modify newRootCmd()**

Remove these lines from `newRootCmd()`:

```go
root.SetVersionTemplate("{{.Version}}\n")
root.SetHelpFunc(styledHelp)
root.CompletionOptions.HiddenDefaultCmd = true
```

And remove `newCompletionCmd(root)` from the `root.AddCommand(...)` call.

Also remove these fields from the root command literal since fang sets them:

```go
Version:       versionString(),
```

The root command should still set `SilenceErrors: true` and `SilenceUsage: true` — fang also sets them, but keeping them explicit is harmless and documents intent.

After edits, `newRootCmd()` should look like:

```go
func newRootCmd() *cobra.Command {
	opts := &runOpts{}

	root := &cobra.Command{
		Use:           data.AppName + " [database-path]",
		Short:         "A terminal UI for tracking everything about your home",
		Long:          "A terminal UI for tracking everything about your home.",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.dbPath = args[0]
			}
			return runTUI(cmd.OutOrStdout(), opts)
		},
	}

	root.Flags().
		BoolVar(&opts.printPath, "print-path", false, "Print the resolved database path and exit")

	root.AddCommand(
		newDemoCmd(),
		newBackupCmd(),
		newConfigCmd(),
		newProCmd(),
	)

	return root
}
```

- [ ] **Step 3: Replace main()**

Replace the `main()` function:

```go
func main() {
	root := newRootCmd()
	if err := fang.Execute(
		context.Background(),
		root,
		fang.WithVersion(versionString()),
		fang.WithColorSchemeFunc(wongColorScheme),
		fang.WithNotifySignal(os.Interrupt),
	); err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			os.Exit(130)
		}
		os.Exit(1)
	}
}
```

Note: the `fmt.Fprintf(os.Stderr, ...)` error line is removed — fang prints
styled errors to stderr automatically before returning the error.

- [ ] **Step 4: Remove unused imports**

After the edits, these imports may become unused in `main.go`:
- `"fmt"` — check if still needed (it is, used in `runTUI`, `runBackup`, etc.)
- `"io"` — still needed
- No imports should become unused since `fmt`, `context`, `errors`, `os`, `io`
  are all still used by the remaining functions.

- [ ] **Step 5: Verify it compiles**

```bash
go build ./cmd/micasa/
```

Expected: builds successfully

- [ ] **Step 6: Quick smoke test**

```bash
./micasa --help
./micasa --version
./micasa pro --help
./micasa backup --help
./micasa nonexistent 2>&1
```

Verify: help output has syntax highlighting, version prints, errors show styled
`ERROR` banner.

- [ ] **Step 7: Commit**

```
feat(cli): wire fang.Execute() for styled help, errors, and version
```

---

### Task 4: Delete help.go and completion.go

**Files:**
- Delete: `cmd/micasa/help.go`
- Delete: `cmd/micasa/completion.go`

- [ ] **Step 1: Delete help.go**

```bash
git rm cmd/micasa/help.go
```

- [ ] **Step 2: Delete completion.go**

```bash
git rm cmd/micasa/completion.go
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./cmd/micasa/
```

Expected: builds successfully — no references to `styledHelp` or
`newCompletionCmd` remain after Task 3.

- [ ] **Step 4: Commit**

```
refactor(cli): remove hand-rolled help and completion (replaced by fang)
```

---

### Task 5: Update tests

**Files:**
- Modify: `cmd/micasa/main_test.go`

- [ ] **Step 1: Update executeCLI to use fang**

The current `executeCLI` calls `root.Execute()` directly. Since fang adds the
completion command and sets up help/version, tests that exercise those features
need to go through fang. Replace `executeCLI`:

```go
func executeCLI(args ...string) (string, string, error) {
	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := fang.Execute(
		context.Background(),
		root,
		fang.WithVersion(versionString()),
		fang.WithColorSchemeFunc(wongColorScheme),
	)
	return stdout.String(), stderr.String(), err
}
```

Note the signature change: returns `(stdout, stderr, error)` instead of
`(stdout, error)`. Fang writes errors to stderr, so tests need access to both.

- [ ] **Step 2: Update all executeCLI call sites**

Every call to `executeCLI` now returns three values. Update each one.

For tests that don't care about stderr (most of them), use `_`:

```go
out, _, err := executeCLI("config", "get", ".chat.llm.model")
```

For tests that check errors, stderr may contain the styled error output:

```go
_, _, err := executeCLI("backup", "--source", ":memory:", dest)
```

Grep for all `executeCLI(` calls and update each. There are approximately 15
call sites in `main_test.go`.

- [ ] **Step 3: Update TestCompletionCmd**

Fang uses cobra's built-in completion which registers a `completion` command
with shell subcommands. The test structure stays the same but the output format
may differ slightly. Update:

```go
func TestCompletionCmd(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			out, _, err := executeCLI("completion", shell)
			require.NoError(t, err)
			assert.NotEmpty(t, out)
			assert.Contains(t, out, "micasa", "completion script should reference the app name")
		})
	}
}
```

- [ ] **Step 4: Update TestVersion_DevShowsCommitHash**

This test runs the built binary via subprocess. Fang formats version as
`version (commit)` when it detects a commit. Since we pass
`WithVersion(versionString())` and `versionString()` returns the bare commit
hash (e.g. `abc1234` or `abc1234-dirty`), fang won't append an extra commit
suffix (it only does that when `WithCommit()` is also passed).

However, fang's version template differs from cobra's default. Test that the
binary still outputs version info. The regex may need loosening:

```go
func TestVersion_DevShowsCommitHash(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat(".git"); err != nil {
		t.Skip("no .git directory; VCS info unavailable (e.g. Nix sandbox)")
	}
	bin := getTestBin(t)
	verCmd := exec.CommandContext(t.Context(), bin, "--version")
	out, err := verCmd.Output()
	require.NoError(t, err, "--version failed")
	got := strings.TrimSpace(string(out))
	assert.NotEqual(t, "dev", got, "expected commit hash, got bare dev")
	// Fang may prefix with the binary name; extract just the version part.
	// Accept: "abc1234", "abc1234-dirty", "simple abc1234", "simple abc1234-dirty"
	assert.Regexp(t, `[0-9a-f]{7,}(-dirty)?`, got, "expected hex hash somewhere in %q", got)
}
```

- [ ] **Step 5: Update TestVersion_Injected**

This test calls `versionString()` directly — no change needed since
`versionString()` is unchanged.

- [ ] **Step 6: Add fang import to test file**

Add to the import block:

```go
"charm.land/fang/v2"
```

Also add `"context"` if not already present.

- [ ] **Step 7: Run all tests**

```bash
go test -shuffle=on ./cmd/micasa/
```

Expected: all tests pass.

- [ ] **Step 8: Run full test suite**

```bash
go test -shuffle=on ./...
```

Expected: no regressions.

- [ ] **Step 9: Commit**

```
test(cli): update tests for fang integration
```

---

### Task 6: Pre-commit and final verification

**Files:** none (verification only)

- [ ] **Step 1: Run linter**

```bash
golangci-lint run ./cmd/micasa/
```

Expected: no warnings.

- [ ] **Step 2: Run full test suite one more time**

```bash
go test -shuffle=on ./...
```

Expected: all pass.

- [ ] **Step 3: Visual smoke test**

Build and run each help command to visually verify the Wong palette:

```bash
go build -o /tmp/micasa-fang ./cmd/micasa/
/tmp/micasa-fang --help
/tmp/micasa-fang --version
/tmp/micasa-fang pro --help
/tmp/micasa-fang config --help
/tmp/micasa-fang backup --help
/tmp/micasa-fang completion --help
/tmp/micasa-fang man -h
/tmp/micasa-fang nonexistent 2>&1
```

Verify:
- Section titles (USAGE, COMMANDS, FLAGS) appear in blue
- Program name (`micasa`) appears in sky blue
- Subcommands appear in orange
- Flags appear in bluish green
- Error banner uses vermillion background
- Code blocks have subtle background

- [ ] **Step 4: Use /pre-commit-check**

Run the pre-commit check skill before final commit.

- [ ] **Step 5: Squash or commit if needed**

If any fixes were needed from steps 1-4, commit them:

```
fix(cli): address lint/test issues from fang integration
```
