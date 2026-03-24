<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Postal Code First: Form Reorder + Auto-fill

**Issue:** [#793](https://github.com/micasa-dev/micasa/issues/793)
**Date:** 2026-03-24

## Problem

The house profile form asks for address fields in traditional order
(street, city, state, ZIP). Entering the postal code first lets the app
auto-fill city and state, reducing keystrokes and errors.
See <https://zipcodefirst.com/>.

## Design

### 1. Form Field Reorder

Move "Postal code" from last in the address group to immediately after
"Nickname" in the Basics group. New input order:

1. Nickname (required)
2. Postal code
3. Address line 1
4. Address line 2
5. City
6. State

Display order in `formatAddress` stays conventional (street, city, state,
postal code) -- the reorder only affects the input form.

**Files:**
- `internal/app/forms.go` -- `startHouseForm()` field order,
  `houseFormData` struct field order

### 2. Auto-fill via zippopotam.us

When the user tabs away from the postal code field (blur), fire an async
`tea.Cmd` that calls `api.zippopotam.us/{country}/{postal_code}`.

On success, deliver a `postalCodeLookupMsg` containing city and state.
The Update handler fills City and State **only if they are currently
empty** -- never overwrite user-provided values.

**API response shape:**

```json
{
  "country": "United States",
  "country abbreviation": "US",
  "post code": "90210",
  "places": [
    {
      "place name": "Beverly Hills",
      "state": "California",
      "state abbreviation": "CA"
    }
  ]
}
```

When multiple places are returned, use the first result.

**Trigger rules:**
- Fire on postal code field blur, not on each keystroke.
- Minimum length gate: at least 3 characters before making the request.
  This avoids wasting calls on partials like "90" while still supporting
  short international codes (e.g., UK "SW1").
- The two fields between postal code and city (address line 1, address
  line 2) provide natural buffer time for the async response (~100-300ms
  typical) to arrive before the user reaches city.

**Error handling:**
- 404 (unknown postal code): silent -- user fills city/state manually.
- Network error / timeout (3s): silent -- log at debug level only.
- No status bar message on failure. Silence is success; "no match" is
  not an error.

### 3. Configuration

New top-level config section in `config.go`:

```toml
[address]
autofill = true
```

- `autofill` (default `true`): When false, postal code is still first in
  the form but no API call is made. Allows users who are offline or
  privacy-conscious to disable the network call.

Env var override: `MICASA_ADDRESS_AUTOFILL`.

Country is auto-detected from locale (see Section 6), not configurable.

### 4. New Package: `internal/address/`

- `lookup.go` -- `Lookup(ctx context.Context, client *http.Client, baseURL, country, postalCode string) (*Result, error)`
  - `Result` struct: `City string`, `State string`
  - Uses the provided `http.Client` (allows test injection)
  - 3-second context timeout
- Response types for JSON unmarshalling (unexported)

### 5. Bubble Tea Integration

- New message type `postalCodeLookupMsg` with City, State, and Err fields.
- `lookupPostalCodeCmd(client *http.Client, baseURL, country, postalCode string) tea.Cmd`
  -- makes the HTTP call, returns the message.
- Add an `addressClient *http.Client` field to `Model`, initialized with
  `&http.Client{Timeout: 5 * time.Second}` at startup. Tests inject a
  mock via `httptest.NewServer`.
- **Blur detection:** `huh` does not expose per-field blur callbacks.
  Instead, track the previously focused field index on the form. In
  `Update`, after forwarding a `tea.KeyMsg` to the form, compare the
  current focused field index to the previous one. When the postal code
  field loses focus (index changed away from it), dispatch the lookup
  command if the value is >= 3 characters and autofill is enabled.
- On receiving `postalCodeLookupMsg`, fill City and State form field
  values if they are currently empty. Use the state abbreviation (e.g.,
  "CA" not "California") since that matches what users typically type
  for US addresses and is shorter.

### 6. Country Auto-detection

Auto-detect at startup, no config key:

1. Check `LANG` / `LC_ALL` env vars for a locale like `en_US.UTF-8` --
   extract the country portion and lowercase it.
2. Fall back to `"us"` if detection fails.

This lives in `internal/config/` alongside locale detection. The resolved
country code is stored on `Model` and passed to the lookup command.

### 7. Testing

- **Unit test** (`internal/address/lookup_test.go`): mock HTTP server
  returning valid, 404, and timeout responses. Verify `Lookup` returns
  correct city/state, nil on 404, error on timeout.
- **User-flow test**: open house form, type postal code "90210", tab to
  next field, verify City and State auto-filled with "Beverly Hills" and
  "CA" (using mock HTTP, not real API).
- **No-overwrite test**: open house form with existing city/state, type
  postal code, verify city/state unchanged.
- **Autofill disabled test**: set config `autofill = false`, type postal
  code, verify no HTTP call.
- **Partial postal code test**: type "90" (below minimum length), tab
  away, verify no HTTP call.
- **Reorder test**: verify the form field order is postal code before
  address lines.

## Non-goals

- Adding a Country field to HouseProfile.
- Validating postal codes (we accept whatever the user types).
- Overwriting user-provided city/state values.
- Disambiguating postal codes that map to multiple places (we use the
  first result; the user can edit if needed).
