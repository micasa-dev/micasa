// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/app"
	"github.com/cpcloud/micasa/internal/config"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// runOpts holds flags for the root (TUI launcher) command.
type runOpts struct {
	dbPath    string
	demo      bool
	years     int
	printPath bool
}

// backupOpts holds flags for the backup subcommand.
type backupOpts struct {
	dest      string
	source    string
	envDBPath string // populated from MICASA_DB_PATH in RunE
}

func newRootCmd() *cobra.Command {
	opts := &runOpts{}

	root := &cobra.Command{
		Use:   data.AppName + " [database-path]",
		Short: "A terminal UI for tracking everything about your home",
		Long:  "A terminal UI for tracking everything about your home.",
		// Accept 0 or 1 positional args (optional database path).
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       versionString(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.dbPath = args[0]
			}
			return runTUI(cmd.OutOrStdout(), opts)
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetHelpFunc(styledHelp)
	root.CompletionOptions.HiddenDefaultCmd = true

	root.Flags().
		BoolVar(&opts.demo, "demo", false, "Launch with sample data in an in-memory database")
	root.Flags().
		IntVar(&opts.years, "years", 0, "Generate N years of simulated home ownership data (requires --demo)")
	root.Flags().
		BoolVar(&opts.printPath, "print-path", false, "Print the resolved database path and exit")

	root.AddCommand(
		newBackupCmd(),
		newConfigCmd(),
		newCompletionCmd(root),
	)

	return root
}

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", data.AppName, err)
		os.Exit(1)
	}
}

func runTUI(w io.Writer, opts *runOpts) error {
	dbPath, err := opts.resolveDBPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}
	if opts.printPath {
		fmt.Fprintln(w, dbPath)
		return nil
	}
	if opts.years > 0 && !opts.demo {
		return fmt.Errorf("--years requires --demo")
	}
	if opts.years < 0 {
		return fmt.Errorf("--years must be non-negative")
	}
	store, err := data.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if err := store.AutoMigrate(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if err := store.SeedDefaults(); err != nil {
		return fmt.Errorf("seed defaults: %w", err)
	}
	if opts.demo {
		if opts.years > 0 {
			summary, err := store.SeedScaledData(opts.years)
			if err != nil {
				return fmt.Errorf("seed scaled data: %w", err)
			}
			fmt.Fprintf(
				os.Stderr,
				"seeded %d years: %d vendors, %d projects, %d appliances, %d maintenance, %d service logs, %d quotes, %d documents\n",
				opts.years,
				summary.Vendors,
				summary.Projects,
				summary.Appliances,
				summary.Maintenance,
				summary.ServiceLogs,
				summary.Quotes,
				summary.Documents,
			)
		} else {
			if err := store.SeedDemoData(); err != nil {
				return fmt.Errorf("seed demo data: %w", err)
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(cfg.Warnings) > 0 {
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "#B8860B", Dark: "#F0E442", // Wong yellow
		})
		for _, w := range cfg.Warnings {
			fmt.Fprintln(os.Stderr, warnStyle.Render("warning:")+" "+w)
		}
	}
	if err := store.SetMaxDocumentSize(cfg.Documents.MaxFileSize.Bytes()); err != nil {
		return fmt.Errorf("configure document size limit: %w", err)
	}
	cacheDir, err := data.DocumentCacheDir()
	if err != nil {
		return fmt.Errorf("resolve document cache directory: %w", err)
	}
	if _, err := data.EvictStaleCache(cacheDir, cfg.Documents.CacheTTLDuration()); err != nil {
		return fmt.Errorf("evict stale cache: %w", err)
	}

	if err := store.ResolveCurrency(cfg.Locale.Currency); err != nil {
		return fmt.Errorf("resolve currency: %w", err)
	}

	appOpts := app.Options{
		DBPath:        dbPath,
		ConfigPath:    config.Path(),
		FilePickerDir: cfg.Documents.ResolvedFilePickerDir(),
	}

	chatLLM := cfg.Chat.LLM
	appOpts.SetChat(
		cfg.Chat.IsEnabled(),
		chatLLM.Provider,
		chatLLM.BaseURL,
		chatLLM.Model,
		chatLLM.APIKey,
		chatLLM.ExtraContext,
		chatLLM.TimeoutDuration(),
		chatLLM.Thinking,
	)

	exLLM := cfg.Extraction.LLM
	extractors := extract.DefaultExtractors(
		cfg.Extraction.MaxPages,
		0, // pdftotext uses its own internal default timeout (30s)
		cfg.Extraction.OCR.IsEnabled(),
	)
	appOpts.SetExtraction(
		exLLM.Provider,
		exLLM.BaseURL,
		exLLM.Model,
		exLLM.APIKey,
		exLLM.TimeoutDuration(),
		exLLM.Thinking,
		extractors,
		exLLM.IsEnabled(),
		cfg.Extraction.OCR.TSV.IsEnabled(),
		cfg.Extraction.OCR.TSV.Threshold(),
	)

	model, err := app.NewModel(store, appOpts)
	if err != nil {
		return fmt.Errorf("initialize app: %w", err)
	}
	// Push current title onto the terminal's title stack, set ours, pop on exit.
	fmt.Fprint(os.Stderr, "\033[22;2t\033]2;micasa\007")
	defer fmt.Fprint(os.Stderr, "\033[23;2t")

	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	if err != nil {
		return fmt.Errorf("running program: %w", err)
	}
	return nil
}

func (opts *runOpts) resolveDBPath() (string, error) {
	if opts.dbPath != "" {
		return data.ExpandHome(opts.dbPath), nil
	}
	if opts.demo {
		return ":memory:", nil
	}
	return data.DefaultDBPath()
}

func newBackupCmd() *cobra.Command {
	opts := &backupOpts{}

	cmd := &cobra.Command{
		Use:           "backup [destination]",
		Short:         "Back up the database to a file",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.dest = args[0]
			}
			opts.envDBPath = os.Getenv("MICASA_DB_PATH")
			return runBackup(cmd.OutOrStdout(), opts)
		},
	}

	cmd.Flags().
		StringVar(&opts.source, "source", "", "Source database path (default: standard location, honors MICASA_DB_PATH)")

	return cmd
}

