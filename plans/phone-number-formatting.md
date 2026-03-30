<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Phone Number Formatting for Vendor Display

**Issue:** micasa-dev/micasa#860
**Date:** 2026-03-30

## Problem

Phone numbers in the vendor table are displayed as raw unformatted strings.
They should be formatted for readability using locale-appropriate conventions.

## Design

### Data Model

Add a `Locale` field to `data.Vendor`:

```go
Locale string `json:"locale"` // ISO 3166-1 alpha-2, e.g. "US", "GB"; empty = system default
```

The `json` tag must not use `omitempty` — the oplog test suite
(`oplog_test.go`) validates that all exported model fields have plain
`json:"snake_case"` tags for correct sync payload serialization.

GORM auto-migrates the column. Run `go generate ./internal/data/` after
adding the field — `genmeta` derives `ColLocale` automatically from the
struct AST. Empty string means "use system locale" via
`config.DetectCountry()`. The column is **not** exposed in the vendor table
UI for now — no entry in `vendorColumnDefs`.

Existing vendors get the column with empty default — correct behavior since
empty = system default. No data migration needed.

**Form round-trip**: Add `Locale` to `vendorFormData` even though it's not
shown in the UI. `vendorFormValues()` copies it from the DB record;
`parseVendorFormData()` carries it through to the `data.Vendor` struct.
Without this, `UpdateVendor` (which uses `Select("*")`) would wipe the
Locale back to empty on every edit.

**FindOrCreateVendor**: `store_vendor.go` `FindOrCreateVendor` uses an
explicit `updates` map for contact fields that extraction callers can clear.
Do **not** add `ColLocale` to this map — extraction callers don't set Locale,
so including it would wipe a manually-set Locale to empty on every
find-or-create call. Locale is set correctly on initial CREATE (from the full
struct) and preserved on subsequent lookups.

### Dependency

Add `github.com/nyaruka/phonenumbers` (no `/v2` suffix) — the standard Go
port of Google's libphonenumber. Handles parsing, validation, and formatting
for all countries. Actively maintained (v1.6.12 released 2026-03-17).

Key API: `phonenumbers.Parse(number, region)` returns `(*PhoneNumber, error)`;
`phonenumbers.Format(num, phonenumbers.NATIONAL)` or `phonenumbers.INTERNATIONAL`.

**Binary size**: Adds ~7 MB to the 55 MB binary (embedded XML metadata for
all countries). A lighter alternative (`dongri/phonenumber`, +0.6 MB) exists
but only does E.164 normalization — it cannot produce locale-specific
NATIONAL formatting like `(555) 123-4567`.

### Cell Kind

New `cellTelephoneNumber` in the `cellKind` iota block (`types.go`). Used
instead of `cellText` for the vendor phone column in `vendorRows()`.

### Formatting Function

New file `internal/locale/phone.go`:

```go
// FormatPhoneNumber formats a phone number string for display.
// regionCode is an uppercase ISO 3166-1 alpha-2 code (e.g. "US").
// Returns the original string unmodified if parsing fails.
func FormatPhoneNumber(number, regionCode string) string
```

Behavior:
- Empty or whitespace-only input returns empty string immediately.
- Parse `number` with `regionCode` as the default region.
- If the parsed number's region matches `regionCode`, format as NATIONAL
  (e.g. `(555) 123-4567` for US). This includes numbers entered with an
  explicit same-region prefix (e.g. `+15551234567` with region `US` →
  national `(555) 123-4567`). National format is more readable for
  same-region users; the raw value with prefix is preserved in the DB.
- If the regions differ (number has explicit international prefix), format as
  INTERNATIONAL (e.g. `+44 20 7946 0958`).
- If parsing fails, return the original string unmodified — no data loss.

### Locale Resolution

Add a `defaultRegion string` parameter to `vendorRows()`. The caller
(`vendorHandler.Load`) computes it once via
`strings.ToUpper(config.DetectCountry())`.

Per-vendor resolution inside `vendorRows()`:
1. Use `strings.ToUpper(vendor.Locale)` if non-empty.
2. Fall back to `defaultRegion` (already uppercased by caller).
3. Pass the resolved region code to `FormatPhoneNumber()`.

Both paths uppercase-normalize because `phonenumbers.Parse` requires
uppercase region codes and nothing enforces casing on the stored Locale.

The cell stores the formatted display string. The raw value stays in the
database unchanged.

### Sorting

