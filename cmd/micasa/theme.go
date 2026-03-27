// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"image/color"

	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
)

// Wong palette hex values used for CLI theming. These duplicate the values in
// internal/app/styles.go intentionally so the CLI binary does not import the
// full TUI package.
//
// Light/dark adaptive pairs: the first value is for light backgrounds, the
// second for dark backgrounds.

func wongColorScheme(c lipgloss.LightDarkFunc) fang.ColorScheme {
	blue := c(lipgloss.Color("#0072B2"), lipgloss.Color("#56B4E9"))
	orange := c(lipgloss.Color("#D55E00"), lipgloss.Color("#E69F00"))
	green := c(lipgloss.Color("#009E73"), lipgloss.Color("#009E73"))
	purple := c(lipgloss.Color("#CC79A7"), lipgloss.Color("#CC79A7"))
	vermillion := c(lipgloss.Color("#D55E00"), lipgloss.Color("#D55E00"))
	base := c(lipgloss.Color("#4B5563"), lipgloss.Color("#9CA3AF"))
	dim := c(lipgloss.Color("#4B5563"), lipgloss.Color("#6B7280"))
	cream := lipgloss.Color("#FFFAF1")
	codeblockBg := c(lipgloss.Color("#F1EFEF"), lipgloss.Color("#2F2E36"))

	return fang.ColorScheme{
		Base:           base,
		Title:          blue,
		Description:    base,
		Codeblock:      codeblockBg,
		Program:        blue,
		DimmedArgument: dim,
		Comment:        dim,
		Flag:           green,
		FlagDefault:    dim,
		Command:        orange,
		QuotedString:   purple,
		Argument:       base,
		ErrorHeader:    [2]color.Color{cream, vermillion},
	}
}