func runBackup(w io.Writer, opts *backupOpts) error {
	sourcePath := opts.source
	if sourcePath == "" {
		if opts.envDBPath != "" {
			sourcePath = opts.envDBPath
		} else {
			var err error
			sourcePath, err = data.DefaultDBPath()
			if err != nil {
				return fmt.Errorf("resolve source path: %w", err)
			}
		}
	} else {
		sourcePath = data.ExpandHome(sourcePath)
	}
	if sourcePath == ":memory:" {
		return fmt.Errorf("cannot back up an in-memory database")
	}
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf(
			"source database %q not found -- check the path or set MICASA_DB_PATH",
			sourcePath,
		)
	}

	destPath := opts.dest
	if destPath == "" {
		destPath = sourcePath + ".backup"
	} else {
		destPath = data.ExpandHome(destPath)
	}

	if err := data.ValidateDBPath(destPath); err != nil {
		return fmt.Errorf("invalid destination: %w", err)
	}
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf(
			"destination %q already exists -- remove it first or choose a different path",
			destPath,
		)
	}

	store, err := data.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = store.Close() }()

	ok, err := store.IsMicasaDB()
	if err != nil {
		return fmt.Errorf("check database schema: %w", err)
	}
	if !ok {
		return fmt.Errorf(
			"%q is not a micasa database -- it must contain vendors, projects, and appliances tables",
			sourcePath,
		)
	}

	if err := store.Backup(context.Background(), destPath); err != nil {
		return err
	}

	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}
	fmt.Fprintln(w, absPath)
	return nil
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "config [filter]",
		Short:         "Manage application configuration",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var filter string
			if len(args) > 0 {
				filter = args[0]
			}
			return runConfigGet(cmd.OutOrStdout(), filter)
		},
	}

	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigEditCmd())

	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "get [filter]",
		Short:         "Query config values with a jq filter (default: identity)",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var filter string
			if len(args) > 0 {
				filter = args[0]
			}
			return runConfigGet(cmd.OutOrStdout(), filter)
		},
	}
}

func runConfigGet(w io.Writer, filter string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	return cfg.Query(w, filter)
}

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "edit",
		Short:         "Open the config file in an editor",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigEdit(config.Path())
		},
	}
}

func runConfigEdit(path string) error {
	if err := config.EnsureConfigFile(path); err != nil {
		return err
	}
	name, args, err := config.EditorCommand(path)
	if err != nil {
		return err
	}
	c := exec.CommandContext( //nolint:gosec // user-controlled editor from $VISUAL/$EDITOR
		context.Background(),
		name,
		args...,
	)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	return nil
}

// versionString returns the version for display. Release builds return
// the version set via ldflags. Dev builds return the short git commit hash
// (with a -dirty suffix if the tree was modified), or "dev" as a last resort.
func versionString() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	var revision string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if revision == "" {
		return version
	}
	if dirty {
		return revision + "-dirty"
	}
	return revision
}
