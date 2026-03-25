// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
)

func newQueryCmd() *cobra.Command {
	var jsonFlag bool

	cmd := &cobra.Command{
		Use:   "query <sql> [database-path]",
		Short: "Run a read-only SQL query",
		Long: `Execute a validated SELECT query against the database.
Only SELECT/WITH statements are allowed. Results are capped at 200 rows
with a 10-second timeout.`,
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			}
			store, err := openExisting(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return runQuery(cmd.Context(), cmd.OutOrStdout(), store, args[0], jsonFlag)
		},
	}

	cmd.Flags().BoolVar(&jsonFlag, "json", false, "Output as JSON")
	return cmd
}

func runQuery(ctx context.Context, w io.Writer, store *data.Store, sql string, asJSON bool) error {
	columns, rows, err := store.ReadOnlyQuery(ctx, sql)
	if err != nil {
		return err
	}

	if asJSON {
		return writeQueryJSON(w, columns, rows)
	}
	return writeQueryText(w, columns, rows)
}

func writeQueryText(w io.Writer, columns []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(columns, "\t")); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush output: %w", err)
	}
	return nil
}

func writeQueryJSON(w io.Writer, columns []string, rows [][]string) error {
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		obj := make(map[string]any, len(columns))
		for j, col := range columns {
			if j < len(row) {
				obj[col] = row[j]
			}
		}
		out[i] = obj
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
