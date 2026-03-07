// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"reflect"

	"github.com/charmbracelet/huh"
)

// dimPath renders a shortened path in the dim text color so it visually
// recedes next to the bold title label.
var dimPath = appStyles.DimPath()

// filePickerDesc returns the description line for a filepicker field,
// reflecting the current ShowHidden state.
func filePickerDesc(showHidden bool) string {
	label := "hidden"
	if showHidden {
		label = "\x1b[9m" + label + "\x1b[29m"
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
	fp.Description(filePickerDesc(filePickerShowHidden(fp)))
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
