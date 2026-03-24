<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# File picker full filesystem navigation

**Issue:** [#485](https://github.com/micasa-dev/micasa/issues/485)

## Problem

The file picker (huh `FilePicker` wrapping bubbles `filepicker`) supports
full up/down navigation via `h`/`backspace`/`left` keys, but this is
completely undiscoverable. Users see no hint that these keys exist, so the
picker appears to only navigate downward.

Additionally, permissions are shown but add no value for a file-upload
picker, and directories are visually indistinct from files.

## Root cause

Back navigation was broken because bubbles `filepicker.New()` defaults to
`CurrentDirectory: "."`. The Back handler computes the parent via
`filepath.Dir(m.CurrentDirectory)`, but `filepath.Dir(".") == "."` -- so
pressing Back resets the cursor to 0 without ever changing the directory.
The user sees the cursor jump to the top (identical to `g`/GoToTop).

The fix: resolve `"."` to an absolute path via `os.Getwd()` and pass it
to `.CurrentDirectory()`. With an absolute path like `/home/user`,
`filepath.Dir` correctly returns `/home`.

## Approach

Use the existing huh `FilePicker` API -- no custom component needed:

1. **Absolute start dir via `.CurrentDirectory()`** -- resolve CWD to an
   absolute path so `filepath.Dir` can compute real parents.

2. **Nav hint via `.Description()`** -- static description showing
   `h/← back · enter open` so users know how to navigate up.

3. **Kill permissions** -- `.ShowPermissions(false)` removes the
   permission column.

4. **Directory styling** -- set `theme.Focused.Directory` to sky blue
   bold (`accent`) and `theme.Focused.File` to bright text so
   directories stand out visually.

5. **Triangle cursor** -- `.Cursor("▸")` and bold `SelectedOption` for
   the focused row.

## Files changed

- `internal/app/forms.go` -- `newDocumentFilePicker`: added
  CurrentDirectory (absolute), Description, Cursor, ShowPermissions(false);
  `formTheme`: added Directory and File styles, bold SelectedOption

## Non-goals

- Custom filepicker component (the built-in one works fine)
- Path input bar or `~`/`/` keybindings (future enhancement)
