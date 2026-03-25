// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "mcp [database-path]",
		Short:         "Run MCP server for LLM client access",
		Long:          "Start a Model Context Protocol server over stdio, exposing micasa data to LLM clients like Claude Desktop and Claude Code.",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			dbPath, err := resolveMCPDBPath(args)
			if err != nil {
				return err
			}
			return runMCP(dbPath)
		},
	}
	return cmd
}

func resolveMCPDBPath(args []string) (string, error) {
	if len(args) > 0 {
		return data.ExpandHome(args[0]), nil
	}
	if envPath := os.Getenv("MICASA_DB_PATH"); envPath != "" {
		return data.ExpandHome(envPath), nil
	}
	return data.DefaultDBPath()
}

func runMCP(dbPath string) error {
	store, err := data.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.SetQueryOnly(); err != nil {
		return fmt.Errorf("set query_only: %w", err)
	}

	ok, err := store.IsMicasaDB()
	if err != nil {
		return fmt.Errorf("validate database: %w", err)
	}
	if !ok {
		return fmt.Errorf(
			"not a micasa database: %s -- run 'micasa' first to create and migrate it",
			dbPath,
		)
	}

	srv := mcp.NewServer(store)
	return srv.ServeStdio()
}
