+++
title = "Your --help was the ugliest screen in the app"
date = 2026-03-19
description = "micasa swapped Kong for Cobra. Now there's tab completion, colored help, and CLI tests that run in 100ms."
+++

Kong parsed my arguments for four months. It worked fine. It also didn't
generate shell completions, so micasa had none. And `--help` was plain
monochrome text next to a TUI with a color palette -- the one screen that
looked like it belonged to a different application.

## Kong out, Cobra in

[#785](https://github.com/micasa-dev/micasa/pull/785) replaces Kong with Cobra.
The main reason was completions -- Kong doesn't generate them, so micasa
never had tab completion. Cobra builds them from the command tree at runtime.
`micasa completion bash` writes a script to stdout, same for zsh and fish.
Add a subcommand next week, the completions already know about it.

While I was in there, I overrode Cobra's help function to use the Wong
palette. Subcommands in one color, flags in another, descriptions in the
adaptive foreground. `--help` looks like the rest of the app now.

Side benefit: Kong's CLI tests compiled the full binary and exec'd it in
a subprocess, which took about ten seconds per run. Cobra commands are
functions -- construct the root, set args, call `Execute()` against an
`io.Writer`. CLI tests went from ~10s to ~100ms.

## The LLM shows its work

The extraction pipeline proposes database operations -- create a vendor,
update a title, link a quote -- but you couldn't see the details. You got
a summary and an accept/reject choice.

Documents now have an **Ops** column
([#776](https://github.com/micasa-dev/micasa/pull/776)). It opens an
interactive JSON tree: every proposed `INSERT`, `UPDATE`, and `DELETE` with
field values inline. <kbd>j</kbd>/<kbd>k</kbd> to navigate,
<kbd>l</kbd> to expand, <kbd>h</kbd> to collapse. Collapsed nodes show
inline previews. Clickable too.

<video src="/videos/ops-tree.webm" class="demo-video" autoplay loop muted playsinline></video>

You can point at the exact field the LLM got wrong before you accept.

## Faster the second time

Re-extracting a document used to redo the full pipeline -- OCR, text
extraction, everything. Now it skips straight to the LLM with the cached
text from the first run
([#763](https://github.com/micasa-dev/micasa/pull/763)).

Same PR: <kbd>r</kbd> in edit mode triggers extraction without opening a
form. A **Model** column shows which LLM produced the extraction. The model
name and operations JSON are persisted in the database -- if you need to
know what model read your invoice six months from now, it's there.

## Other things since last week

- **Charm v2** -- bubbletea, lipgloss, huh, bubblezone, and glamour all
  migrated to their v2 releases. Go 1.26 required. The `bubbletea-overlay`
  dependency got inlined. Nothing should look different, but if something
  does, [open an issue](https://github.com/micasa-dev/micasa/issues)
  ([#788](https://github.com/micasa-dev/micasa/pull/788)).
- **`micasa demo`** -- `--demo` is now a subcommand. `micasa demo --years 10`
  instead of `micasa --demo --years 10`
  ([#787](https://github.com/micasa-dev/micasa/pull/787)).
- **Keybinding hints** -- two-tier keycap rendering: pill keycaps for inline
  hints, bold accent for reference panels like the help overlay
  ([#783](https://github.com/micasa-dev/micasa/pull/783)).
- **Document restore** -- accepting an extraction on a soft-deleted document
  now restores it instead of silently writing to a hidden row
  ([#777](https://github.com/micasa-dev/micasa/pull/777)).
- **Hide-deleted** -- soft-deleting a row now respects your explicit
  hide-deleted toggle instead of overriding it
  ([#774](https://github.com/micasa-dev/micasa/pull/774)).
- **Sort** -- toggling sort didn't visually activate until you pressed a
  navigation key; the cached viewport wasn't being invalidated
  ([#773](https://github.com/micasa-dev/micasa/pull/773)).
- **Service log sync** -- closing the service log overlay auto-syncs and
  highlights the Last column so you see the update immediately
  ([#772](https://github.com/micasa-dev/micasa/pull/772)).
- **Error rendering** -- failed extraction step errors render as plain text
  instead of raw JSON
  ([#778](https://github.com/micasa-dev/micasa/pull/778)).

## Try it

```sh
go run github.com/micasa-dev/micasa/cmd/micasa@latest demo
```

Tab completions:

```sh
source <(micasa completion bash)
source <(micasa completion zsh)
micasa completion fish | source
```

Binaries on the
[releases page](https://github.com/micasa-dev/micasa/releases/latest).
