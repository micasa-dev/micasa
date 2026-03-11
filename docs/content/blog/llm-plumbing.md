+++
title = "Your LLM is not a load-bearing wall"
date = 2026-03-05
description = "micasa now talks to ten LLM providers. It still doesn't trust any of them."
+++

The [extraction pipeline](/blog/extraction/) shipped a week ago with
a hand-rolled OpenAI-compatible client. Naturally, it only worked with Ollama.
It might sound weird, but that was intentional: local-first, no requirement to
send the details of your house to our AI overlords.

After a couple weeks of using various models locally, I learned (perhaps the
hard way), that local models just _cannot_ hold a candle to the frontier models
offered by the large AI companies.

To that end, I wanted two things:

1. Support for frontier models, like those of OpenAI and Anthropic.
2. To avoid conversations like "Yeah, but do you support `$MY_FAVORITE_API`".

Enter `any-llm-go`.

## Someone else's problem

[`any-llm-go`](https://github.com/mozilla-ai/any-llm-go) is a Go library from
Mozilla that wraps the official SDKs for Ollama, OpenAI, Anthropic, Gemini,
Groq, Mistral, DeepSeek, OpenRouter, llama.cpp, and Llamafile behind one
interface. I deleted my client and replaced it with about forty lines of setup
([#566](https://github.com/cpcloud/micasa/pull/566)). The binary grew from
27 MB to 47 MB from linking all the provider SDKs, which is fine because the
alternative was me writing auth code for ten APIs.

**Forty-seven megabytes** is _absolutely enormous_ for a terminal application,
but who in their right mind wants to deal with **ten** REST APIs that are all
doing basically the same thing?

Apparently the answer to that question is: the people doing the lord's work
over at Mozilla.

Back to the tech.

The provider details are configured in your [micasa config](/docs/reference/configuration/#chatllm-section):

```toml
[chat.llm]
provider = "anthropic"
model = "claude-sonnet-4-5-latest"
api_key = "sk-..."
```

Local Ollama still works with zero config. Nothing changed for the default
setup.

## Two pipelines, two models

Extraction reads invoices and proposes database fields. Chat answers
natural-language questions about your data. These want different things --
extraction wants a small model that's fast and precise with JSON, chat wants
something that can actually reason about whether your roof maintenance is
overdue.

They used to share a model. Now they don't
([#575](https://github.com/cpcloud/micasa/pull/575)):

```toml
[chat.llm]
provider = "ollama"
model = "qwen3"

[extraction.llm]
provider = "anthropic"
model = "claude-haiku-4-5-latest"
api_key = "sk-..."
```

Chat runs locally, extraction runs on Anthropic. Or both local. Or both cloud.
Each pipeline has its own independent `[chat.llm]` and `[extraction.llm]`
sections -- no inheritance, no cross-contamination.

## Picking models at runtime

<kbd>r</kbd> on a completed extraction step opens a fuzzy model picker instead of
immediately rerunning ([#560](https://github.com/cpcloud/micasa/pull/560)).
Type to filter, arrows to navigate, enter to select. If the model isn't local,
it pulls first.

This matters because extraction is trial-and-error. A clean PDF with selectable
text and a 3B model works fine. A photo of a receipt from a parking lot that
you took while drunk might need something bigger. Switching without leaving the
overlay keeps the loop tight.

## Extraction in the background

OCR on a multi-page scan takes a while. <kbd>ctrl+b</kbd> now backgrounds a running
extraction ([#559](https://github.com/cpcloud/micasa/pull/559)). The status bar
shows a spinner while jobs run and a count when they finish. <kbd>ctrl+b</kbd> again
foregrounds the latest result for review. Nothing auto-accepts -- you always
look before it writes.

## Other things since last week

- **[Locale-aware currency](/docs/reference/configuration/#locale-section)** --
  EUR gets comma decimals and period grouping (`1.234,56`), GBP gets the pound
  sign, JPY drops decimal places. Auto-detected from your system locale or set
  via `MICASA_LOCALE_CURRENCY`.
  ([#467](https://github.com/cpcloud/micasa/pull/467))
- **[Imperial/metric toggle](/docs/guide/house-profile/)** -- <kbd>U</kbd> switches
  between square feet and square meters. Defaults to metric unless your locale
  is US, Liberia, or Myanmar.
  ([#555](https://github.com/cpcloud/micasa/pull/555))
- **[Resolved incidents](/docs/guide/incidents/)** -- resolving an incident now
  sets a proper `resolved` status. <kbd>D</kbd> permanently deletes resolved incidents
  with confirmation.
  ([#588](https://github.com/cpcloud/micasa/pull/588))
- **`config --dump`** -- prints fully resolved config as annotated TOML with
  env var hints. API keys are omitted to prevent us from doing dumb things like
  pasting secrets into an AI.
  ([#597](https://github.com/cpcloud/micasa/pull/597))
- **Extraction timeout** -- configurable LLM timeout (default 2 minutes) so
  a hung model doesn't lock the overlay.
  ([#604](https://github.com/cpcloud/micasa/pull/604))

Under the hood: the 56-field `Model` God-struct got demoted to demi-god status during
some [code surgery](https://github.com/cpcloud/micasa/pull/567) with
generics in the data layer (-248 lines of code, +339 lines of tests), and
eight new [static analysis tools](https://github.com/cpcloud/micasa/pull/599)
run in pre-commit. Static analysis seems to keep the bot army away from some of
its dumber tendencies, so let's load up on it.

## Try it

```sh
go run github.com/cpcloud/micasa/cmd/micasa@latest --demo
```

Or with a cloud provider:

```sh
export MICASA_CHAT_LLM_PROVIDER=anthropic
export MICASA_CHAT_LLM_MODEL=claude-haiku-4-5-latest
export MICASA_CHAT_LLM_API_KEY=sk-...
go run github.com/cpcloud/micasa/cmd/micasa@latest --demo
```

Binaries on the
[releases page](https://github.com/cpcloud/micasa/releases/latest).
