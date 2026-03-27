<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Fang CLI Integration

Adopt `charm.land/fang/v2` for styled help pages, error output, version
handling, man pages, and shell completions. Replace the hand-rolled `help.go`
and `completion.go` with fang's batteries-included setup, using a custom Wong
colorblind-safe palette.

## Motivation

The current CLI help is functional but plain: examples are uniformly dimmed,
there are no code-block backgrounds, and errors are a single unformatted line.
Fang provides per-token syntax highlighting (program name, subcommands, flags,
quoted strings, comments), code blocks with subtle backgrounds, styled error
banners, automatic man page generation, and shell completions --- all from a
single `fang.Execute()` call.

## Color Scheme (Wong Palette)

Fang's `WithColorSchemeFunc` accepts a function `func(lipgloss.LightDarkFunc)
fang.ColorScheme`. Map each `ColorScheme` slot to Wong colors using
light/dark adaptive pairs:

| Token            | Light           | Dark            |
|------------------|-----------------|-----------------|
| Title            | `#0072B2` Blue  | `#56B4E9` Sky   |
| Program name     | `#0072B2` Blue  | `#56B4E9` Sky   |
| Command          | `#D55E00` Verm  | `#E69F00` Org   |
| Flag             | `#009E73` BGrn  | `#009E73` BGrn  |
| Quoted string    | `#CC79A7` RPur  | `#CC79A7` RPur  |
| Argument / Base  | `#4B5563`       | `#9CA3AF`       |
| Dimmed / Comment | `#4B5563`       | `#6B7280`       |
| Flag default     | `#4B5563`       | `#6B7280`       |
| Error header fg  | `#FFFAF1` cream | `#FFFAF1` cream |
| Error header bg  | `#D55E00` Verm  | `#D55E00` Verm  |
| Codeblock bg     | `#F1EFEF`       | `#2F2E36`       |

## Changes

### Delete

- `cmd/micasa/help.go` --- all 134 lines replaced by fang's help renderer
- `cmd/micasa/completion.go` --- fang adds its own completion command

### Modify

**`cmd/micasa/main.go`**

- Add `charm.land/fang/v2` import.
- Move color scheme to new `theme.go` (see Add section).
- In `newRootCmd()`:
  - Remove `root.SetHelpFunc(styledHelp)`.
  - Remove `root.SetVersionTemplate("{{.Version}}\n")`.
  - Remove `root.CompletionOptions.HiddenDefaultCmd = true`.
  - Remove `newCompletionCmd(root)` from `root.AddCommand(...)`.
  - Remove `root.Version = versionString()` (fang sets it).
- In `main()`, replace:
  ```go
  if err := root.Execute(); err != nil {
      if errors.Is(err, tea.ErrInterrupted) {
          os.Exit(130)
      }
      fmt.Fprintf(os.Stderr, "%s: %v\n", data.AppName, err)
      os.Exit(1)
  }
  ```
  with:
  ```go
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
  ```
  Note: fang already handles printing styled errors to stderr, so the
  `fmt.Fprintf` line is removed.
- Keep `versionString()` --- it detects `-dirty` which fang does not.

**`cmd/micasa/main_test.go`**

- `executeCLI` continues to use `root.Execute()` for non-fang tests (config,
  backup, path resolution). These test business logic, not CLI chrome.
- Add `executeFang` helper that wraps with `fang.Execute()` for tests that need
  fang behavior.
- `TestCompletionCmd`: update to use fang's completion subcommand structure
  (`completion bash`, `completion zsh`, `completion fish` still works --- fang
  uses cobra's built-in completion which supports the same shells plus
  powershell).
- `TestVersion_DevShowsCommitHash`: may need regex update since fang appends
  `(commit)` format when it detects a commit hash via `debug.ReadBuildInfo()`.
  Since we pass `WithVersion(versionString())` which already includes the hash
  in dev mode, and we don't pass `WithCommit()`, fang won't append anything
  extra. Test should still pass as-is.

### Add

**`cmd/micasa/theme.go`** (new file)

Contains `wongColorScheme` function. Keeps the theme definition separate from
command wiring for readability. Uses `lipgloss.AdaptiveColor`-style light/dark
pairs matching the table above.

**`go.mod` / `go.sum`**

Add `charm.land/fang/v2` and its transitive dependencies (`muesli/mango-cobra`,
`muesli/roff`, `charmbracelet/colorprofile`, `charmbracelet/x/exp/charmtone`).

## What We Get For Free

- **Syntax-highlighted examples**: program name, subcommands, flags, args,
  quoted strings, and comments each get distinct Wong palette colors.
- **Code blocks**: usage and example sections render inside a subtle
  background panel.
- **Styled errors**: prominent `ERROR` badge in vermillion with "try --help"
  hint for usage errors.
- **Man pages**: hidden `man` subcommand generates roff-formatted man pages.
- **Shell completions**: cobra's built-in completion (bash, zsh, fish,
  powershell).
- **Version with commit**: `--version` shows version plus short commit hash.
- **Windows VT processing**: fang enables VT on older Windows automatically.

## What We Lose / Trade

- Fine-grained control over help layout (fang's layout is opinionated: Usage
  and Examples in code blocks, Commands and Flags outside). The current layout
  is similar enough that this is acceptable.
- The custom completion command had `completion bash|zsh|fish` exactly. Fang's
  uses cobra's built-in which adds powershell. Functionally equivalent.

## Not In Scope

- Changing the TUI's internal color scheme (stays Wong via `appStyles`).
- Adding examples to commands that don't currently have them (can be done
  later).
- Light-mode testing (the mapping is defined; visual verification is a
  follow-up).
