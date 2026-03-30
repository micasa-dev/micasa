<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Phone Number Formatting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Format vendor phone numbers for display using locale-appropriate
conventions (national/international) powered by `nyaruka/phonenumbers`.

**Architecture:** Add `FormatPhoneNumber` in `internal/locale/phone.go`,
a new `cellTelephoneNumber` kind, and a `Locale` field on `data.Vendor`.
Formatting happens at row-build time; the DB stores raw values. All display
surfaces (TUI table, CLI `show`, extraction preview) call the same function.

**Tech Stack:** Go, `github.com/nyaruka/phonenumbers`, GORM auto-migration,
`genmeta` code generation.

**Spec:** `plans/phone-number-formatting.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/locale/phone.go` | `FormatPhoneNumber` function |
| Create | `internal/locale/phone_test.go` | Unit tests for `FormatPhoneNumber` |
| Modify | `internal/data/models.go:144-156` | Add `Locale` field to `Vendor` struct |
| Regen  | `internal/data/meta_generated.go` | `go generate` adds `ColLocale` |
| Modify | `internal/app/types.go:374-386` | Add `cellTelephoneNumber` to iota |
| Modify | `internal/app/coldefs.go:221` | Bump phone column `Max` to 20, set `Kind` |
| Modify | `internal/app/tables.go:309-332` | Add `defaultRegion` param, format phone, use new cell kind |
| Modify | `internal/app/handlers.go:485-498` | Pass `defaultRegion` to `vendorRows` |
| Modify | `internal/app/forms.go:122-129` | Add `Locale` to `vendorFormData` |
| Modify | `internal/app/forms.go:1093-1106` | Carry `Locale` through `parseVendorFormData` |
| Modify | `internal/app/forms.go:1148-1157` | Copy `Locale` in `vendorFormValues` |
| Modify | `internal/app/mag.go:29-30` | Update comment (phone numbers have own kind) |
| Modify | `cmd/micasa/show.go:446-451` | Format phone in CLI table output |
| Modify | `internal/app/extraction_render.go:710` | Add `fmtPhone` formatter |
| Test   | `internal/app/vendor_test.go` (or new) | User-flow TUI tests |
| Test   | `cmd/micasa/show_test.go` (or similar) | CLI output tests |

---

### Task 1: FormatPhoneNumber — tests first

**Files:**
- Create: `internal/locale/phone_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package locale_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/micasa-dev/micasa/internal/locale"
)

func TestFormatPhoneNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		number string
		region string
		want   string
	}{
		{"US national", "5551234567", "US", "(555) 123-4567"},
		{"UK national", "02079460958", "GB", "020 7946 0958"},
		{"international prefix", "+442079460958", "US", "+44 20 7946 0958"},
		{"same-region prefix", "+15551234567", "US", "(555) 123-4567"},
		{"already formatted", "(555) 123-4567", "US", "(555) 123-4567"},
		{"garbage passthrough", "not a phone", "US", "not a phone"},
		{"empty string", "", "US", ""},
		{"whitespace only", "   ", "US", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := locale.FormatPhoneNumber(tt.number, tt.region)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/locale/ -run TestFormatPhoneNumber -v
```

Expected: FAIL — `FormatPhoneNumber` not defined (compilation error).

---

### Task 2: FormatPhoneNumber — implementation

**Files:**
- Create: `internal/locale/phone.go`

- [ ] **Step 1: Write the implementation**

```go
// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package locale

import (
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// FormatPhoneNumber formats a phone number string for display.
// regionCode is an uppercase ISO 3166-1 alpha-2 code (e.g. "US").
// Returns the original string unmodified if parsing fails.
func FormatPhoneNumber(number, regionCode string) string {
	trimmed := strings.TrimSpace(number)
	if trimmed == "" {
		return ""
	}
	parsed, err := phonenumbers.Parse(trimmed, regionCode)
	if err != nil {
		return number
	}
	if phonenumbers.GetRegionCodeForNumber(parsed) == regionCode {
		return phonenumbers.Format(parsed, phonenumbers.NATIONAL)
	}
	return phonenumbers.Format(parsed, phonenumbers.INTERNATIONAL)
}
```

- [ ] **Step 2: Add the dependency and tidy**

The import of `nyaruka/phonenumbers` in `phone.go` is the first reference.
Run `go mod tidy` to add it to `go.mod` and `go.sum`:

