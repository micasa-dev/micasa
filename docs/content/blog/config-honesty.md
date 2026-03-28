+++
title = "Your config file was lying to you"
date = 2026-03-13
description = "micasa 2.0 breaks your config on purpose, and now you can interrogate it with jq."
+++

I shipped a breaking change last week. On purpose.

The config system had a shared `[llm]` section that both the chat and
extraction pipelines inherited from. If you set `llm.model`, chat used it.
Extraction also used it, unless you set `llm.extraction.model`, which
overrode the base. Unless you also set the environment variable, which
overrode the override.

I wrote this. I tested it. It passed. It was also wrong.

The problem with cascading defaults is that they feel elegant until someone
asks "what model is extraction using right now?" and the answer depends on
which of four config sources got evaluated last. I spent twenty minutes
debugging my own config system last month. That was the moment.

## Orthogonal or nothing

[v2.0](https://github.com/micasa-dev/micasa/releases/tag/v2.0.0) rips out the
shared `[llm]` section and replaces it with two independent sections that
don't know about each other
([#747](https://github.com/micasa-dev/micasa/pull/747)):

```toml
[chat.llm]
provider = "ollama"
model = "qwen3"

[extraction.llm]
provider = "anthropic"
model = "claude-haiku-4-5-latest"
api_key = "sk-…"
```

No inheritance. No cascading. Changing `chat.llm.model` affects chat.
Changing `extraction.llm.model` affects extraction. Nothing else happens.

If you had a `[llm]` section in your config, it's ignored now. The migration
is manual -- copy the values into the right subsections. I deleted over a
hundred lines of compatibility shims and deprecated-key migration code, and
the config tests got shorter by a thousand lines. That's usually a sign
you're removing the right thing.

## Talking to your config

The old `micasa config` printed a TOML dump. The new one has two subcommands
([#750](https://github.com/micasa-dev/micasa/pull/750)):

`config edit` opens your config file in `$EDITOR`. If there's no file yet,
it creates one with annotated example TOML so you're not staring at a blank
buffer trying to remember the key names.

`config get` takes a jq filter:

```sh
micasa config get '.chat.llm.model'
# qwen3

micasa config get '.extraction.llm'
# [extraction.llm]
# provider = "anthropic"
# model = "claude-haiku-4-5-latest"
```

Scalars print bare, objects encode as TOML, arrays as JSON. API keys are
stripped so you can pipe the output somewhere without accidentally leaking
secrets. The jq integration uses `gojq`, so it's compiled in -- no runtime
dependency.

## Validation without the boilerplate

The config validation used to be a hundred lines of hand-written `if` chains.
Now it's struct tags
([#761](https://github.com/micasa-dev/micasa/pull/761)):

```go
type LLMConfig struct {
    Provider string        `toml:"provider" validate:"omitempty,provider"`
    Model    string        `toml:"model"`
    Timeout  time.Duration `toml:"timeout"  validate:"omitempty,positive_duration"`
}
```

Three custom validators (`provider`, `positive_duration`, `nonneg_duration`)
cover everything the old code did. Error messages use the TOML field names,
not the Go struct names, so when it tells you `chat.llm.provider: unknown
provider "foo"`, you know exactly which line to fix.

## Seasons

Homeowners think in seasons. "What do I need to do this spring" is a more
natural question than "show me all maintenance items with an interval between
60 and 120 days."

Maintenance items now have an optional season tag -- spring, summer, fall,
winter ([#733](https://github.com/micasa-dev/micasa/pull/733)). The dashboard
picks up the current season and shows a section for items tagged with it. Pin
the season column to filter the table. Inline-edit it with <kbd>e</kbd>.

## Better OCR for structured documents

The extraction pipeline now sends spatial layout annotations from Tesseract
to the LLM ([#724](https://github.com/micasa-dev/micasa/pull/724)). Each line
gets a compact `[left,top,width]` prefix so the model knows where text sits
on the page. Lines with low OCR confidence get flagged with a confidence
score.

This matters for invoices and forms where spatial relationships carry
meaning -- the number next to "Total" is more useful than the number next to
"Page." Token overhead is roughly 2x, which is a reasonable trade for not
having the model confuse your invoice total with a date. Toggle it off with
<kbd>t</kbd> in the extraction overlay if you're feeding it clean text
documents.

## Other things since last week

- **Code-generated columns** -- column constants are now generated from
  declarative definitions in `coldefs.go`
  ([#760](https://github.com/micasa-dev/micasa/pull/760)). Adding a column is
  a one-line edit followed by `go generate`. No more hand-syncing iota blocks
  with column specs.
- **Weekly releases** -- micasa stopped releasing on every push to main
  ([#756](https://github.com/micasa-dev/micasa/pull/756)). Releases now run on
  a weekly schedule with grouped release notes. Fewer notifications, better
  changelogs.
- **Docs audit** -- config reference, keybindings page, README, and website
  all got a pass for accuracy
  ([#743](https://github.com/micasa-dev/micasa/pull/743)).

## Try it

```sh
go run github.com/micasa-dev/micasa/cmd/micasa@latest demo
```

If you're upgrading from 1.x, check the
[config reference](/docs/reference/configuration/) for the new section
layout. Binaries on the
[releases page](https://github.com/micasa-dev/micasa/releases/latest).
