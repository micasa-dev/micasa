<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# House Profile Overlay Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the inline expanded house header with a three-column overlay, nickname pill in collapsed header, and inline field editing.

**Architecture:** Shared field definitions (`houseFieldDef` slice) drive both the initial full-screen form and the overlay's inline editor. The overlay implements the existing `overlay` interface (`model.go:1342`) and slots into `overlays()` (`model.go:1432`). Collapsed header is a restyled version of the current `houseCollapsed()`.

**Tech Stack:** Go, Bubble Tea, lipgloss, huh, bubblezone

**Spec:** `plans/house-profile-redesign.md`

---

### Task 1: Shared Field Definitions

Extract field metadata into a reusable slice that describes every `HouseProfile` field: its key, display label, section, how to build a `huh.Field`, and how to read/write values.

**Files:**
- Create: `internal/app/house_fields.go`
- Test: `internal/app/house_fields_test.go`

- [ ] **Step 1: Write test that all HouseProfile fields are covered**

```go
func TestHouseFieldDefsComplete(t *testing.T) {
	t.Parallel()
	// Every editable field on houseFormData must have a def.
	defs := houseFieldDefs()
	keys := make(map[string]bool, len(defs))
	for _, d := range defs {
		require.False(t, keys[d.key], "duplicate key: %s", d.key)
		keys[d.key] = true
	}
	// Derive expected keys from houseFormData struct fields.
	rt := reflect.TypeOf(houseFormData{})
	for i := range rt.NumField() {
		f := rt.Field(i)
		if f.Name == "m" { // skip Model backref
			continue
		}
		key := toSnakeCase(f.Name) // e.g. YearBuilt → year_built
		assert.True(t, keys[key], "houseFormData.%s (key %q) has no field def", f.Name, key)
	}
}

func TestHouseFieldDefSections(t *testing.T) {
	t.Parallel()
	defs := houseFieldDefs()
	for _, d := range defs {
		assert.NotZero(t, d.label, "field %s has empty label", d.key)
		assert.True(t, d.section >= houseSectionIdentity && d.section <= houseSectionFinancial,
			"field %s has invalid section", d.key)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -shuffle=on -run TestHouseFieldDefs ./internal/app/`
Expected: FAIL — `houseFieldDefs` undefined

- [ ] **Step 3: Implement houseFieldDef type and slice**

Create `internal/app/house_fields.go` with:

```go
type houseSection int

const (
	houseSectionIdentity  houseSection = iota
	houseSectionStructure
	houseSectionUtilities
	houseSectionFinancial
)

type houseFieldDef struct {
	key     string
	label   string
	section houseSection
	// build creates a huh.Field bound to the given *string value.
	// The Model receiver is needed for unit-system-aware titles.
	build func(m *Model, value *string) huh.Field
	// get reads the display value from a HouseProfile.
	get func(p data.HouseProfile, cur locale.Currency, us data.UnitSystem) string
	// set writes a string pointer in houseFormData.
	set func(fd *houseFormData, value string)
}

func houseFieldDefs() []houseFieldDef { ... }
```

The `houseFieldDefs()` function returns the ordered slice. Each entry mirrors the fields in `houseFormData` (`forms.go:35-63`) and `HouseProfile` (`models.go:104-135`). Sections:
- **Identity**: nickname, address_line1, address_line2, city, state, postal_code
- **Structure**: year_built, square_feet, lot_square_feet, bedrooms, bathrooms, foundation_type, wiring_type, roof_type, exterior_type, basement_type
- **Utilities**: heating_type, cooling_type, water_source, sewer_type, parking_type
- **Financial**: insurance_carrier, insurance_policy, insurance_renewal, property_tax, hoa_name, hoa_fee

Each `build` func replicates the corresponding `huh.NewInput()` call from `startHouseForm()` (`forms.go:194-287`), including validators (`requiredText`, `optionalInt`, `optionalFloat`, `optionalDate`, `optionalMoney`).

Each `get` func reads from `data.HouseProfile` and formats for display (int → `strconv.Itoa`, `*int64` cents → `cur.FormatOptionalCents`, `*time.Time` → `data.FormatDate`).

