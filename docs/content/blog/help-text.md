+++
title = "Your --help was the ugliest screen in the app"
date = 2026-03-19
description = "micasa swapped Kong for Cobra. Now there's tab completion, colored help, and CLI tests that run in 100ms."
+++

Kong parsed my arguments for four months without complaint. It never crashed,
never misrouted a flag, never did anything wrong. It also never generated shell
completions, so micasa didn't have any. And the `--help` output was plain
monochrome text sitting next to a TUI that uses a carefully chosen color
palette. The help screen was the one part of micasa that looked like a
different application.

## Three things for free

[#785](https://github.com/cpcloud/micasa/pull/785) replaces Kong with Cobra.

**Tab completion exists now.** Kong doesn't generate shell completions, so
micasa shipped without them. Cobra builds completions from the command
definitions at runtime. `micasa completion bash`, `micasa completion zsh`,
`micasa completion fish` -- each writes a completion script to stdout. Flags,
subcommands, file arguments. If I add a subcommand next week, the completions
already know about it.

**Help has colors.** Cobra lets you override the help function. micasa's
`--help` now uses the Wong palette -- subcommands in one color, flags in
another, descriptions in the adaptive foreground. It matches the TUI. The
terminal no longer changes personalities when you ask for help.

**CLI tests don't build a binary.** Under Kong, testing the CLI meant
compiling the full `micasa` binary and exec-ing it in a subprocess. Go had
to link the entire dependency tree every time, which took about ten seconds.
Cobra commands are functions -- construct the root command, set the args,
call `Execute()` against an `io.Writer`, check the output. CLI test time
dropped from ~10 seconds to ~100 milliseconds.

## The LLM shows its work

The extraction pipeline proposes database operations -- create a vendor,
update a title, link a quote -- but until now you couldn't see the details
of what it proposed. You got a table preview and an accept/reject choice.

The Documents tab now has an **Ops** column
([#776](https://github.com/cpcloud/micasa/pull/776)). Selecting it opens
an interactive JSON tree showing every proposed `INSERT`, `UPDATE`, and `DELETE`
with all field values inline. Navigate with <kbd>j</kbd>/<kbd>k</kbd>,
expand with <kbd>l</kbd>, collapse with <kbd>h</kbd>. Collapsed nodes show
inline previews like `{email: "...", name: "..."}` so you can scan without
expanding everything. <kbd>g</kbd>/<kbd>G</kbd> jump to first and last.
Mouse clicks work too.

![ops tree overlay](/images/ops-tree.webp)

If the LLM proposed something wrong, you can now point at the exact field
before you accept.

## Faster the second time

Running extraction twice on the same document used to redo the full
pipeline -- OCR, text extraction, everything. Now it skips the expensive
layers and feeds cached text straight to the LLM
([#763](https://github.com/cpcloud/micasa/pull/763)). Re-extraction is
nearly instant for documents where OCR was the bottleneck.

The same PR added <kbd>r</kbd> in edit mode to trigger extraction from the
table without opening a form, a **Model** column showing which LLM produced
the extraction, and persistent extraction metadata in the database. If you
need to know what model read your invoice six months from now, it's there.

## Other things since last week

- **Keybinding hints** -- two-tier keycap rendering: pill keycaps for inline
  hints in status bars and overlay footers, bold accent for reference panels
  like the help overlay and calendar legend
  ([#783](https://github.com/cpcloud/micasa/pull/783)).
- **Document restore** -- accepting an extraction on a soft-deleted document
  now restores it instead of silently writing to a hidden row
  ([#777](https://github.com/cpcloud/micasa/pull/777)).
- **Hide-deleted** -- soft-deleting a row now respects your explicit
  hide-deleted toggle instead of overriding it
  ([#774](https://github.com/cpcloud/micasa/pull/774)).
- **Sort viewport** -- toggling sort no longer left the viewport pointing
  at the wrong rows
  ([#773](https://github.com/cpcloud/micasa/pull/773)).
- **Service log sync** -- closing the service log overlay auto-syncs and
  highlights the Last column so you see the update immediately
  ([#772](https://github.com/cpcloud/micasa/pull/772)).
- **Error rendering** -- failed extraction step errors render as plain text
  instead of raw JSON
  ([#778](https://github.com/cpcloud/micasa/pull/778)).

## Try it

```sh
go run github.com/cpcloud/micasa/cmd/micasa@latest --demo
```

Set up tab completions:

```sh
micasa completion bash > /etc/bash_completion.d/micasa
micasa completion zsh > "${fpath[1]}/_micasa"
micasa completion fish > ~/.config/fish/completions/micasa.fish
```

Binaries on the
[releases page](https://github.com/cpcloud/micasa/releases/latest).
