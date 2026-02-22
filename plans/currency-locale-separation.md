<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Currency/Locale Separation

## Problem

The current `locale.Currency` type conflates two independent concerns:

1. **Currency unit** (what money is this?) -- the ISO 4217 code (EUR, USD)
2. **Formatting locale** (how do we display numbers?) -- grouping/decimal
   separators, symbol placement

`Resolve("EUR")` implicitly picks German formatting via the `currencyLocales`
map. A French user with EUR gets `1.234,56 EUR` (German style) instead of
`1 234,56 EUR` (French style). Currency and locale are independent axes -- just
like timestamps (stored in UTC) and timezones (display concern).

## Design

### Mental model: timestamp/timezone analogy

| Concept          | Timestamp analogy | Currency analogy                  |
|------------------|-------------------|-----------------------------------|
| Stored value     | UTC               | Currency code (EUR) in DB         |
| Display setting  | Timezone           | Formatting locale from LANG/LC_*  |
| Persisted?       | Yes (UTC)          | Code: yes. Locale: no.            |

### API changes

**Before:**
```go
func Resolve(code string) (Currency, error)
func MustResolve(code string) Currency
func ResolveDefault(configured string) (Currency, error)
```

**After:**
```go
func Resolve(code string, tag language.Tag) (Currency, error)
func MustResolve(code string, tag language.Tag) Currency
func ResolveDefault(configured string) (Currency, error)  // detects locale internally
func DetectLocale() language.Tag                           // new
func DefaultCurrency() Currency                            // unchanged (test convenience)
```

### Deleted

- `currencyLocales` map -- the static currency-to-locale mapping
- `localeForCurrency()` function

### New: `DetectLocale()`

Reads the user's formatting locale from the environment:

```
LC_MONETARY > LC_ALL > LANG > language.AmericanEnglish
```

Parses POSIX locale strings (e.g. `fr_FR.UTF-8`) into BCP 47 tags.
Never persisted -- ephemeral per session.

### Cached separators

Grouping and decimal separators are computed once at construction time
instead of on every `FormatCents`/`parseCents` call.

### Store changes

`Store.ResolveCurrency(configured)` now:
1. Reads currency code from DB (or detects + persists)
2. Calls `locale.DetectLocale()` for the formatting tag
3. Passes both to `locale.Resolve(code, tag)`

## Examples

| User locale | DB currency | Formatting              |
|-------------|-------------|-------------------------|
| fr_FR       | EUR         | `1 234,56 EUR` (French) |
| de_DE       | EUR         | `1.234,56 EUR` (German) |
| en_US       | EUR         | `EUR1,234.56` (American) |
| en_US       | USD         | `$1,234.56`             |
| ja_JP       | JPY         | yen sign + `1,500.00`    |

## Out of scope

- Zero-decimal currency handling (JPY `.00`) -- separate follow-up
- Currency change CLI command -- separate follow-up
- Exchange rate conversion -- separate follow-up
