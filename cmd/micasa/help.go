// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Wong palette colors (duplicated from internal/app/styles.go so the CLI
// binary does not import the full TUI package).
var (
	helpAccent    = lipgloss.AdaptiveColor{Light: "#0072B2", Dark: "#56B4E9"}
	helpSecondary = lipgloss.AdaptiveColor{Light: "#D55E00", Dark: "#E69F00"}
	helpDim       = lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#6B7280"}
	helpMid       = lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#9CA3AF"}
)

var (
	helpHeading = lipgloss.NewStyle().
			Foreground(helpAccent).
			Bold(true)
	helpCmd = lipgloss.NewStyle().
		Foreground(helpSecondary)
	helpFlag = lipgloss.NewStyle().
			Foreground(helpSecondary)
	helpDesc = lipgloss.NewStyle().
			Foreground(helpMid)
	helpDimStyle = lipgloss.NewStyle().
			Foreground(helpDim)
)

func styledHelp(cmd *cobra.Command, _ []string) {
	var b strings.Builder

	if cmd.Long != "" {
		fmt.Fprintln(&b, helpDesc.Render(cmd.Long))
		fmt.Fprintln(&b)
	} else if cmd.Short != "" {
		fmt.Fprintln(&b, helpDesc.Render(cmd.Short))
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

	fmt.Fprint(cmd.OutOrStdout(), b.String())
}
