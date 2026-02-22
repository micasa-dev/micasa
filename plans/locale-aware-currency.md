<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Locale-Aware Currency Formatting

Issue: #407

## Problem

All money formatting is hardcoded USD (`$1,234.56`). Users in other locales
need correct symbols, placement, and separators (e.g. `1.234,56 €`,
`£1,234.56`).

## Key Design Constraint: Database Portability

Currency is a property of the **database**, not the viewer's locale. If a
user in Germany records expenses in euros and sends the DB to a friend in
the US, the friend must see `€` amounts — not `$`. Showing a different
currency symbol without an exchange rate would silently display wrong values.

This means the currency code is **stored in SQLite** (settings table or
house profile). Config/locale only provides the default for first-run /
unset databases.

## Approach: `golang.org/x/text/currency` + `golang.org/x/text/message`

Already an indirect dep (`golang.org/x/text` v0.34.0). Promote to direct.
These packages implement CLDR formatting rules — symbol placement, decimal
vs thousands separators, narrow vs standard symbols — without us hand-rolling
any of it.

`currency.NarrowSymbol` resolves to the actual symbol glyph (`$`, `€`, `£`,
`¥`) — never the ISO code. We always prefer symbols over codes in all display
surfaces (headers, cells, form hints, house profile).

## Resolution Order (for initial/default currency only)

When the DB has no currency set yet:

1. **Env var** `MICASA_CURRENCY` — e.g. `EUR`, `CAD`, `GBP`
2. **TOML** `[locale] currency = "EUR"`
3. **Auto-detect** from `LC_MONETARY` / `LC_ALL` / `LANG` env
4. **Fallback** `USD`

Once resolved, the value is persisted to the DB. From that point forward, the
DB value is authoritative.

## Storage

Add a `currency` text column to the settings/app_settings table (or house
profile if that's the natural home). Stores the ISO 4217 code (`USD`, `EUR`,
etc.) — the code is the storage key, the symbol is derived at display time.

Single-file backup principle preserved: currency lives in SQLite.

## New package: `internal/locale`

```go
package locale

// Currency holds resolved currency formatting state.
type Currency struct {
    Unit   currency.Unit // e.g. currency.USD, currency.EUR
    symbol string        // cached narrow symbol: "$", "€", "£", "¥"
}

// Resolve applies the resolution order to produce a Currency.
// code is the ISO 4217 string (from DB, env, config, or auto-detect).
func Resolve(code string) (Currency, error) { ... }

// FormatCents formats int64 cents with the correct symbol, placement,
// and separators per CLDR rules.
func (c Currency) FormatCents(cents int64) string { ... }

// FormatCompactCents formats using abbreviated notation (1.2k, 45k, 1.3M)
// with the correct currency symbol.
func (c Currency) FormatCompactCents(cents int64) string { ... }

// Symbol returns the narrow symbol glyph ("$", "€", "£", "¥") for column
// header annotations. Always a symbol, never an ISO code.
func (c Currency) Symbol() string { ... }

// ParseCents parses user input, stripping the currency symbol if present.
// Bare numbers always accepted.
func (c Currency) ParseCents(input string) (int64, error) { ... }
```

## Surfaces to Update

### Formatting (internal/data/validation.go)

- `FormatCents` / `FormatOptionalCents` — become thin wrappers or move to
  methods on `locale.Currency`
- `FormatCompactCents` / `FormatCompactOptionalCents` — same
- `parseCents` — accept the configured symbol (strip it like we strip `$`
  today), but remain lenient (bare numbers always work)

### Display

- `annotateMoneyHeaders` (`compact.go:42`) — use `Currency.Symbol()` instead
  of hardcoded `"$"`
- `compactMoneyValue` (`compact.go:81`) — strip the right symbol
- `headerTitleWidth` (`table.go:278`) — account for symbol width (may be
  multi-byte for `€`)
- `centsValue` / `centsCell` (`tables.go:874-895`) — pass currency
- House profile display (`house.go:94, 257`)

### Forms

- `moneyError` (`forms.go:2111`) — hint should show locale-appropriate example
- Form value population (lines 2136-2225) — use currency-aware formatting

### Config

- `config.go` — add `Locale` struct, env override, example TOML
- Thread `Currency` from config load + DB through `App` init into all
  formatters

## Threading Strategy

The `App` struct holds the resolved `locale.Currency`. On startup:
1. Load config (env/TOML/auto-detect)
2. Check DB for stored currency
3. If DB has one, use it (authoritative). If not, use config default and
   persist it to DB.
4. Pass `Currency` through to all formatting call sites.

## What We Skip

- Exchange rates / conversion (out of scope per issue)
- Per-field currency (all fields use the same database-wide currency)
- Measurement units (#394 is separate)

## Test Plan

- Unit tests for `locale.Resolve` with various inputs (code, env, empty)
- Unit tests for formatting: USD, EUR, GBP, JPY (zero-decimal currency)
- Roundtrip tests: format -> parse for each currency
- Config loading tests with `[locale]` section
- DB persistence: store EUR, reopen, confirm EUR is used
- Portability: DB with EUR opened with USD config still shows EUR
- Existing tests updated to pass explicit USD (no behavior change for defaults)
