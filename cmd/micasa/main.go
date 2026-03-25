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
	"os/signal"
	"path/filepath"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/micasa-dev/micasa/internal/app"
	"github.com/micasa-dev/micasa/internal/config"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// runOpts holds flags for the root (TUI launcher) command.
type runOpts struct {
	dbPath    string
	printPath bool
}

// demoOpts holds flags for the demo subcommand.
type demoOpts struct {
	dbPath string
	years  int
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
		BoolVar(&opts.printPath, "print-path", false, "Print the resolved database path and exit")

	root.AddCommand(
		newDemoCmd(),
		newBackupCmd(),
		newConfigCmd(),
		newCompletionCmd(root),
		newProCmd(),
		newMCPCmd(),
		newShowCmd(),
		newQueryCmd(),
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
		_, _ = fmt.Fprintln(w, dbPath)
		return nil
	}
	return launchTUI(dbPath, nil)
}

// seedOpts controls optional demo-data seeding passed from the demo
// subcommand. A nil value means no seeding. A non-nil value always
// triggers demo seeding: years==0 seeds the small fixed demo, years>0
// seeds N years of scaled data.
type seedOpts struct {
	years int
}

func launchTUI(dbPath string, seed *seedOpts) error {
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
	if seed != nil {
		if seed.years > 0 {
			summary, err := store.SeedScaledData(seed.years)
			if err != nil {
				return fmt.Errorf("seed scaled data: %w", err)
			}
			fmt.Fprintf(
				os.Stderr,
				"seeded %d years: %d vendors, %d projects, %d appliances, %d maintenance, %d service logs, %d quotes, %d documents\n",
				seed.years,
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
		isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stderr)
		warnColor := "#F0E442" // Wong yellow (dark bg)
		if !isDark {
			warnColor = "#B8860B" // Wong yellow (light bg)
		}
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(warnColor))
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
		DBPath:          dbPath,
		ConfigPath:      config.Path(),
		FilePickerDir:   cfg.Documents.ResolvedFilePickerDir(),
		AddressAutofill: cfg.Address.IsAutofillEnabled(),
		AddressCountry:  config.DetectCountry(),
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

	tryLoadSyncConfig(store, &appOpts)

	model, err := app.NewModel(store, appOpts)
	if err != nil {
		return fmt.Errorf("initialize app: %w", err)
	}
	// Push current title onto the terminal's title stack, set ours, pop on exit.
	fmt.Fprint(os.Stderr, "\033[22;2t\033]2;micasa\007")
	defer fmt.Fprint(os.Stderr, "\033[23;2t")

	_, err = tea.NewProgram(model).Run()
	if err != nil {
		return fmt.Errorf("running program: %w", err)
	}
	return nil
}

// resolveDBPath returns the database path to use. Precedence:
// 1. Explicit positional arg (opts.dbPath)
// 2. data.DefaultDBPath(), which honors MICASA_DB_PATH env var internally
func (opts *runOpts) resolveDBPath() (string, error) {
	if opts.dbPath != "" {
		return data.ExpandHome(opts.dbPath), nil
	}
	return data.DefaultDBPath()
}

func newDemoCmd() *cobra.Command {
	opts := &demoOpts{}

	cmd := &cobra.Command{
		Use:           "demo [database-path]",
		Short:         "Launch with sample data in an in-memory database",
		Long:          "Launch with fictitious sample data. Without a path argument, uses an in-memory database.",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.dbPath = args[0]
			}
			return runDemo(opts)
		},
	}

	cmd.Flags().
		IntVar(&opts.years, "years", 0, "Generate N years of simulated home ownership data")

	return cmd
}

// resolveDBPath returns the database path for demo mode. Defaults to
// ":memory:" when no explicit path is given.
func (opts *demoOpts) resolveDBPath() string {
	if opts.dbPath != "" {
		return data.ExpandHome(opts.dbPath)
	}
	return ":memory:"
}

func runDemo(opts *demoOpts) error {
	if opts.years < 0 {
		return fmt.Errorf("--years must be non-negative")
	}
	// Non-nil seedOpts always triggers demo seeding; years==0 seeds the
	// small fixed demo, years>0 seeds N years of scaled data.
	return launchTUI(opts.resolveDBPath(), &seedOpts{years: opts.years})
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

// resolveBackupSource returns the source database path for backup. Precedence:
// 1. Explicit --source flag
// 2. MICASA_DB_PATH env var (passed via opts.envDBPath)
// 3. data.DefaultDBPath() platform default
func (opts *backupOpts) resolveBackupSource() (string, error) {
	if opts.source != "" {
		return data.ExpandHome(opts.source), nil
	}
	if opts.envDBPath != "" {
		return opts.envDBPath, nil
	}
	return data.DefaultDBPath()
}

func runBackup(w io.Writer, opts *backupOpts) error {
	sourcePath, err := opts.resolveBackupSource()
	if err != nil {
		return fmt.Errorf("resolve source path: %w", err)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := store.Backup(ctx, destPath); err != nil {
		return err
	}

	absPath, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}
	_, _ = fmt.Fprintln(w, absPath)
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
	return cfg.Query(context.Background(), w, filter)
}

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "edit",
		Short:         "Open the config file in an editor",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
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