```bash
go mod tidy
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/locale/ -run TestFormatPhoneNumber -v
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```
feat(locale): add FormatPhoneNumber with international support
```

---

### Task 3: Add Locale field to Vendor model

**Files:**
- Modify: `internal/data/models.go:144-156`
- Regen: `internal/data/meta_generated.go`

- [ ] **Step 1: Add Locale field to Vendor struct**

In `internal/data/models.go`, after the `Notes` field (line 151) and before
`Documents` (line 152), add:

```go
Locale  string         `                                                                             json:"locale"`
```

Keep the tag alignment consistent with the surrounding fields.

- [ ] **Step 2: Run code generation**

```bash
go generate ./internal/data/
```

Verify `ColLocale` appears in `internal/data/meta_generated.go`:

```bash
rg ColLocale internal/data/meta_generated.go
```

Expected: `ColLocale = "locale"` in the constants block.

- [ ] **Step 3: Verify the build compiles**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```
feat(data): add Locale field to Vendor model
```

---

### Task 4: Add cellTelephoneNumber kind

**Files:**
- Modify: `internal/app/types.go:374-386`
- Modify: `internal/app/mag.go:29-30`

- [ ] **Step 1: Add cellTelephoneNumber to the iota**

In `internal/app/types.go`, after `cellOps` (line 385), add:

```go
cellTelephoneNumber // formatted phone number; passthrough for styling
```

- [ ] **Step 2: Update mag.go comment**

In `internal/app/mag.go`, replace lines 28-30:

```go
// Only transform kinds that carry meaningful numeric data.
// cellText is excluded because it covers serial numbers,
// model numbers, and other identifiers that happen to look numeric.
```

(Removed "phone numbers" from the comment since they now have
`cellTelephoneNumber`.)

- [ ] **Step 3: Verify the build compiles**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```
feat(ui): add cellTelephoneNumber kind
```

---

### Task 5: Wire phone formatting into vendor table — tests first

**Files:**
- Test: `internal/app/vendor_test.go` (or new file
  `internal/app/phone_format_test.go`)

- [ ] **Step 1: Write user-flow test for formatted phone in table**

```go
func TestVendorPhoneFormattedInTable(t *testing.T) {
	// Not parallel: t.Setenv modifies process-global state.
	// Force US default so the UK phone only formats correctly when the
	// implementation actually consults Vendor.Locale (not the system default).
	t.Setenv("LC_ALL", "en_US.UTF-8")
	m := newTestModelWithStore(t)

	require.NoError(t, m.store.CreateVendor(&data.Vendor{
		Name:   "Phone Test Co",
		Phone:  "02079460958",
		Locale: "GB",
	}))

	// Reload the vendor tab.
	for i, tab := range m.tabs {
		if tab.Kind == tabVendors {
			m.active = i
			break
		}
	}
	require.NoError(t, m.reloadActiveTab())

	tab := m.activeTab()
	require.NotEmpty(t, tab.Rows)
	require.NotEmpty(t, tab.CellRows)

	// Find the phone cell (vendorColPhone).
	phoneCell := tab.CellRows[0][int(vendorColPhone)]
	assert.Equal(t, "020 7946 0958", phoneCell.Value)
	assert.Equal(t, cellTelephoneNumber, phoneCell.Kind)
}
```

- [ ] **Step 2: Write test for Locale round-trip on edit**

This test must go through the real edit-form flow (`startEditVendorForm` →
`vendorFormValues` → user edits → `submitVendorForm`) rather than setting
`m.fs.formData` directly. The round-trip works because `vendorFormValues`
copies Locale from the DB record, and no huh.Form field overwrites it.

```go
func TestVendorLocalePreservedOnEdit(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create vendor with a Locale set.
	require.NoError(t, m.store.CreateVendor(&data.Vendor{
		Name:   "UK Plumber",
		Phone:  "02079460958",
		Locale: "GB",
	}))

	vendors, err := m.store.ListVendors(false)
	require.NoError(t, err)
	require.Len(t, vendors, 1)
	id := vendors[0].ID

	// Open the real edit form — this calls vendorFormValues(vendor)
	// which copies Locale from the DB record into formData.
	require.NoError(t, m.startEditVendorForm(id))
	m.fs.form.Init()

	// Change the name through the form data (simulates user editing).
	values, ok := m.fs.formData.(*vendorFormData)
	require.True(t, ok)
	values.Name = "UK Plumber Renamed"

	// Submit — parseVendorFormData carries Locale through.
	require.NoError(t, m.submitVendorForm())

	// Reload and verify Locale is preserved.
	updated, err := m.store.GetVendor(id)
	require.NoError(t, err)
	assert.Equal(t, "GB", updated.Locale)
	assert.Equal(t, "UK Plumber Renamed", updated.Name)
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/app/ -run "TestVendorPhoneFormattedInTable|TestVendorLocalePreservedOnEdit" -v
```

Expected: FAIL — `TestVendorPhoneFormattedInTable` fails because the phone
cell still has `cellText` and raw value `"02079460958"`.
`TestVendorLocalePreservedOnEdit` fails because `vendorFormValues` doesn't
copy Locale yet, so `UpdateVendor` wipes it to `""`.

---

### Task 6: Wire phone formatting into vendor table — implementation

**Files:**
- Modify: `internal/app/coldefs.go:221`
- Modify: `internal/app/tables.go:309-332`
- Modify: `internal/app/handlers.go:485-498`
- Modify: `internal/app/forms.go:122-129`
- Modify: `internal/app/forms.go:1093-1106`
- Modify: `internal/app/forms.go:1148-1157`

- [ ] **Step 1: Update column def — bump Max, set Kind**

In `internal/app/coldefs.go`, change the Phone column (line 221):

```go
{"Phone", columnSpec{Title: "Phone", Min: 12, Max: 20, Kind: cellTelephoneNumber}},
```

- [ ] **Step 2: Update vendorRows — add defaultRegion, format phone**

In `internal/app/tables.go`, change the `vendorRows` function signature and
body. Add `defaultRegion string` parameter and use it:

```go
func vendorRows(
	vendors []data.Vendor,
	quoteCounts map[string]int,
	jobCounts map[string]int,
	docCounts map[string]int,
	defaultRegion string,
) ([]table.Row, []rowMeta, [][]cell) {
	return buildRows(vendors, func(v data.Vendor) rowSpec {
		region := defaultRegion
		if v.Locale != "" {
			region = strings.ToUpper(v.Locale)
		}
		return rowSpec{
			ID:      v.ID,
			Deleted: v.DeletedAt.Valid,
			Cells: []cell{
				{Value: shortID(v.ID), Kind: cellReadonly},
				{Value: v.Name, Kind: cellText},
				{Value: v.ContactName, Kind: cellText},
				{Value: v.Email, Kind: cellText},
				{Value: locale.FormatPhoneNumber(v.Phone, region), Kind: cellTelephoneNumber},
				{Value: v.Website, Kind: cellText},
				{Value: countStr(quoteCounts, v.ID), Kind: cellDrilldown},
				{Value: countStr(jobCounts, v.ID), Kind: cellDrilldown},
				{Value: countStr(docCounts, v.ID), Kind: cellDrilldown},
			},
		}
	})
}
```

Add `"strings"` to the stdlib imports in `tables.go`. (`locale` is already
imported.)

- [ ] **Step 3: Update vendorHandler.Load — pass defaultRegion**

In `internal/app/handlers.go`, update the `Load` method (around line 497):

```go
func (vendorHandler) Load(
	store *data.Store,
	showDeleted bool,
) ([]table.Row, []rowMeta, [][]cell, error) {
	vendors, err := store.ListVendors(showDeleted)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := entityIDs(vendors, func(v data.Vendor) string { return v.ID })
	quoteCounts := fetchCounts(store.CountQuotesByVendor, ids)
	jobCounts := fetchCounts(store.CountServiceLogsByVendor, ids)
	docCounts := fetchDocCounts(store, data.DocumentEntityVendor, ids)
	defaultRegion := strings.ToUpper(config.DetectCountry())
	rows, meta, cellRows := vendorRows(vendors, quoteCounts, jobCounts, docCounts, defaultRegion)
	return rows, meta, cellRows, nil
}
```

Add imports for `config` and `strings`:

```go
"strings"

"github.com/micasa-dev/micasa/internal/config"
```

- [ ] **Step 4: Add Locale to vendorFormData**

In `internal/app/forms.go`, add `Locale` field to `vendorFormData` (around
line 128, after `Notes`):

```go
type vendorFormData struct {
	Name        string
	ContactName string
	Email       string
	Phone       string
	Website     string
	Notes       string
	Locale      string
}
```

- [ ] **Step 5: Carry Locale through parseVendorFormData**

In `internal/app/forms.go`, update `parseVendorFormData` (around line 1098)
to include Locale:

```go
func (m *Model) parseVendorFormData() (data.Vendor, error) {
	values, err := formDataAs[vendorFormData](m)
	if err != nil {
		return data.Vendor{}, err
	}
	return data.Vendor{
		Name:        strings.TrimSpace(values.Name),
		ContactName: strings.TrimSpace(values.ContactName),
		Email:       strings.TrimSpace(values.Email),
		Phone:       strings.TrimSpace(values.Phone),
		Website:     strings.TrimSpace(values.Website),
		Notes:       strings.TrimSpace(values.Notes),
		Locale:      strings.TrimSpace(values.Locale),
	}, nil
}
```

- [ ] **Step 6: Copy Locale in vendorFormValues**

In `internal/app/forms.go`, update `vendorFormValues` (around line 1148):

```go
func vendorFormValues(vendor data.Vendor) *vendorFormData {
	return &vendorFormData{
		Name:        vendor.Name,
		ContactName: vendor.ContactName,
		Email:       vendor.Email,
		Phone:       vendor.Phone,
		Website:     vendor.Website,
		Notes:       vendor.Notes,
		Locale:      vendor.Locale,
	}
}
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/app/ -run "TestVendorPhoneFormattedInTable|TestVendorLocalePreservedOnEdit" -v
```

Expected: PASS.

- [ ] **Step 8: Run full test suite**

```bash
go test -shuffle=on ./...
```

Expected: all PASS.

- [ ] **Step 9: Commit**

```
feat(ui): format vendor phone numbers in table display
```

---

### Task 7: CLI `show vendors` formatting — tests first

**Files:**
- Modify: `cmd/micasa/show_test.go`

Existing tests: `TestShowVendorsText` (line 171) and `TestShowVendorsJSON`
(line 193) use `newTestStoreWithMigration`, `runShow`, and `bytes.Buffer`.

- [ ] **Step 1: Update existing TestShowVendorsText to use a parseable number**

The existing test uses `"555-1234"` which is too short for libphonenumber.
Change it to a full 10-digit number and assert the formatted output:

```go
func TestShowVendorsText(t *testing.T) {
	// Not parallel: t.Setenv modifies process-global state.
	// LC_ALL has highest precedence in DetectCountry().
	t.Setenv("LC_ALL", "en_US.UTF-8")
	store := newTestStoreWithMigration(t)

	require.NoError(t, store.CreateVendor(&data.Vendor{
		Name:        "Acme Plumbing",
		ContactName: "John Doe",
		Email:       "john@acme.com",
		Phone:       "5551234567",
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "vendors", false, false))

	out := buf.String()
	assert.Contains(t, out, "=== VENDORS ===")
	assert.Contains(t, out, "Acme Plumbing")
	assert.Contains(t, out, "John Doe")
	assert.Contains(t, out, "john@acme.com")
	assert.Contains(t, out, "(555) 123-4567")
}
```

- [ ] **Step 2: Add JSON regression test asserting raw phone**

```go
func TestShowVendorsJSONPhoneRaw(t *testing.T) {
	t.Parallel()
	store := newTestStoreWithMigration(t)

	require.NoError(t, store.CreateVendor(&data.Vendor{
		Name:  "Raw Phone Co",
		Phone: "5551234567",
	}))

	var buf bytes.Buffer
	require.NoError(t, runShow(&buf, store, "vendors", true, false))

	var result []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.Len(t, result, 1)
	assert.Equal(t, "5551234567", result[0]["phone"],
		"JSON output must carry raw phone, not formatted")
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./cmd/micasa/ -run "TestShowVendorsText|TestShowVendorsJSONPhoneRaw" -v
```

Expected: `TestShowVendorsText` FAIL (output has `5551234567` not
`(555) 123-4567`). `TestShowVendorsJSONPhoneRaw` PASS (JSON is already raw).

---

### Task 8: CLI `show vendors` formatting — implementation

**Files:**
- Modify: `cmd/micasa/show.go:446-451`

- [ ] **Step 1: Update vendorCols phone formatter**

In `cmd/micasa/show.go`, change the PHONE column (line 450):

```go
{"PHONE", func(v data.Vendor) string {
	region := strings.ToUpper(config.DetectCountry())
	if v.Locale != "" {
		region = strings.ToUpper(v.Locale)
	}
	return fmtStr(locale.FormatPhoneNumber(v.Phone, region))
}},
```

Add imports for `config` and `locale` (`strings` is already imported):

```go
"github.com/micasa-dev/micasa/internal/config"
"github.com/micasa-dev/micasa/internal/locale"
```

Leave `vendorToMap` unchanged — JSON carries raw values.

- [ ] **Step 2: Run tests**

```bash
go test ./cmd/micasa/ -run "TestShow" -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```
feat(cli): format phone numbers in show vendors table output
```

---

### Task 9: Extraction preview formatting — tests first

**Files:**
- Test: `internal/app/extraction_render_test.go` (or add to existing)

`previewColumns` and `fmtPhone` are unexported, so the test must be in
`package app`. There are no existing tests for `previewColumns`.

- [ ] **Step 1: Write test for vendor preview phone formatter**

```go
func TestPreviewColumnsVendorFormatsPhone(t *testing.T) {
	// Not parallel: t.Setenv modifies process-global state.
	// LC_ALL has highest precedence in DetectCountry().
	t.Setenv("LC_ALL", "en_US.UTF-8")
	cur, err := locale.ResolveDefault("")
	require.NoError(t, err)
	cols := previewColumns(data.TableVendors, cur)

	// Find the phone column by key.
	var phoneFmt func(any) string
	for _, c := range cols {
		if c.dataKey == data.ColPhone {
			phoneFmt = c.format
			break
		}
	}
	require.NotNil(t, phoneFmt, "phone column not found in vendor preview")

	// Should format parseable numbers, passthrough garbage.
	assert.Equal(t, "(555) 123-4567", phoneFmt("5551234567"))
	assert.Equal(t, "not a phone", phoneFmt("not a phone"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/app/ -run TestPreviewColumnsVendorFormatsPhone -v
```

Expected: FAIL — phone column still uses `fmtAnyText` which returns
`"5551234567"` unchanged.

---

### Task 10: Extraction preview formatting — implementation

**Files:**
- Modify: `internal/app/extraction_render.go:710`

- [ ] **Step 1: Add fmtPhone formatter and use it**

In `internal/app/extraction_render.go`, near the other `fmt*` functions, add:

```go
func fmtPhone(v any) string {
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return locale.FormatPhoneNumber(s, strings.ToUpper(config.DetectCountry()))
}
```

Then change the vendor phone column def (line 710):

```go
{data.ColPhone, s[4], fmtPhone},
```

Add `"github.com/micasa-dev/micasa/internal/config"` to the imports.
(`strings` and `locale` are already imported.)

- [ ] **Step 2: Run tests**

```bash
go test ./internal/app/ -run "TestExtraction" -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```
feat(ui): format phone numbers in extraction preview
```

---

### Task 11: Inline edit test

**Files:**
- Test: `internal/app/vendor_test.go` (or
  `internal/app/inline_edit_dispatch_test.go`)

- [ ] **Step 1: Write test verifying inline edit uses raw value**

```go
func TestVendorInlineEditPhoneUsesRawValue(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create vendor with raw phone.
	require.NoError(t, m.store.CreateVendor(&data.Vendor{
		Name:  "Raw Phone Co",
		Phone: "5559876543",
	}))

	// Switch to vendor tab and reload.
	for i, tab := range m.tabs {
		if tab.Kind == tabVendors {
			m.active = i
			break
		}
	}
	require.NoError(t, m.reloadActiveTab())

	tab := m.activeTab()
	require.NotEmpty(t, tab.Rows)
	id := tab.Rows[0].ID

	// Open inline edit for phone column.
	require.NoError(t, m.inlineEditVendor(id, vendorColPhone))
	require.NotNil(t, m.inlineInput)

	// The inline input should contain the raw DB value, not formatted.
	assert.Equal(t, "5559876543", m.inlineInput.Input.Value())
}
```

- [ ] **Step 2: Run test**

```bash
go test ./internal/app/ -run TestVendorInlineEditPhoneUsesRawValue -v
```

Expected: PASS (inline editing already loads raw value from DB via
`vendorFormValues`; this test confirms the behavior is preserved).

- [ ] **Step 3: Commit**

```
test: verify inline phone edit uses raw DB value
```

---

### Task 12: Final verification and vendor hash update

- [ ] **Step 1: Run full test suite**

```bash
go test -shuffle=on ./...
```

Expected: all PASS.

- [ ] **Step 2: Run linter**

```bash
golangci-lint run
```

Expected: no warnings.

- [ ] **Step 3: Run pre-commit**

Use `/pre-commit-check` to verify everything passes.

- [ ] **Step 4: Update vendor hash if needed**

Use `/update-vendor-hash` if `go.sum` changed.

- [ ] **Step 5: Squash-ready check**

Review the commit log. All commits should be logical, atomic changes. No
fixup commits left behind.
