+++
title = "Configuration"
weight = 2
description = "CLI flags, environment variables, config file, and LLM setup."
linkTitle = "Configuration"
+++

micasa has minimal configuration -- it's designed to work out of the box.

## CLI

micasa has three subcommands. `run` is the default and launches the TUI;
`config` prints resolved configuration values; `backup` creates a database
snapshot.

```
Usage: micasa <command> [flags]

Commands:
  run [<db-path>]    Launch the TUI (default).
  config <key>       Print the value of a config key.
  backup [<dest>]    Back up the database to a file.

Flags:
  -h, --help       Show context-sensitive help.
      --version    Show version and exit.
```

### `run` (default)

```
micasa [<db-path>] [flags]
```

| Flag | Description |
|------|-------------|
| `<db-path>` | SQLite database path. Overrides `MICASA_DB_PATH`. |
| `--demo` | Launch with fictitious sample data in an in-memory database. |
| `--years=N` | Generate N years of simulated data. Requires `--demo`. |
| `--print-path` | Print the resolved database path and exit. |

When `--demo` is combined with a path, the demo data is written to that
file so you can restart with the same state:

```sh
micasa --demo /tmp/my-demo.db   # creates and populates
micasa /tmp/my-demo.db          # reopens with the demo data
```

### `config`

```
micasa config [<key>] [--dump]
```

Print the resolved value of a configuration key (dot-delimited TOML path):

```sh
micasa config llm.model             # current model name
micasa config llm.base_url          # LLM API endpoint
micasa config documents.max_file_size  # max doc size in bytes
```

Use `--dump` to print the entire resolved configuration as valid TOML.
Each field is annotated with its environment variable: `# env: VAR` as a
hint, or `# src(env): VAR` when that variable is actively overriding the
value. API keys are omitted to avoid accidentally leaking secrets (e.g.
pasting output into a chat or issue).

```sh
micasa config --dump
```

### `backup`

```
micasa backup [<dest>] [--source <path>]
```

| Flag | Description |
|------|-------------|
| `<dest>` | Destination file path. Defaults to `<source>.backup`. |
| `--source` | Source database path. Defaults to the standard location. Honors `MICASA_DB_PATH`. |

Creates a consistent snapshot using SQLite's Online Backup API, safe to
run while the TUI is open:

```sh
micasa backup ~/backups/micasa-$(date +%F).db
micasa backup --source /path/to/micasa.db ~/backups/snapshot.db
```

## Environment variables