Each `set` func writes to the corresponding `houseFormData` string field.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -shuffle=on -run TestHouseFieldDefs ./internal/app/`
Expected: PASS

- [ ] **Step 5: Write test for get/set round-trip**

```go
func TestHouseFieldDefGetSet(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	defs := houseFieldDefs()
	fd := m.houseFormValues(m.house)
	for _, d := range defs {
		val := d.get(m.house, m.cur, m.unitSystem)
		// For non-empty fields, get should return non-empty string.
		// We just verify no panics and round-trip consistency.
		d.set(fd, val)
	}
}
```

- [ ] **Step 6: Run test, verify pass**

Run: `go test -shuffle=on -run TestHouseFieldDefGetSet ./internal/app/`
Expected: PASS

- [ ] **Step 7: Commit**

```
feat(ui): add shared house field definitions

Introduce houseFieldDef type and ordered slice covering all
HouseProfile fields. Each def captures key, label, section,
huh.Field builder, getter, and setter — foundation for both
the full form and the overlay inline editor.
```

---

### Task 2: Refactor Initial House Form to Use Shared Defs

Replace the hand-coded field construction in `startHouseForm()` with iteration over `houseFieldDefs()`. Must preserve existing behavior exactly.

**Files:**
- Modify: `internal/app/forms.go` — `startHouseForm()` (line 194-287)

- [ ] **Step 1: Run existing tests to establish baseline**

Run: `go test -shuffle=on ./internal/app/`
Expected: PASS (all existing tests)

- [ ] **Step 2: Refactor startHouseForm to iterate defs**

Rewrite `startHouseForm()` to:
1. Call `houseFieldDefs()` to get the ordered list.
2. Group defs by section.
3. For each section, build a `huh.Group` by calling `d.build(m, &values.Field)` for each def in that section.
4. Assemble groups into `huh.NewForm(...)`.
5. Preserve the postal code autofill references (postalCodeInput, cityInput, stateInput) — these are the only special-case fields that need to be captured during iteration.
6. Preserve the "only nickname required" description on basics group when `!m.hasHouse`.

The `set` function on each def maps key → `houseFormData` field pointer, so the iteration can bind `build` to the correct `*string`.

- [ ] **Step 3: Run full test suite to verify no regressions**

Run: `go test -shuffle=on ./internal/app/`
Expected: PASS — identical behavior

- [ ] **Step 4: Commit**

```
refactor(ui): drive house form from shared field definitions

