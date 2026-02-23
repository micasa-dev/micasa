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
micasa config <key>
```

Print the resolved value of a configuration key (dot-delimited TOML path):

```sh
micasa config llm.model             # current model name
micasa config llm.base_url          # LLM API endpoint
micasa config documents.max_file_size  # max doc size in bytes
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
| `OLLAMA_HOST` | `http://localhost:11434/v1` | `llm.base_url` | LLM API base URL |
| `MICASA_LLM_MODEL` | `qwen3` | `llm.model` | LLM model name |
| `MICASA_LLM_TIMEOUT` | `5s` | `llm.timeout` | LLM operation timeout |
| `MICASA_MAX_DOCUMENT_SIZE` | `50 MiB` | `documents.max_file_size` | Max document import size |
| `MICASA_CACHE_TTL` | `30d` | `documents.cache_ttl` | Document cache lifetime |
| `MICASA_CACHE_TTL_DAYS` | -- | `documents.cache_ttl_days` | Deprecated; use `MICASA_CACHE_TTL` |
| `MICASA_EXTRACTION_MODEL` | (chat model) | `extraction.model` | LLM model for document extraction |
| `MICASA_EXTRACTION_ENABLED` | `true` | `extraction.enabled` | Enable/disable LLM extraction |
| `MICASA_EXTRACTION_THINKING` | `false` | `extraction.thinking` | Enable model thinking for extraction |
| `MICASA_TEXT_TIMEOUT` | `30s` | `extraction.text_timeout` | pdftotext timeout |
| `MICASA_MAX_EXTRACT_PAGES` | `20` | `extraction.max_extract_pages` | Max pages to OCR per document |
| `MICASA_LLM_THINKING` | (unset) | `llm.thinking` | Enable model thinking for chat |

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
# Base URL for an OpenAI-compatible API endpoint.
# Ollama (default): http://localhost:11434/v1
# llama.cpp:        http://localhost:8080/v1
# LM Studio:        http://localhost:1234/v1
base_url = "http://localhost:11434/v1"

# Model name passed in chat requests.
model = "qwen3"

# Optional: custom context appended to all system prompts.
# Use this to inject domain-specific details about your house, currency, etc.
# extra_context = "My house is a 1920s craftsman in Portland, OR. All budgets are in CAD."

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

# Maximum pages to OCR for scanned documents. Default: 20.
# max_extract_pages = 20

# Set to false to disable LLM-powered extraction. Text extraction and OCR
# still run independently.
# enabled = true

# Enable model thinking for extraction. Default: false (faster, no <think>).
# thinking = false
```

### `[llm]` section

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `base_url` | string | `http://localhost:11434/v1` | Root URL of an OpenAI-compatible API. micasa appends `/chat/completions`, `/models`, etc. |
| `model` | string | `qwen3` | Model identifier sent in chat requests. Must be available on the server. |
| `extra_context` | string | (empty) | Free-form text appended to all LLM system prompts. Useful for telling the model about your house, preferred currency, or regional conventions. |
| `timeout` | string | `"5s"` | Max wait time for quick LLM operations (ping, model listing). Go duration syntax, e.g. `"10s"`, `"500ms"`. Increase for slow servers. |
| `thinking` | bool | (unset) | Enable model thinking mode (e.g. qwen3 `<think>` blocks). Unset = don't send the option (server default). |

### `[documents]` section

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `max_file_size` | string or integer | `"50 MiB"` | Maximum file size for document imports. Accepts unitized strings (`"50 MiB"`, `"1.5 GiB"`) or bare integers (bytes). Must be positive. |
| `cache_ttl` | string or integer | `"30d"` | Cache lifetime for extracted documents. Accepts `"30d"`, `"720h"`, or bare integers (seconds). Set to `"0s"` to disable eviction. |
| `cache_ttl_days` | integer | -- | Deprecated. Use `cache_ttl` instead. Bare integer interpreted as days. Cannot be set alongside `cache_ttl`. |

### `[extraction]` section

Controls the document extraction pipeline. Text extraction and OCR are
independent and always available when their tools are installed. The LLM layer
adds structured data extraction (document type, costs, dates, vendor matching).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `model` | string | (chat model) | Model for document extraction. Falls back to `llm.model` if empty. A small, fast model (e.g. `qwen2.5:7b`) works well. |
| `text_timeout` | string | `"30s"` | Max time for `pdftotext` to run. Go duration syntax, e.g. `"1m"`. Increase for very large PDFs. |
| `max_extract_pages` | int | `20` | Maximum pages to OCR per scanned document. Front-loaded info is typically in the first pages. |
| `enabled` | bool | `true` | Set to `false` to disable LLM-powered extraction. Text extraction and OCR still run. |
| `thinking` | bool | `false` | Enable model thinking mode for extraction. Disable for faster structured output. |

### Supported LLM backends

micasa talks to any server that implements the OpenAI chat completions API
with streaming (SSE). [Ollama](https://ollama.com) is the primary tested
backend:

| Backend | Default URL | Notes |
|---------|-------------|-------|
| [Ollama](https://ollama.com) | `http://localhost:11434/v1` | Default and tested. Models are pulled automatically if not present. |
| [llama.cpp server](https://github.com/ggml-org/llama.cpp) | `http://localhost:8080/v1` | Should work (untested). Pass `--host` and `--port` when starting the server. |
| [LM Studio](https://lmstudio.ai) | `http://localhost:1234/v1` | Should work (untested). Enable the local server in LM Studio settings. |

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
All costs are in USD. Property tax is assessed annually in November.
The HVAC system is a heat pump (Mitsubishi hyper-heat) -- no gas furnace.
"""
```

This helps the model give more relevant answers without you repeating context
in every question.

## Persistent preferences

Some preferences are stored in the SQLite database and persist across
restarts. These are controlled through the UI rather than config files:

| Preference | Default | How to change |
|------------|---------|---------------|
| Dashboard on startup | Shown | Press `D` to toggle; your choice is remembered |
| LLM model | From config | Changed automatically when you switch models in the chat interface |
