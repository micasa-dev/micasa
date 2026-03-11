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
micasa config chat.llm.model           # current chat model name
micasa config extraction.llm.model     # extraction model name
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

Every config key has a corresponding environment variable derived
mechanically from the dotted TOML path:

```
MICASA_ + UPPER(dotted.path with "." replaced by "_")
```

For example, `documents.max_file_size` becomes `MICASA_DOCUMENTS_MAX_FILE_SIZE`.
You can always infer the env var name from the config key.

| Variable | Default | Config equivalent | Description |
|----------|---------|-------------------|-------------|
| `MICASA_DB_PATH` | [Platform default](#platform-data-directory) | -- | Database file path |
| `MICASA_CHAT_ENABLE` | `true` | `chat.enable` | Enable/disable the chat feature |
| `MICASA_CHAT_LLM_PROVIDER` | `ollama` | `chat.llm.provider` | Chat LLM provider name |
| `MICASA_CHAT_LLM_BASE_URL` | `http://localhost:11434` | `chat.llm.base_url` | Chat LLM API base URL |
| `MICASA_CHAT_LLM_MODEL` | `qwen3` | `chat.llm.model` | Chat LLM model name |
| `MICASA_CHAT_LLM_API_KEY` | (empty) | `chat.llm.api_key` | Chat API key for cloud providers |
| `MICASA_CHAT_LLM_EXTRA_CONTEXT` | (empty) | `chat.llm.extra_context` | Custom context appended to chat system prompts |
| `MICASA_CHAT_LLM_TIMEOUT` | `5m` | `chat.llm.timeout` | Chat inference timeout |
| `MICASA_CHAT_LLM_THINKING` | (unset) | `chat.llm.thinking` | Chat model thinking mode |
| `MICASA_EXTRACTION_MAX_PAGES` | `0` | `extraction.max_pages` | Max pages to OCR per document (0 = no limit) |
| `MICASA_EXTRACTION_LLM_ENABLE` | `true` | `extraction.llm.enable` | Enable/disable LLM extraction |
| `MICASA_EXTRACTION_LLM_PROVIDER` | `ollama` | `extraction.llm.provider` | Extraction LLM provider name |
| `MICASA_EXTRACTION_LLM_BASE_URL` | `http://localhost:11434` | `extraction.llm.base_url` | Extraction LLM API base URL |
| `MICASA_EXTRACTION_LLM_MODEL` | `qwen3` | `extraction.llm.model` | Extraction LLM model name |
| `MICASA_EXTRACTION_LLM_API_KEY` | (empty) | `extraction.llm.api_key` | Extraction API key for cloud providers |
| `MICASA_EXTRACTION_LLM_TIMEOUT` | `5m` | `extraction.llm.timeout` | Extraction inference timeout |
| `MICASA_EXTRACTION_LLM_THINKING` | (unset) | `extraction.llm.thinking` | Extraction model thinking mode |
| `MICASA_EXTRACTION_OCR_ENABLE` | `true` | `extraction.ocr.enable` | Enable/disable OCR on documents |
| `MICASA_EXTRACTION_OCR_TSV_ENABLE` | `true` | `extraction.ocr.tsv.enable` | Enable/disable spatial layout annotations |
| `MICASA_EXTRACTION_OCR_TSV_CONFIDENCE_THRESHOLD` | `70` | `extraction.ocr.tsv.confidence_threshold` | OCR confidence threshold (0-100) |
| `MICASA_DOCUMENTS_MAX_FILE_SIZE` | `50 MiB` | `documents.max_file_size` | Max document import size |
| `MICASA_DOCUMENTS_CACHE_TTL` | `30d` | `documents.cache_ttl` | Document cache lifetime |
| `MICASA_DOCUMENTS_CACHE_TTL_DAYS` | -- | `documents.cache_ttl_days` | Deprecated; use `MICASA_DOCUMENTS_CACHE_TTL` |
| `MICASA_DOCUMENTS_FILE_PICKER_DIR` | (Downloads) | `documents.file_picker_dir` | Starting directory for the file picker |
| `MICASA_LOCALE_CURRENCY` | (auto-detect) | `locale.currency` | ISO 4217 currency code (e.g. `USD`, `EUR`, `GBP`) |

### `MICASA_DB_PATH`

Sets the default database path when no positional argument is given. Equivalent
to passing the path as an argument:

```sh
export MICASA_DB_PATH=/path/to/my/house.db
micasa   # uses /path/to/my/house.db
```

### `MICASA_CHAT_LLM_MODEL`

Sets the chat LLM model name, overriding the config file value:

```sh
export MICASA_CHAT_LLM_MODEL=llama3.3
micasa   # uses llama3.3 instead of the default qwen3 for chat
```

### `MICASA_CHAT_LLM_TIMEOUT`

Sets the maximum time for a single chat LLM response (including streaming),
overriding the config file value. Uses Go duration syntax:

```sh
export MICASA_CHAT_LLM_TIMEOUT=10m
micasa   # waits up to 10m for chat LLM responses
```

### `MICASA_DOCUMENTS_MAX_FILE_SIZE`

Sets the maximum file size for document imports, overriding the config file
value. Accepts unitized strings or bare integers (bytes). Must be positive:

```sh
export MICASA_DOCUMENTS_MAX_FILE_SIZE="100 MiB"
micasa   # allows documents up to 100 MiB
```

### `MICASA_DOCUMENTS_CACHE_TTL`

Sets the document cache lifetime, overriding the config file value. Accepts
day-suffixed strings (`30d`), Go durations (`720h`), or bare integers
(seconds). Set to `0` to disable eviction:

```sh
export MICASA_DOCUMENTS_CACHE_TTL=7d
micasa   # evicts cache entries older than 7 days
```

### `MICASA_DOCUMENTS_CACHE_TTL_DAYS`

Deprecated. Use `MICASA_DOCUMENTS_CACHE_TTL` instead. Accepts a bare integer
interpreted as days. Cannot be set alongside `MICASA_DOCUMENTS_CACHE_TTL`.

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
# Each section is self-contained. No section's values affect another section.

[chat]
# Set to false to hide the chat feature from the UI.
# enable = true

[chat.llm]
# LLM connection settings for the chat (NL-to-SQL) pipeline.
# provider = "ollama"
base_url = "http://localhost:11434"
model = "qwen3"
# api_key = ""
# timeout = "5m"
# thinking = "medium"
# extra_context = "My house is a 1920s craftsman in Portland, OR."

[extraction]
# max_pages = 0

[extraction.llm]
# LLM connection settings for document extraction.
# enable = true
# provider = "ollama"
model = "qwen3"
# timeout = "5m"
# thinking = "low"

[extraction.ocr]
# enable = true

[extraction.ocr.tsv]
# enable = true
# confidence_threshold = 70

[documents]
# max_file_size = "50 MiB"
# cache_ttl = "30d"

[locale]
# currency = "USD"
```

### `[chat]` section

Controls the chat (NL-to-SQL) feature and its LLM settings.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable` | bool | `true` | Set to `false` to hide the chat feature from the UI. |

### `[chat.llm]` section

LLM connection settings for the chat pipeline. Each field has its own
default; no values are inherited from other config sections.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `provider` | string | `ollama` | LLM provider. Supported: `ollama`, `anthropic`, `openai`, `openrouter`, `deepseek`, `gemini`, `groq`, `mistral`, `llamacpp`, `llamafile`. Auto-detected from `base_url` and `api_key` when not set. |
| `base_url` | string | `http://localhost:11434` | Root URL of the provider's API. No `/v1` suffix needed. |
| `model` | string | `qwen3` | Model identifier sent in chat requests. |
| `api_key` | string | (empty) | Authentication credential. Required for cloud providers. Leave empty for local servers. |
| `timeout` | string | `"5m"` | Inference timeout for chat responses (including streaming). Go duration syntax, e.g. `"10m"`. |
| `thinking` | string | (unset) | Model reasoning effort level. Supported: `none`, `low`, `medium`, `high`, `auto`. Empty = server default. |
| `extra_context` | string | (empty) | Custom text appended to chat system prompts. Useful for domain-specific details about your house. Currency is handled automatically via `[locale]`. |

### `[extraction.llm]` section

LLM connection settings for the document extraction pipeline. Fully
independent from `[chat.llm]` -- each pipeline has its own provider,
model, and credentials.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable` | bool | `true` | Set to `false` to disable LLM-powered structured extraction. OCR and pdftotext still run. |
| `provider` | string | `ollama` | LLM provider for extraction. Same options as `[chat.llm]`. |
| `base_url` | string | `http://localhost:11434` | API base URL for extraction. |
| `model` | string | `qwen3` | Model for extraction. Extraction works well with small, fast models optimized for structured JSON output. |
| `api_key` | string | (empty) | Authentication credential for extraction. |
| `timeout` | string | `"5m"` | Extraction inference timeout. |
| `thinking` | string | (unset) | Reasoning effort level for extraction. |

### `[documents]` section

Document attachment limits and caching.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `max_file_size` | string or integer | `"50 MiB"` | Maximum file size for document imports. Accepts unitized strings (`"50 MiB"`, `"1.5 GiB"`) or bare integers (bytes). Must be positive. |
| `cache_ttl` | string or integer | `"30d"` | Cache lifetime for extracted documents. Accepts `"30d"`, `"720h"`, or bare integers (seconds). Set to `"0s"` to disable eviction. |
| `cache_ttl_days` | integer | -- | Deprecated. Use `cache_ttl` instead. Bare integer interpreted as days. Cannot be set alongside `cache_ttl`. |

### `[extraction]` section

Document extraction pipeline settings.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `max_pages` | int | `0` | Maximum pages to OCR per scanned document. 0 means no limit. |

### `[extraction.ocr]` section

OCR sub-pipeline settings. Requires `tesseract` and `pdftocairo`.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable` | bool | `true` | Set to `false` to disable OCR on documents. When disabled, scanned pages and images produce no text. |

### `[extraction.ocr.tsv]` section

Spatial layout annotations (line-level bounding boxes) from tesseract OCR.
Improves extraction accuracy for invoices and forms with tabular data,
at ~2x token overhead.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enable` | bool | `true` | Set to `false` to disable spatial annotations sent to the LLM. |
| `confidence_threshold` | int | `70` | Confidence threshold (0-100). Lines with OCR confidence below this value include a confidence score; lines above omit it to save tokens. Set to 0 to never show confidence. |

### `[locale]` section

Locale and currency settings. Controls currency formatting across all money
fields in the application.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `currency` | string | (auto-detect) | ISO 4217 currency code (e.g. `USD`, `EUR`, `GBP`, `JPY`). Auto-detected from `LC_MONETARY`/`LANG` if not set, falls back to `USD`. Persisted to the database on first run -- after that the DB value is authoritative. |

Currency resolution order (highest to lowest):

1. Database value (authoritative once set -- makes the DB file portable)
2. `MICASA_LOCALE_CURRENCY` environment variable
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
`api_key` in `[chat.llm]` and/or `[extraction.llm]`. Cloud providers use
their own default base URLs when none is configured.

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

The `extra_context` field in `[chat.llm]` is injected into every system
prompt sent to the chat LLM, giving it persistent knowledge about your
situation:

```toml
[chat.llm]
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
| Currency | USD | Set via `[locale] currency` in config, `MICASA_LOCALE_CURRENCY` env var, or auto-detected from system locale. Persisted to the database on first use |