startHouseForm now iterates houseFieldDefs() instead of hand-coding
each huh.Field. No behavior change — same fields, validators, and
groups. Prepares for inline overlay editing.
```

---

### Task 3: Collapsed Header Redesign

Replace `House` pill with nickname, add bright/dim value styling, k-suffix sqft formatting, and empty field indicator.

**Files:**
- Modify: `internal/app/house.go` — `houseCollapsed()` (line 44), `houseTitle()` (line 37)
- Create: `internal/app/house_format.go` — `formatKSqft()` helper
- Test: `internal/app/house_test.go`

- [ ] **Step 1: Write test for k-suffix formatting**

```go
func TestFormatKSqft(t *testing.T) {
	t.Parallel()
	tests := []struct {
		sqft int
		want string
	}{
		{0, ""},
		{850, "850"},
		{1200, "1.2k"},
		{1950, "2k"},
		{2400, "2.4k"},
		{5000, "5k"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatKSqft(tt.sqft), "sqft=%d", tt.sqft)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -shuffle=on -run TestFormatKSqft ./internal/app/`
Expected: FAIL — `formatKSqft` undefined

- [ ] **Step 3: Implement formatKSqft**

In `internal/app/house_format.go`:

```go
func formatKSqft(sqft int) string {
	if sqft == 0 {
		return ""
	}
	if sqft < 1000 {
		return strconv.Itoa(sqft)
	}
	rounded := math.Round(float64(sqft)/100) / 10
	if rounded == math.Trunc(rounded) {
		return fmt.Sprintf("%.0fk", rounded)
	}
	return fmt.Sprintf("%.1fk", rounded)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -shuffle=on -run TestFormatKSqft ./internal/app/`
Expected: PASS

- [ ] **Step 5: Write test for empty field count**

```go
func TestHouseEmptyFieldCount(t *testing.T) {
	t.Parallel()
	defs := houseFieldDefs()
	// Profile with all fields empty except nickname.
	p := data.HouseProfile{Nickname: "Test"}
	count := houseEmptyFieldCount(p, locale.USD, data.UnitSystemImperial)
	// Should be total defs minus 1 (nickname is set).
	assert.Equal(t, len(defs)-1, count)
}
```

- [ ] **Step 6: Implement houseEmptyFieldCount**

```go
func houseEmptyFieldCount(p data.HouseProfile, cur locale.Currency, us data.UnitSystem) int {
	count := 0
	for _, d := range houseFieldDefs() {
		if strings.TrimSpace(d.get(p, cur, us)) == "" {
			count++
		}
	}
	return count
}
```

- [ ] **Step 7: Run test, verify pass**

- [ ] **Step 8: Write test for collapsed header rendering**

```go
func TestHouseCollapsedNicknamePill(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.showHouse = false
	view := m.houseCollapsed()
	assert.Contains(t, view, m.house.Nickname, "should show nickname")
	assert.NotContains(t, view, "House", "should not show House label")
}
```

- [ ] **Step 9: Rewrite houseCollapsed()**

Replace current implementation to:
1. Render nickname in `HeaderTitle()` pill style (replaces `houseTitle()` which rendered "House").
2. Append `▸` badge.
3. Dot-separated vitals: city/state (dim), bed/bath (bright values, dim suffixes), k-formatted sqft (bright value, dim "sf"), year (bright).
4. If empty fields > 0, append `· ○ N` in warning style.

- [ ] **Step 10: Update houseTitle() for overlay context**

When overlay is active, `houseTitle()` should still use `AccentOutline` style but render the nickname instead of "House". Rename to `housePill()` for clarity.

- [ ] **Step 11: Run full test suite**

Run: `go test -shuffle=on ./internal/app/`
Expected: PASS — TestHouseToggle and other existing tests may need assertion updates for "House" → nickname text.

- [ ] **Step 12: Commit**

```
feat(ui): redesign collapsed house header with nickname pill

Replace House pill with nickname, add bright/dim value hierarchy,
k-suffix sqft formatting (2,400 → 2.4k), and ○ N empty field
indicator in warning color.
```

---

### Task 4: House Overlay State and Rendering

Add overlay state to Model, build the three-column grid renderer, wire into overlay stack.

**Files:**
- Create: `internal/app/house_overlay.go` — overlay type, state, rendering, key dispatch
- Modify: `internal/app/model.go` — add `houseOverlay *houseOverlayState` field, register in `overlays()`
- Modify: `internal/app/view.go` — add to overlay render stack in `buildView()`
- Modify: `internal/app/house.go` — `houseView()` toggle now opens overlay instead of expanding header
- Test: `internal/app/house_overlay_test.go`

- [ ] **Step 1: Write test for overlay visibility toggle**

```go
func TestHouseOverlayToggle(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	assert.Nil(t, m.houseOverlay, "overlay should start nil")

	sendKey(m, keyTab)
	assert.NotNil(t, m.houseOverlay, "tab should open overlay")

	sendKey(m, keyEsc)
	assert.Nil(t, m.houseOverlay, "esc should close overlay")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -shuffle=on -run TestHouseOverlayToggle ./internal/app/`
Expected: FAIL — `houseOverlay` field doesn't exist

- [ ] **Step 3: Define houseOverlayState and overlay adapter**

In `internal/app/house_overlay.go`:

```go
type houseOverlayState struct {
	section  int // 0=identity, 1=structure, 2=utilities, 3=financial
	row      int // cursor row within current section
	editing  bool
	form     *huh.Form
	formData *houseFormData // temporary form data during inline edit
}
```

Implement the `overlay` interface (`model.go:1342`):

```go
type houseProfileOverlay struct{ m *Model }

func (o houseProfileOverlay) isVisible() bool   { return o.m.houseOverlay != nil }
func (o houseProfileOverlay) hidesMainKeys() bool { return true }
func (o houseProfileOverlay) handleKey(msg tea.KeyPressMsg) tea.Cmd { ... }
```

- [ ] **Step 4: Add Model field and register overlay**

In `model.go`, add `houseOverlay *houseOverlayState` to Model struct (near `showHouse` at line 203).

In `overlays()` (`model.go:1432`), add `houseProfileOverlay{m}` above the existing entries (so it has higher priority than help/chat/etc but note that dashboard has its own dispatch path).

In `buildView()` (`view.go:30-44`), add the house overlay entry:
```go
{m.houseOverlay != nil, m.buildHouseOverlay},
```
Position it after dashboard but before calendar.

- [ ] **Step 5: Wire toggle**

In `handleCommonKeys` (`model_keys.go:75-77`), replace `m.showHouse = !m.showHouse` with:
```go
if m.houseOverlay != nil {
    m.houseOverlay = nil
} else if m.hasHouse {
    m.houseOverlay = &houseOverlayState{section: 1, row: 0}
}
m.resizeTables()
```

Update `houseView()` in `house.go`: remove the `m.showHouse` expanded branch. The expanded view now lives in the overlay. `houseView()` always returns the collapsed header (or setup prompt).

Remove `showHouse` field from Model.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -shuffle=on -run TestHouseOverlayToggle ./internal/app/`
Expected: PASS

- [ ] **Step 7: Write test for overlay rendering content**

```go
func TestHouseOverlayRenders(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab) // open overlay
	view := m.buildView()
	assert.Contains(t, view, "Structure")
	assert.Contains(t, view, "Utilities")
	assert.Contains(t, view, "Financial")
	assert.Contains(t, view, m.house.Nickname)
}
```

- [ ] **Step 8: Implement buildHouseOverlay()**

In `internal/app/house_overlay.go`, implement `buildHouseOverlay()`:

1. **Identity line**: nickname pill + full address (OSC8 maps link) + completion fraction right-aligned.
2. **Three columns**: iterate `houseFieldDefs()`, group by section, render each section as a column with header + horizontal rule + label/value rows.
3. **Cursor**: highlight the focused field (section + row) with a `▸` prefix and background highlight style.
4. **Empty fields**: render `○ —` or `○ not set` in warning color.
5. **Hint bar**: `↑↓ navigate  ←→ section  enter edit  esc close`
6. **Compose**: join columns horizontally with gap, wrap in `OverlayBox()` style, size to content.

The three columns have different lengths. Pad shorter columns with empty lines to align. Use `lipgloss.JoinHorizontal(lipgloss.Top, ...)` for column layout.

- [ ] **Step 9: Run test, verify pass**

Run: `go test -shuffle=on -run TestHouseOverlayRenders ./internal/app/`
Expected: PASS

- [ ] **Step 10: Run full test suite, fix any regressions**

Run: `go test -shuffle=on ./internal/app/`

Existing tests that reference `m.showHouse` need updating to use `m.houseOverlay` instead. Key tests to update:
- `TestHouseToggle` (`mode_test.go:296`) — assert `m.houseOverlay != nil` instead of `m.showHouse`
- `TestHouseHeaderClickToggles` (`mouse_test.go:161`) — same
- `view_test.go:45` — same
- Any test that sets `m.showHouse = true` — set `m.houseOverlay = &houseOverlayState{section: 1}`

- [ ] **Step 11: Commit**

```
feat(ui): add house profile overlay with three-column grid

Replace inline expanded header with centered overlay. Implements
overlay interface, renders Structure/Utilities/Financial columns
with cursor highlighting and empty field indicators.

closes #842
```

---

### Task 5: Keyboard Navigation in Overlay

Implement column-major cursor movement with identity section above the grid.

**Files:**
- Modify: `internal/app/house_overlay.go` — `handleKey()` method
- Test: `internal/app/house_overlay_test.go`

- [ ] **Step 1: Write test for column-major navigation**

```go
func TestHouseOverlayNavigation(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab) // open overlay

	// Starts at first structure field (section=1, row=0).
	require.NotNil(t, m.houseOverlay)
	assert.Equal(t, 1, m.houseOverlay.section)
	assert.Equal(t, 0, m.houseOverlay.row)

	// Down moves within column.
	sendKey(m, keyDown)
	assert.Equal(t, 1, m.houseOverlay.section)
	assert.Equal(t, 1, m.houseOverlay.row)

	// Right jumps to utilities column.
	sendKey(m, keyRight)
	assert.Equal(t, 2, m.houseOverlay.section)

	// Right again to financial.
	sendKey(m, keyRight)
	assert.Equal(t, 3, m.houseOverlay.section)

	// Right at rightmost column clamps.
	sendKey(m, keyRight)
	assert.Equal(t, 3, m.houseOverlay.section)

	// Up from row 0 in grid moves to identity section.
	m.houseOverlay.section = 1
	m.houseOverlay.row = 0
	sendKey(m, keyUp)
	assert.Equal(t, 0, m.houseOverlay.section, "should enter identity section")
}
```

- [ ] **Step 2: Run test to verify it fails**

Expected: FAIL — `handleKey` not yet dispatching navigation

- [ ] **Step 3: Implement handleKey navigation**

In `houseProfileOverlay.handleKey()`:

```go
func (o houseProfileOverlay) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	s := o.m.houseOverlay
	if s.editing {
		return o.m.handleHouseEditKey(msg)
	}
	switch {
	case key.Matches(msg, o.m.keys.CursorDown):
		o.m.houseOverlayDown()
	case key.Matches(msg, o.m.keys.CursorUp):
		o.m.houseOverlayUp()
	case key.Matches(msg, o.m.keys.ColRight):
		o.m.houseOverlayRight()
	case key.Matches(msg, o.m.keys.ColLeft):
		o.m.houseOverlayLeft()
	case key.Matches(msg, o.m.keys.Confirm):
		o.m.houseOverlayStartEdit()
	case key.Matches(msg, o.m.keys.HelpClose):
		o.m.houseOverlay = nil
	case key.Matches(msg, o.m.keys.HouseToggle):
		o.m.houseOverlay = nil
		o.m.resizeTables()
	}
	return nil
}
```

Navigation methods:
- `houseOverlayDown()`: if identity section, move to structure col 0 row 0. If grid, increment row. Clamp to section length.
- `houseOverlayUp()`: decrement row. If row < 0 and in grid, move to identity section.
- `houseOverlayRight()`: if identity, cycle identity fields. If grid, move to next section (1→2→3), clamp row to target section length.
- `houseOverlayLeft()`: reverse of right.

Compute section lengths from `houseFieldDefs()` filtered by section.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -shuffle=on -run TestHouseOverlayNavigation ./internal/app/`
Expected: PASS

- [ ] **Step 5: Write test for row clamping on shorter columns**

```go
func TestHouseOverlayRowClamping(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)

	// Move deep into structure column (which is the longest).
	defs := houseFieldDefs()
	structLen := 0
	for _, d := range defs {
		if d.section == houseSectionStructure {
			structLen++
		}
	}
	for i := 0; i < structLen-1; i++ {
		sendKey(m, keyDown)
	}
	assert.Equal(t, structLen-1, m.houseOverlay.row)

	// Jump to utilities (shorter) — row should clamp.
	sendKey(m, keyRight)
	utilLen := 0
	for _, d := range defs {
		if d.section == houseSectionUtilities {
			utilLen++
		}
	}
	assert.LessOrEqual(t, m.houseOverlay.row, utilLen-1)
}
```

- [ ] **Step 6: Run test, verify pass**

- [ ] **Step 7: Commit**

```
feat(ui): add column-major keyboard navigation to house overlay

Arrow keys navigate within and between sections. Row position
is remembered when jumping columns, clamped to shorter sections.
Up from top of grid enters identity section.
```

---

### Task 6: Mouse Interaction in Overlay

Zone-mark every field, handle click-to-select and double-click-to-edit.

**Files:**
- Modify: `internal/app/house_overlay.go` — add zone marks to field rendering
- Modify: `internal/app/mouse.go` — add zone constants, handle clicks in overlay
- Test: `internal/app/house_overlay_test.go`

- [ ] **Step 1: Write test for zone existence**

```go
func TestHouseOverlayFieldZones(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	_ = m.buildView() // trigger zone registration
	requireZone(t, m, "house-field-nickname")
	requireZone(t, m, "house-field-year_built")
	requireZone(t, m, "house-field-heating_type")
	requireZone(t, m, "house-field-insurance_carrier")
}
```

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Add zone marks to field rendering**

In `buildHouseOverlay()`, wrap each field's rendered line with `m.zones.Mark(zoneHouseField+d.key, line)`.

Add zone prefix constant to `mouse.go`:
```go
zoneHouseField = "house-field-"
```

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Write test for click-to-select**

```go
func TestHouseOverlayClickSelectsField(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab)
	_ = m.buildView()

	// Click a field in utilities section.
	zone := m.zones.Get("house-field-heating_type")
	require.True(t, zone.Visible(), "heating zone should be visible")
	sendClick(m, zone.X()+1, zone.Y())

	// Cursor should have moved to that field.
	assert.Equal(t, 2, m.houseOverlay.section, "should be in utilities")
}
```

- [ ] **Step 6: Implement mouse handling**

In `mouse.go`, add handling for `zoneHouseField` clicks inside the house overlay dispatch path. On click:
1. Parse the zone ID suffix to get the field key.
2. Find the field's section and row index in `houseFieldDefs()`.
3. Set `m.houseOverlay.section` and `m.houseOverlay.row`.

On double-click, also trigger `houseOverlayStartEdit()`.

- [ ] **Step 7: Run tests, verify pass**

- [ ] **Step 8: Commit**

```
feat(ui): add mouse zones and click handling to house overlay

Every field is zone-marked. Single click selects field (moves
cursor). Double-click opens inline edit.
```

---

### Task 7: Inline Edit in Overlay

Enter on a field opens a single-field `huh.Form` in-place using the shared field def.

**Files:**
- Modify: `internal/app/house_overlay.go` — edit mode rendering and key handling
- Modify: `internal/app/forms.go` — `submitHouseForm()` reuse
- Test: `internal/app/house_overlay_test.go`

- [ ] **Step 1: Write test for inline edit flow**

```go
func TestHouseOverlayInlineEdit(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab) // open overlay

	// Navigate to year built (structure, row 0).
	require.NotNil(t, m.houseOverlay)
	assert.False(t, m.houseOverlay.editing)

	// Enter to edit.
	sendKey(m, keyEnter)
	assert.True(t, m.houseOverlay.editing)
	assert.NotNil(t, m.houseOverlay.form)

	// Esc to cancel.
	sendKey(m, keyEsc)
	assert.False(t, m.houseOverlay.editing)
	assert.Nil(t, m.houseOverlay.form)
}
```

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Implement houseOverlayStartEdit()**

```go
func (m *Model) houseOverlayStartEdit() {
	s := m.houseOverlay
	def := m.houseOverlayCurrentDef()
	if def == nil {
		return
	}
	// Create temporary form data from current profile.
	fd := m.houseFormValues(m.house)
	// Build single-field form.
	field := def.build(m, m.houseFieldPtr(fd, def.key))
	form := huh.NewForm(huh.NewGroup(field))
	form.WithWidth(m.houseOverlayFieldWidth())
	s.form = form
	s.formData = fd
	s.editing = true
}
```

`houseFieldPtr(fd, key)` returns a `*string` pointer to the appropriate `houseFormData` field based on the def's key. This uses the `set` function or a switch on key.

- [ ] **Step 4: Implement handleHouseEditKey()**

When editing:
- Forward key messages to `s.form.Update(msg)`.
- On Enter/submit: extract the edited value, run the existing `submitHouseForm()` parse-and-save logic, refresh `m.house` from store, set `s.editing = false`.
- On Esc: discard, set `s.editing = false`, nil the form.

The submit path reuses `submitHouseForm()` (`forms.go:1875`) by setting `m.fs.formData` temporarily, or by extracting the parse logic into a helper that both paths call.

- [ ] **Step 5: Implement in-place rendering**

In `buildHouseOverlay()`, when `s.editing` is true and the cursor is on the current field, render `s.form.View()` in place of the normal label/value line.

- [ ] **Step 6: Run test, verify pass**

- [ ] **Step 7: Write test that edit persists**

```go
func TestHouseOverlayEditPersists(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	sendKey(m, keyTab) // open overlay

	// Navigate to nickname (identity section, row 0).
	sendKey(m, keyUp) // from structure row 0 → identity
	sendKey(m, keyEnter) // start edit

	require.True(t, m.houseOverlay.editing)
	require.NotNil(t, m.houseOverlay.form)

	// Clear existing value (ctrl+a to select all, then type replaces).
	sendKey(m, "ctrl+a")
	// Type new value — huh form captures keystrokes.
	for _, r := range "Bungalow" {
		sendKey(m, string(r))
	}
	sendKey(m, keyEnter) // submit

	assert.False(t, m.houseOverlay.editing, "should exit edit mode")
	assert.Equal(t, "Bungalow", m.house.Nickname, "nickname should persist")
}
```

- [ ] **Step 8: Run tests, verify pass**

- [ ] **Step 9: Commit**

```
feat(ui): add inline field editing to house overlay

Enter on a focused field opens a single-field huh.Form in-place.
Uses shared field definitions for widget and validation. Esc
cancels, Enter submits and persists to DB.
```

---

### Task 8: Narrow Width and Cleanup

Handle narrow terminals (< 80 cols) and remove pixel art.

**Files:**
- Modify: `internal/app/house_overlay.go` — narrow-width single-column stacking
- Modify: `internal/app/house.go` — remove `houseExpanded()`, `houseArt()`, `houseSection()`
- Modify: `internal/app/styles.go` — remove `HouseRoof`, `HouseWall`, `HouseWindow`, `HouseDoor` styles
- Test: `internal/app/house_overlay_test.go`

- [ ] **Step 1: Write test for narrow-width rendering**

```go
func TestHouseOverlayNarrowWidth(t *testing.T) {
	t.Parallel()
	m := newTestModelWithDemoData(t, 42)
	m.width = 78 // below 80
	sendKey(m, keyTab)
	view := m.buildView()
	// In narrow mode, sections should be stacked vertically.
	// Structure should appear before Utilities in the output.
	structIdx := strings.Index(view, "Structure")
	utilIdx := strings.Index(view, "Utilities")
	finIdx := strings.Index(view, "Financial")
	require.NotEqual(t, -1, structIdx)
	require.NotEqual(t, -1, utilIdx)
	require.NotEqual(t, -1, finIdx)
	assert.Less(t, structIdx, utilIdx)
	assert.Less(t, utilIdx, finIdx)
}
```

- [ ] **Step 2: Run test to verify it fails (or passes if already stacking)**

- [ ] **Step 3: Implement narrow-width logic**

In `buildHouseOverlay()`, check `m.effectiveWidth() < 80`. If narrow, render sections stacked vertically (each section is full-width) instead of side-by-side columns. Reuse the same field rendering — just change the layout join from horizontal to vertical.

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Remove pixel art and dead code**

Delete from `house.go`:
- `houseExpanded()` — replaced by overlay
- `houseArt()` — pixel art removed per spec
- `houseSection()` — only used by `houseExpanded()`

Remove the `showHouse` field from Model if not already done in Task 4.

Delete from `styles.go`:
- `HouseRoof()`, `HouseWall()`, `HouseWindow()`, `HouseDoor()` accessors
- Their backing private fields (if dedicated — check if they alias existing fields like `fgAccent`, `fgTextMid`, `fgSecondary`, `fgWarning`)

Only remove the accessor methods and any dedicated private fields. Do not remove shared base styles that other accessors also use.

- [ ] **Step 6: Run full test suite**

Run: `go test -shuffle=on ./internal/app/`
Expected: PASS

- [ ] **Step 7: Commit**

```
refactor(ui): remove pixel art and add narrow-width stacking

Drop houseExpanded, houseArt, houseSection, and house art styles.
Overlay stacks sections vertically when terminal is under 80 cols.
```

---

### Task 9: Final Integration and Demo

Verify everything works end-to-end, update test coverage, record demo.

**Files:**
- Test: `internal/app/house_overlay_test.go` — any remaining coverage gaps
- Modify: `internal/app/mode_test.go` — update legacy toggle tests
- Modify: `internal/app/mouse_test.go` — update house header click tests

- [ ] **Step 1: Run full test suite with coverage**

Run: `nix run '.#coverage'`
Verify new code is exercised. Add tests for any uncovered paths.

- [ ] **Step 2: Verify no regressions manually**

Run the app: `nix run '.#micasa'`
Check:
- Collapsed header shows nickname pill + vitals
- Tab opens overlay with three columns
- Arrow keys navigate, Enter edits, Esc closes
- Click on fields selects them
- Narrow terminal (resize to < 80 cols) shows stacked layout
- No-house state still shows setup prompt

- [ ] **Step 3: Record demo**

Run: `/record-demo`
Capture before/after comparison showing:
- Old collapsed → new collapsed with nickname pill
- Old expanded → new overlay
- Keyboard navigation through fields
- Inline edit of a field

- [ ] **Step 4: Commit demo**

```
docs(ui): record house profile redesign demo
```
