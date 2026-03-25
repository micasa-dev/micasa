// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"reflect"

	"charm.land/huh/v2"
)

// dimPath renders a shortened path in the dim text color so it visually
// recedes next to the bold title label.
var dimPath = appStyles.DimPath()

// filePickerDesc returns the description line for a filepicker field,
// reflecting the current ShowHidden state.
func filePickerDesc(showHidden bool) string {
	label := "\x1b[9mhidden\x1b[29m"
	if showHidden {
		label = "hidden"
	}
	return keyH + "/" + symLeft + " back " + symMiddleDot + " " +
		keyEnter + " open " + symMiddleDot + " " +
		keyShiftH + " " + label
}

// filePickerShowHidden reads the inner bubbles filepicker's ShowHidden field
// via reflection. Returns false if the field is not a FilePicker.
func filePickerShowHidden(field huh.Field) bool {
	v := reflect.ValueOf(field)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	picker := v.FieldByName("picker")
	if !picker.IsValid() {
		return false
	}
	sh := picker.FieldByName("ShowHidden")
	if !sh.IsValid() {
		return false
	}
	return sh.Bool()
}

// syncPickerDescription updates fp's description to reflect its current
// ShowHidden state.
func syncPickerDescription(fp *huh.FilePicker) {
	fp.Description(filePickerDesc(filePickerShowHidden(fp)))
}

// syncFilePickerDescription updates the focused FilePicker's description to
// reflect the current ShowHidden state. No-op if the focused field is not a
// *huh.FilePicker.
func syncFilePickerDescription(form *huh.Form) {
	field := form.GetFocusedField()
	if field == nil {
		return
	}
	fp, ok := field.(*huh.FilePicker)
	if !ok {
		return
	}
	syncPickerDescription(fp)
}

// filePickerCurrentDir returns the bubbles filepicker's CurrentDirectory from
// a huh.FilePicker field via reflection (the picker field is unexported).
// Returns "" if the field is not a FilePicker.
func filePickerCurrentDir(field huh.Field) string {
	v := reflect.ValueOf(field)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	picker := v.FieldByName("picker")
	if !picker.IsValid() {
		return ""
	}
	dir := picker.FieldByName("CurrentDirectory")
	if !dir.IsValid() {
		return ""
	}
	return dir.String()
}

// filePickerDescription reads the huh FilePicker's description field via
// reflection. Returns "" if the field is not a FilePicker.
func filePickerDescription(field huh.Field) string {
	v := reflect.ValueOf(field)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	d := v.FieldByName("description")
	if !d.IsValid() {
		return ""
	}
	return d.String()
}

// filePickerTitle reads the huh FilePicker's title field via reflection.
// Returns "" if the field is not a FilePicker.
func filePickerTitle(field huh.Field) string {
	v := reflect.ValueOf(field)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.FieldByName("title")
	if !t.IsValid() {
		return ""
	}
	return t.String()
}

// syncPickerTitle updates fp's title to show the current directory (dimmed,
// ~ abbreviated) next to the base label stored in the picker's Key. No-op
// when Key or CurrentDirectory is empty.
func syncPickerTitle(fp *huh.FilePicker) {
	dir := filePickerCurrentDir(fp)
	if dir == "" {
		return
	}
	base := fp.GetKey()
	if base == "" {
		return
	}
	// SGR 22 cancels bold from the outer Title style; lipgloss Bold(false)
	// alone doesn't emit it when nested inside a pre-bolded string.
	fp.Title(base + " \x1b[22m" + dimPath.Render("in "+shortenHome(dir)))
}

// syncFilePickerTitle updates the focused FilePicker's title to show the
// current directory (dimmed, ~ abbreviated) next to the base label. The base
// label is stored in the field's Key. No-op if the focused field is not a
// *huh.FilePicker.
func syncFilePickerTitle(form *huh.Form) {
	field := form.GetFocusedField()
	if field == nil {
		return
	}
	fp, ok := field.(*huh.FilePicker)
	if !ok {
		return
	}
	syncPickerTitle(fp)
}