`cellTelephoneNumber` falls to the `default` branch in `cellSortCmp`
(text sort). Sorting formatted phone numbers alphabetically is adequate —
phone-number sort order is rarely meaningful. No special comparator needed.

### Fake Data

`fake.Vendor()` leaves `Locale` empty (system default). This is correct
for demo data — the formatting uses `DetectCountry()` which defaults to
`"us"`.

### Other Display Surfaces

Phone numbers also appear unformatted in two places outside the main table:

- **CLI `show vendors`** (`cmd/micasa/show.go`): The table-output column
  (`vendorCols`) uses raw `vendor.Phone`. Apply `FormatPhoneNumber` with the
  same locale resolution (vendor Locale → `DetectCountry()` fallback). Leave
  `vendorToMap` (JSON output) unchanged — JSON should carry raw values for
  machine consumption.
- **Extraction preview** (`extraction_render.go`): The phone column uses
  `fmtAnyText` (passthrough). Add a `fmtPhone` formatter that calls
  `FormatPhoneNumber` with the system default region. Extraction data comes
  from the LLM before commit, so vendor Locale is unavailable — system
  default is the best we can do.

### Sync Compatibility

Old clients (without Locale column) silently drop the field from incoming
oplog entries — no error, no corruption. New clients treat missing Locale as
empty string (system default). Forward and backward compatibility are safe.
Specifically, `applyUpdate` uses `tx.Table(...).Updates(map)` which only
generates `SET` clauses for keys present in the map — an old client's payload
without `"locale"` will not touch the column, preserving the stored value.

### Inline Editing

When editing a phone cell inline, the user edits the raw stored value (not the
formatted version). On save, the raw value goes to the DB unchanged.

### Column Width

Current phone column spec: `Min: 12, Max: 16`. International formatted numbers
can exceed 16 chars (e.g. `+44 20 7946 0958` = 16). Bump `Max` to 20 to
accommodate international formats.

### Rendering

The table renderer handles `cellTelephoneNumber` the same as `cellText` for
styling — the formatting is already applied when building rows. All existing
`cellKind` branch points (15+ locations: `cellStyle`, `compareCells`,
`magFormat`, `renderCell`, `enterHint`, `editHint`, `compactMoneyCells`,
`cellDisplayValue`, etc.) either use `default:` branches or if-else chains
that fall through correctly. The linter currently uses
`default-signifies-exhaustive: true`, so no exhaustive-switch updates are
needed. Note: #870 is switching this to `false` — if that lands first, this
PR will need explicit `case cellTelephoneNumber:` entries at each branch
point.

The distinct cell kind exists for:
- Semantic clarity in the type system.
- Future extensibility (e.g. click-to-call, validation indicators).

**Housekeeping**: Update the comment in `mag.go:29-30` which says "cellText
is excluded because it covers phone numbers" — phone numbers now have their
own kind.

### Testing

**Unit tests** (`internal/locale/phone_test.go`) for `FormatPhoneNumber()`:
- US national: `"5551234567"` + `"US"` → `"(555) 123-4567"`
- UK national: `"02079460958"` + `"GB"` → `"020 7946 0958"`
- International prefix: `"+442079460958"` + `"US"` → `"+44 20 7946 0958"`
- Same-region prefix: `"+15551234567"` + `"US"` → `"(555) 123-4567"`
- Already-formatted: `"(555) 123-4567"` + `"US"` → `"(555) 123-4567"`
- Garbage input: `"not a phone"` + `"US"` → `"not a phone"` (passthrough)
- Empty string: `""` + `"US"` → `""`
- Whitespace only: `"  "` + `"US"` → `""`

**User-flow tests** (`internal/app/vendor_test.go` or similar):
- Create vendor via form with raw phone `"5551234567"`, submit, reload tab,
  assert cell value is `"(555) 123-4567"` and cell kind is
  `cellTelephoneNumber`.
- Inline edit phone cell: open inline editor, verify the field contains the
  raw DB value (not formatted), change it, save, verify the table re-renders
  with the new formatted value.
- Locale fallback: create vendor with empty Locale, verify formatting uses
  `config.DetectCountry()` default.

**Form round-trip test**: Edit an existing vendor (change name only), verify
the Locale field is preserved (not wiped to empty).

**CLI test**: `show vendors` table output includes formatted phone numbers.

**CLI JSON regression test**: `show vendors --json` output carries raw
unformatted phone values (guards against accidental formatting in JSON path).

**Extraction preview test**: Extraction preview for a vendor shows formatted
phone number using system default region.
