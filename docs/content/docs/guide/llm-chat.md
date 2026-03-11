+++
title = "LLM Chat"
weight = 10
description = "Ask questions about your home data using a local LLM."
linkTitle = "LLM Chat"
+++

Let's be honest: **this is a gimmick.** You can sort and filter the tables
faster than a 32-billion-parameter model can figure out whether your furnace
filter column is called `next` or `next_due`. But I'm trying to preempt every future conversation that ends with "but
does it have AI?" so here we are. You're
welcome. I'm sorry.

micasa includes a built-in chat interface that lets you ask questions about
your home data. A local LLM translates your question into
SQL, runs it against your database, and summarizes the results. Everything
runs locally -- your data never leaves your machine.

<video src="/videos/llm-chat.webm" class="demo-video" autoplay loop muted playsinline></video>

## Prerequisites

You need a local LLM server running an OpenAI-compatible API.
[Ollama](https://ollama.com) is the recommended and tested option:

```sh
# install Ollama (macOS/Linux)
curl -fsSL https://ollama.com/install.sh | sh

# pull the default model
ollama pull qwen3

# start the server (if not already running)
ollama serve
```

micasa connects to Ollama at `http://localhost:11434/v1` by default. See
[Configuration]({{< ref "/docs/reference/configuration" >}}) to change the server
URL, model, or backend.

## Opening the chat

Press <kbd>@</kbd> from Nav or Edit mode to open the chat overlay. A text input
appears at the bottom of a centered panel. Type a question and press <kbd>enter</kbd>.

Press <kbd>esc</kbd> to dismiss the overlay. Your conversation is preserved -- press
<kbd>@</kbd> again to pick up where you left off.

## Asking questions

Type a natural language question about your home data:

- "How many projects are on hold right now?"
- "Which projects are underway?"
- "When is the HVAC filter due?"
- "Show me all quotes from Ace Plumbing"
- "What appliances have warranties expiring this year?"

micasa translates your question through a two-stage pipeline:

1. **SQL generation** -- the LLM writes a SQL query against your schema
2. **Result interpretation** -- the query runs, and the LLM summarizes the
   results

The model has access to your full database schema, including table
relationships, column types, and the actual distinct values stored in key
columns (project types, statuses, vendor names, etc.). This means it can
handle fuzzy references like "plumbing stuff" or "planned projects" without
you needing to know the exact column values.

### Follow-up questions

The LLM maintains conversational context within a session. You can ask
follow-up questions that reference previous answers:

- "How much did I spend on plumbing?" then "What about electrical?"
- "Show me active projects" then "Which ones are over budget?"

Context resets when you close micasa.

## SQL display

Press <kbd>ctrl+s</kbd> to toggle SQL query visibility. When on, each answer shows the
generated SQL query in a formatted code block above the response. This is
useful for verifying what the model is actually querying, or learning how your
data is structured.

SQL is pretty-printed with uppercased keywords, indented clauses, and
one-column-per-line SELECT lists. The toggle is retroactive -- it
shows or hides SQL for the entire conversation, not just new messages.

SQL streams in real-time as the model generates it, so you can see the query
taking shape before results appear.

## Cancellation

Press <kbd>ctrl+c</kbd> while the model is generating to cancel the current request.
An "Interrupted" notice appears in the conversation. Your next question
replaces the notice.

## Prompt history

Use <kbd>up</kbd>/<kbd>down</kbd> arrows (or <kbd>ctrl+p</kbd>/<kbd>ctrl+n</kbd>) to browse previous prompts.
History is saved to the database and persists across sessions.

## Slash commands

The chat input supports a few slash commands:

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/models` | List models available on the server |
| `/model <name>` | Switch to a different model |
| `/sql` | Toggle SQL display (same as <kbd>ctrl+s</kbd>) |

### Switching models

Type `/model ` (with a trailing space) to activate the model picker -- an
inline autocomplete list showing both locally downloaded models and popular
models available for download. Use <kbd>up</kbd>/<kbd>down</kbd> to navigate and <kbd>enter</kbd> to
select.

If you pick a model that isn't downloaded yet, micasa pulls it automatically.
A progress bar shows download progress. Press <kbd>ctrl+c</kbd> to cancel a pull.

## Mag mode

Press <kbd>ctrl+o</kbd> to toggle [mag mode](https://magworld.pw) -- an easter egg that
replaces money values with their order of magnitude (`$1,250` becomes `$ 🠡3`).
Works with any configured currency. Applies everywhere including LLM responses.
Live toggle, instant update.

## Output quality

Look, it's a small language model running on your laptop, not an oracle.
It will confidently produce nonsense sometimes -- that's the deal. Quality
depends heavily on which model you're running, and which model you can run
depends on how much GPU you're packing. A few things to keep in mind:

**Wrong SQL is common.** Small models (7B-14B parameters) frequently generate
SQL that doesn't match the schema, joins tables incorrectly, or misinterprets
your question. micasa provides the model with your full schema and actual
database values to help, but it's not foolproof. Toggle <kbd>ctrl+s</kbd> to inspect
the generated SQL when an answer looks off.

**Phrasing matters.** The same question worded differently can produce
different results. "How much did plumbing cost?" and "Total plumbing spend"
might yield different SQL. If you get a bad answer, try rephrasing.

**Bigger models are better.** If you have the hardware, larger models
(32B+ parameters) produce noticeably more accurate SQL and more useful
summaries. The default `qwen3` is a good starting point, but stepping up
to something like `qwen3:32b` or `deepseek-r1:32b` makes a real difference.

**Hallucinated numbers.** The model sometimes invents numbers that aren't in
your data, especially for aggregation queries. If a dollar amount or count
looks surprising, verify it with the SQL view or check the actual table.

**Case and abbreviations.** micasa instructs the model to use case-insensitive
matching and maps common abbreviations (like "plan" to "planned"), but
models occasionally ignore these instructions. If a query returns no results
when you expected some, the model may have used a case-sensitive or
literal match.

**Not a replacement for looking at the data.** The chat is best for quick
lookups and ad-hoc questions -- "when is X due?", "how much did Y cost?",
"show me Z." For anything you'd act on financially or contractually, verify
the answer against the actual tables.

## Data and privacy

micasa defaults to a local LLM server on your machine. When you use the chat,
here is exactly what gets sent to the LLM endpoint.

### What the model sees

The two-stage pipeline sends different data at each step:

1. **SQL generation** (stage 1) -- the model receives your database **schema**
   (table names, column names, types) plus a sample of **distinct values** from
   key columns (project types, statuses, vendor names, etc.). It does **not**
   see full row data at this stage.
2. **Result interpretation** (stage 2) -- the model receives the SQL query
   results (just the rows matching your question) and summarizes them.

If the model fails to produce valid SQL, micasa falls back to a single-stage
mode that sends a **full dump of all non-deleted rows** from every user table.
Internal columns (`id`, `created_at`, `updated_at`, `deleted_at`) and document
file contents are excluded, but everything else -- addresses, costs, vendor
contacts, appliance details, notes -- is included.

In both modes, the model also receives your **conversation history** from the
current session and any **extra context** you configured.

### Local by default

The default endpoint is `http://localhost:11434/v1` (Ollama running on your
machine). With this setup, all data stays on your computer -- nothing is sent
over the network.

### Remote endpoints

If you point `base_url` at a remote server (a cloud-hosted Ollama instance,
a LAN machine, etc.), your home data travels over the network to that server.
micasa connects over plain HTTP by default. Consider:

- **Use HTTPS** if the endpoint supports it (`https://...`)
- **Trust the server** -- the operator of a remote LLM endpoint can see
  everything micasa sends, including the full data dump in fallback mode
- **Network exposure** -- anyone who can intercept traffic between you and a
  remote HTTP endpoint can read your home data

The LLM feature is entirely optional. If you never configure a `[chat.llm]`
section, no data is ever sent anywhere.

## Configuration

The chat requires a `[chat.llm]` section in your config file. If no LLM is
configured, the chat overlay shows a helpful hint with the config path and
a sample configuration.

See [Configuration]({{< ref "/docs/reference/configuration" >}}) for the full
reference, including how to set `extra_context` to give the model persistent
knowledge about your house. Currency is configured separately via
`[locale] currency` and is automatically available to the LLM.
