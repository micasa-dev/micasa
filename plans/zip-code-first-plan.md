<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Postal Code First Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reorder the house form to put postal code first, and auto-fill city/state via zippopotam.us when the user tabs away.

**Architecture:** New `internal/address/` package handles the HTTP lookup. The `updateForm` method in `model.go` detects when focus leaves the postal code field and dispatches an async `tea.Cmd`. Config gets a new `[address]` section with an `autofill` toggle. Country is auto-detected from locale.

**Tech Stack:** Go, Bubble Tea, huh forms, net/http, zippopotam.us REST API

**Spec:** `docs/superpowers/specs/2026-03-24-zip-code-first-design.md`

---

### Task 1: Address Lookup Package

**Files:**
- Create: `internal/address/lookup.go`
- Create: `internal/address/lookup_test.go`

- [x] **Step 1: Write the failing tests**
- [x] **Step 2: Run tests to verify they fail**
- [x] **Step 3: Write the implementation**
- [x] **Step 4: Run tests to verify they pass**
- [x] **Step 5: Commit**

---

### Task 2: Config -- Address Section

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/country.go`
- Create: `internal/config/country_test.go`

- [x] **Step 1: Write the failing test for country detection**
- [x] **Step 2: Run test to verify it fails**
- [x] **Step 3: Implement country detection**
- [x] **Step 4: Run test to verify it passes**
- [x] **Step 5: Add Address config struct**
- [x] **Step 6: Run full config tests**
- [x] **Step 7: Commit**

---

### Task 3: Reorder House Form Fields

**Files:**
- Modify: `internal/app/forms.go`
- Create: `internal/app/form_postal_code_test.go`

- [x] **Step 1: Write the failing test for field order**
- [x] **Step 2: Run test to verify current behavior**
- [x] **Step 3: Reorder the struct and form fields**
- [x] **Step 4: Run tests**
- [x] **Step 5: Commit**

---

### Task 4: Wire Postal Code Auto-fill Into the TUI

**Files:**
- Modify: `internal/app/types.go` (formState, Options)
- Modify: `internal/app/model.go` (Model fields, NewModel, updateForm, Update)
- Modify: `internal/app/forms.go` (startHouseForm)
- Modify: `cmd/micasa/main.go` (Options wiring)
- Create: `internal/app/postal_code.go` (message type, command)

- [x] **Step 1: Write the failing user-flow tests**
- [x] **Step 2: Create postal_code.go**
- [x] **Step 3: Add Model/Options fields**
- [x] **Step 4: Blur detection in updateForm**
- [x] **Step 5: Handle postalCodeLookupMsg in Update**
- [x] **Step 6: Wire main.go**
- [x] **Step 7: Run all tests**
- [x] **Step 8: Commit**

---

### Task 5: Final Checks

- [ ] **Step 1: Fix linter warnings**
- [ ] **Step 2: Run pre-commit checks**
- [ ] **Step 3: Final commit if needed**
