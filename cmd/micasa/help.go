// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Wong palette colors (duplicated from internal/app/styles.go so the CLI
// binary does not import the full TUI package).
type helpColor struct{ Light, Dark string }

func (c helpColor) resolve(isDark bool) color.Color {
	if isDark {
		return lipgloss.Color(c.Dark)
	}
	return lipgloss.Color(c.Light)
}

var (
	helpAccentPair    = helpColor{Light: "#0072B2", Dark: "#56B4E9"}
	helpSecondaryPair = helpColor{Light: "#D55E00", Dark: "#E69F00"}
	helpDimPair       = helpColor{Light: "#4B5563", Dark: "#6B7280"}
	helpMidPair       = helpColor{Light: "#4B5563", Dark: "#9CA3AF"}
)

func helpStyles(isDark bool) (heading, cmd, flag, desc, dim lipgloss.Style) {
	heading = lipgloss.NewStyle().
		Foreground(helpAccentPair.resolve(isDark)).
		Bold(true)
	cmd = lipgloss.NewStyle().
		Foreground(helpSecondaryPair.resolve(isDark))
	flag = lipgloss.NewStyle().
		Foreground(helpSecondaryPair.resolve(isDark))
	desc = lipgloss.NewStyle().
		Foreground(helpMidPair.resolve(isDark))
	dim = lipgloss.NewStyle().
		Foreground(helpDimPair.resolve(isDark))
	return
}

func styledHelp(cmd *cobra.Command, _ []string) {
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stderr)
	helpHeading, helpCmd, helpFlag, helpDesc, helpDimStyle := helpStyles(isDark)

	var b strings.Builder

	if cmd.Long != "" {
		fmt.Fprintln(&b, helpDesc.Render(cmd.Long))
		fmt.Fprintln(&b)
	} else if cmd.Short != "" {
		fmt.Fprintln(&b, helpDesc.Render(cmd.Short))
		fmt.Fprintln(&b)
	}

	if cmd.HasExample() {
		fmt.Fprintln(&b, helpHeading.Render("Examples"))
		for _, line := range strings.Split(cmd.Example, "\n") {
			fmt.Fprintln(&b, helpDimStyle.Render(line))
		}
		fmt.Fprintln(&b)
	}

	if cmd.Runnable() {
		fmt.Fprintln(&b, helpHeading.Render("Usage"))
		fmt.Fprintf(&b, "  %s\n\n", helpDimStyle.Render(cmd.UseLine()))
	}

	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(&b, helpHeading.Render("Commands"))
		maxLen := 0
		for _, c := range cmd.Commands() {
			if !c.IsAvailableCommand() && c.Name() != "help" {
				continue
			}
			if n := len(c.Name()); n > maxLen {
				maxLen = n
			}
		}
		for _, c := range cmd.Commands() {
			if !c.IsAvailableCommand() && c.Name() != "help" {
				continue
			}
			name := helpCmd.Render(c.Name())
			pad := strings.Repeat(" ", maxLen-len(c.Name()))
			fmt.Fprintf(&b, "  %s%s  %s\n", name, pad, helpDesc.Render(c.Short))
		}
		fmt.Fprintln(&b)
	}

	if cmd.HasAvailableLocalFlags() {
		fmt.Fprintln(&b, helpHeading.Render("Flags"))
		cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			var names string
			if f.Shorthand != "" {
				names = fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name)
			} else {
				names = fmt.Sprintf("    --%s", f.Name)
			}
			fmt.Fprintf(&b, "  %s  %s\n", helpFlag.Render(names), helpDesc.Render(f.Usage))
		})
		fmt.Fprintln(&b)
	}

	if cmd.HasAvailableInheritedFlags() {
		fmt.Fprintln(&b, helpHeading.Render("Global Flags"))
		cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			var names string
			if f.Shorthand != "" {
				names = fmt.Sprintf("-%s, --%s", f.Shorthand, f.Name)
			} else {
				names = fmt.Sprintf("    --%s", f.Name)
			}
			fmt.Fprintf(&b, "  %s  %s\n", helpFlag.Render(names), helpDesc.Render(f.Usage))
		})
		fmt.Fprintln(&b)
	}

	_, _ = fmt.Fprint(cmd.OutOrStdout(), b.String())
}