| Variable | Default | Config equivalent | Description |
|----------|---------|-------------------|-------------|
| `MICASA_DB_PATH` | [Platform default](#platform-data-directory) | -- | Database file path |
| `MICASA_LLM_PROVIDER` | `ollama` | `llm.provider` | LLM provider name |
| `OLLAMA_HOST` | `http://localhost:11434` | `llm.base_url` | LLM API base URL |
| `MICASA_LLM_BASE_URL` | `http://localhost:11434` | `llm.base_url` | LLM API base URL (alias for `OLLAMA_HOST`) |
| `MICASA_LLM_MODEL` | `qwen3` | `llm.model` | LLM model name |
| `MICASA_LLM_API_KEY` | (empty) | `llm.api_key` | LLM API key for cloud providers |
| `MICASA_LLM_EXTRA_CONTEXT` | (empty) | `llm.extra_context` | Custom context appended to LLM system prompts |
| `MICASA_LLM_TIMEOUT` | `5s` | `llm.timeout` | LLM operation timeout |
| `MICASA_MAX_DOCUMENT_SIZE` | `50 MiB` | `documents.max_file_size` | Max document import size |
| `MICASA_CACHE_TTL` | `30d` | `documents.cache_ttl` | Document cache lifetime |
| `MICASA_CACHE_TTL_DAYS` | -- | `documents.cache_ttl_days` | Deprecated; use `MICASA_CACHE_TTL` |
| `MICASA_EXTRACTION_MODEL` | (chat model) | `extraction.model` | LLM model for document extraction |
| `MICASA_EXTRACTION_ENABLED` | `true` | `extraction.enabled` | Enable/disable LLM extraction |
| `MICASA_EXTRACTION_THINKING` | `false` | `extraction.thinking` | Enable model thinking for extraction |
| `MICASA_TEXT_TIMEOUT` | `30s` | `extraction.text_timeout` | pdftotext timeout |
| `MICASA_MAX_EXTRACT_PAGES` | `0` | `extraction.max_extract_pages` | Max pages to OCR per document (0 = no limit) |
| `MICASA_LLM_THINKING` | (unset) | `llm.thinking` | Enable model thinking for chat |
| `MICASA_CURRENCY` | (auto-detect) | `locale.currency` | ISO 4217 currency code (e.g. `USD`, `EUR`, `GBP`) |

### `MICASA_DB_PATH`

Sets the default database path when no positional argument is given. Equivalent
to passing the path as an argument:

```sh
export MICASA_DB_PATH=/path/to/my/house.db
micasa   # uses /path/to/my/house.db
```

### `OLLAMA_HOST`

Sets the LLM API base URL, overriding the config file value. If the URL
doesn't end with `/v1`, it's appended automatically:

```sh
export OLLAMA_HOST=http://192.168.1.50:11434
micasa   # connects to http://192.168.1.50:11434/v1
```

### `MICASA_LLM_MODEL`

Sets the LLM model name, overriding the config file value:

```sh
export MICASA_LLM_MODEL=llama3.3
micasa   # uses llama3.3 instead of the default qwen3
```

### `MICASA_LLM_TIMEOUT`

Sets the LLM timeout for quick operations (ping, model listing), overriding
the config file value. Uses Go duration syntax:

```sh
export MICASA_LLM_TIMEOUT=15s
micasa   # waits up to 15s for LLM server responses
```

### `MICASA_MAX_DOCUMENT_SIZE`

Sets the maximum file size for document imports, overriding the config file
value. Accepts unitized strings or bare integers (bytes). Must be positive:

```sh
export MICASA_MAX_DOCUMENT_SIZE="100 MiB"
micasa   # allows documents up to 100 MiB
```

### `MICASA_CACHE_TTL`

Sets the document cache lifetime, overriding the config file value. Accepts
day-suffixed strings (`30d`), Go durations (`720h`), or bare integers
(seconds). Set to `0` to disable eviction:

```sh
export MICASA_CACHE_TTL=7d
micasa   # evicts cache entries older than 7 days
```

### `MICASA_CACHE_TTL_DAYS`

Deprecated. Use `MICASA_CACHE_TTL` instead. Accepts a bare integer
interpreted as days. Cannot be set alongside `MICASA_CACHE_TTL`.

### Platform data directory

micasa uses platform-aware data directories (via
[adrg/xdg](https://github.com/adrg/xdg)). When no path is specified (via
argument or `MICASA_DB_PATH`), the database is stored at:

| Platform | Default path |
|----------|-------------|
| Linux    | `$XDG_DATA_HOME/micasa/micasa.db` (default `~/.local/share/micasa/micasa.db`) |
| macOS    | `~/Library/Application Support/micasa/micasa.db` |
| Windows  | `%LOCALAPPDATA%\micasa\micasa.db` |

On Linux, `XDG_DATA_HOME` is respected per the [XDG Base Directory
Specification](https://specifications.freedesktop.org/basedir-spec/latest/).

## Database path resolution order

The database path is resolved in this order:

1. Positional CLI argument, if provided
2. `MICASA_DB_PATH` environment variable, if set
3. Platform data directory (see table above)

In `--demo` mode without a path argument, an in-memory database (`:memory:`)
is used.

## Config file

micasa reads a TOML config file from your platform's config directory:

| Platform | Default path |
|----------|-------------|
| Linux    | `$XDG_CONFIG_HOME/micasa/config.toml` (default `~/.config/micasa/config.toml`) |
| macOS    | `~/Library/Application Support/micasa/config.toml` |
| Windows  | `%APPDATA%\micasa\config.toml` |

The config file is optional. If it doesn't exist, all settings use their
defaults. Unset fields fall back to defaults -- you only need to specify the
values you want to change.

### Example config

```toml
# micasa configuration

[llm]
# LLM provider. Supported: ollama, anthropic, openai, openrouter,
# deepseek, gemini, groq, mistral, llamacpp, llamafile.
# Auto-detected from base_url and api_key when not set.
# provider = "ollama"

# Base URL for the provider's API. No /v1 suffix needed.
# Ollama (default): http://localhost:11434
# llama.cpp:        http://localhost:8080
# LM Studio:        http://localhost:1234
base_url = "http://localhost:11434"

# Model name passed in chat requests.
model = "qwen3"

# API key for cloud providers. Not needed for local servers like Ollama.
# api_key = ""

# Optional: custom context appended to all system prompts.
# Use this to inject domain-specific details about your house, region, etc.
# extra_context = "My house is a 1920s craftsman in Portland, OR."

# Timeout for quick LLM server operations (ping, model listing).
# Go duration syntax: "5s", "10s", "500ms", etc. Default: "5s".
# Increase if your LLM server is slow to respond.
# timeout = "5s"

# Enable model thinking mode for chat (e.g. qwen3 <think> blocks).
# Unset = don't send (server default), true = enable, false = disable.
# thinking = false

[documents]
# Maximum file size for document imports. Accepts unitized strings or bare
# integers (bytes). Default: 50 MiB.
# max_file_size = "50 MiB"

# How long to keep extracted document cache entries before evicting on startup.
# Accepts "30d", "720h", or bare integers (seconds). Set to "0s" to disable.
# Default: 30d.
# cache_ttl = "30d"

[extraction]
# Model for document extraction. Defaults to llm.model. Extraction works well
# with small, fast models optimized for structured JSON output.
# model = "qwen2.5:7b"

# Timeout for pdftotext. Go duration syntax: "30s", "1m", etc. Default: "30s".
# Increase if you routinely process very large PDFs.
# text_timeout = "30s"

# Maximum pages to OCR for scanned documents. 0 = no limit. Default: 0.
# max_extract_pages = 0

# Set to false to disable LLM-powered extraction.
# When disabled, no structured data is extracted from documents.
# enabled = true

# Enable model thinking for extraction. Default: false (faster, no <think>).
# thinking = false

[locale]
# ISO 4217 currency code for all money fields. Stored in the database on first
# run; after that the database value is authoritative (portable DB files keep
# their currency even when opened on a machine with different locale settings).
# Auto-detected from LC_MONETARY/LANG if not set. Default: USD.
# Override with MICASA_CURRENCY env var.
# currency = "USD"
```

### `[llm]` section

LLM provider, model, and connection settings. These are the shared defaults
for all LLM pipelines (chat and extraction). Per-pipeline overrides can be
set in `[llm.chat]` and `[llm.extraction]`.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `provider` | string | `ollama` | LLM provider. Supported: `ollama`, `anthropic`, `openai`, `openrouter`, `deepseek`, `gemini`, `groq`, `mistral`, `llamacpp`, `llamafile`. Auto-detected from `base_url` and `api_key` when not set. |
| `base_url` | string | `http://localhost:11434` | Root URL of the provider's API. No `/v1` suffix needed -- each provider handles path construction. |
| `model` | string | `qwen3` | Model identifier sent in chat requests. Must be available on the server. |
| `api_key` | string | (empty) | Authentication credential. Required for cloud providers (Anthropic, OpenAI, etc.). Leave empty for local servers. |
| `extra_context` | string | (empty) | Free-form text appended to all LLM system prompts. Useful for telling the model about your house or regional conventions. Currency is handled automatically via `[locale]`. |
| `timeout` | string | `"5s"` | Max wait time for quick LLM operations (ping, model listing). Go duration syntax, e.g. `"10s"`, `"500ms"`. Increase for slow servers. |
| `thinking` | bool | (unset) | Enable model thinking mode (e.g. qwen3 `<think>` blocks). Unset = don't send the option (server default). |

### `[llm.chat]` section

Per-pipeline LLM overrides for the chat (NL-to-SQL) pipeline. Empty fields
inherit from `[llm]`. Use this to run chat on a different provider or model
than the default.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `provider` | string | (inherits) | Override LLM provider for chat. |
| `base_url` | string | (inherits) | Override API base URL for chat. |
| `model` | string | (inherits) | Override model for chat. |
| `api_key` | string | (inherits) | Override API key for chat. |
| `timeout` | string | (inherits) | Override timeout for chat. |
| `thinking` | string | (inherits) | Override thinking mode for chat. |

### `[llm.extraction]` section

Per-pipeline LLM overrides for document extraction. Empty fields inherit
from `[llm]`. Use this to run extraction on a smaller, faster model while
keeping a more capable model for chat.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `provider` | string | (inherits) | Override LLM provider for extraction. |
| `base_url` | string | (inherits) | Override API base URL for extraction. |
| `model` | string | (inherits) | Override model for extraction. |
| `api_key` | string | (inherits) | Override API key for extraction. |
| `timeout` | string | (inherits) | Override timeout for extraction. |
| `thinking` | string | (inherits) | Override thinking mode for extraction. |

### `[documents]` section

Document attachment limits and caching.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `max_file_size` | string or integer | `"50 MiB"` | Maximum file size for document imports. Accepts unitized strings (`"50 MiB"`, `"1.5 GiB"`) or bare integers (bytes). Must be positive. |
| `cache_ttl` | string or integer | `"30d"` | Cache lifetime for extracted documents. Accepts `"30d"`, `"720h"`, or bare integers (seconds). Set to `"0s"` to disable eviction. |
| `cache_ttl_days` | integer | -- | Deprecated. Use `cache_ttl` instead. Bare integer interpreted as days. Cannot be set alongside `cache_ttl`. |

### `[extraction]` section

Document extraction pipeline settings. Requires an LLM -- OCR and pdftotext
are internal pipeline steps that feed the LLM, not standalone features.
When enabled, the pipeline extracts structured data (document type, costs,
dates, vendor matching) from uploaded documents.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `model` | string | (chat model) | **Deprecated.** Use `[llm.extraction] model` instead. Falls back to `llm.model` if empty. |
| `text_timeout` | string | `"30s"` | Max time for `pdftotext` to run. Go duration syntax, e.g. `"1m"`. Increase for very large PDFs. |
| `max_extract_pages` | int | `0` | Maximum pages to OCR per scanned document. 0 means no limit. |
| `enabled` | bool | `true` | Set to `false` to disable LLM-powered extraction. When disabled, no structured data is extracted from documents. |
| `thinking` | bool | `false` | **Deprecated.** Use `[llm.extraction] thinking` instead. |

### `[locale]` section

Locale and currency settings. Controls currency formatting across all money
fields in the application.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `currency` | string | (auto-detect) | ISO 4217 currency code (e.g. `USD`, `EUR`, `GBP`, `JPY`). Auto-detected from `LC_MONETARY`/`LANG` if not set, falls back to `USD`. Persisted to the database on first run -- after that the DB value is authoritative. |

Currency resolution order (highest to lowest):

1. Database value (authoritative once set -- makes the DB file portable)
2. `MICASA_CURRENCY` environment variable
3. `[locale] currency` config value
4. Auto-detect from `LC_MONETARY` or `LANG` locale
5. `USD` fallback

Formatting is locale-correct: EUR uses comma decimals and period grouping
(`1.234,56`), GBP uses the pound sign (`£750.00`), JPY uses yen with no
decimal places, etc.

### Supported LLM backends

micasa talks to any server that implements the OpenAI chat completions API
with streaming (SSE). All providers -- including Ollama -- communicate via
OpenAI-compatible endpoints; there is no native SDK dependency on any
provider.

#### Local backends

[Ollama](https://ollama.com) is the primary tested backend:

| Backend | Default URL | Notes |
|---------|-------------|-------|
| [Ollama](https://ollama.com) | `http://localhost:11434/v1` | Default and tested. Models are pulled automatically if not present. |
| [llama.cpp server](https://github.com/ggml-org/llama.cpp) | `http://localhost:8080/v1` | Should work (untested). Pass `--host` and `--port` when starting the server. |
| [llamafile](https://github.com/Mozilla-Ocho/llamafile) | `http://localhost:8080/v1` | Single-file executable with built-in server. |
| [LM Studio](https://lmstudio.ai) | `http://localhost:1234/v1` | Should work (untested). Enable the local server in LM Studio settings. |

#### Cloud providers

micasa also supports cloud LLM providers. Set `provider`, `base_url`, and
`api_key` in the `[llm]` section. Cloud providers use their own default
base URLs when none is configured.

| Provider | Notes |
|----------|-------|
| [OpenAI](https://openai.com) | GPT-4o, o1, etc. |
| [Anthropic](https://anthropic.com) | Claude models. Does not support model listing. |
| [DeepSeek](https://deepseek.com) | DeepSeek-R1, DeepSeek-V3, etc. |
| [Google Gemini](https://ai.google.dev) | Gemini models. |
| [Groq](https://groq.com) | Fast inference for open models. |
| [Mistral](https://mistral.ai) | Mistral and Mixtral models. |
| [OpenRouter](https://openrouter.ai) | Multi-provider gateway. Uses the OpenAI protocol. |

### Override precedence

Environment variables override config file values. The full precedence order
(highest to lowest):

1. Environment variables (see [table above](#environment-variables))
2. Config file values
3. Built-in defaults

### `extra_context` examples

The `extra_context` field is injected into every system prompt sent to the
LLM, giving it persistent knowledge about your situation:

```toml
[llm]
extra_context = """
My house is a 1920s craftsman bungalow in Portland, OR.
Property tax is assessed annually in November.
The HVAC system is a heat pump (Mitsubishi hyper-heat) -- no gas furnace.
"""
```

This helps the model give more relevant answers without you repeating context
in every question. Currency is configured separately via `[locale] currency`
and is automatically available to the LLM -- no need to mention it in
`extra_context`.

## Persistent preferences

Some preferences are stored in the SQLite database and persist across
restarts. These are controlled through the UI rather than config files:

| Preference | Default | How to change |
|------------|---------|---------------|
| Dashboard on startup | Shown | Press <kbd>D</kbd> to toggle; your choice is remembered |
| LLM model | From config | Changed automatically when you switch models in the chat interface |
| Currency | USD | Set via `[locale] currency` in config, `MICASA_CURRENCY` env var, or auto-detected from system locale. Persisted to the database on first use |
