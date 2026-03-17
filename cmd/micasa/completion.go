// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"github.com/spf13/cobra"
)

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "completion [bash|zsh|fish]",
		Short:         "Generate shell completion scripts",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:           "bash",
			Short:         "Generate bash completion script",
			SilenceErrors: true,
			SilenceUsage:  true,
			Args:          cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
			},
		},
		&cobra.Command{
			Use:           "zsh",
			Short:         "Generate zsh completion script",
			SilenceErrors: true,
			SilenceUsage:  true,
			Args:          cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return root.GenZshCompletion(cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:           "fish",
			Short:         "Generate fish completion script",
			SilenceErrors: true,
			SilenceUsage:  true,
			Args:          cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			},
		},
	)

	return cmd
}
