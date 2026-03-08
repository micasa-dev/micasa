// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/alecthomas/kong"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cpcloud/micasa/internal/app"
	"github.com/cpcloud/micasa/internal/config"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/extract"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

type cli struct {
	Run     runCmd           `cmd:"" default:"withargs" help:"Launch the TUI (default)."`
	Backup  backupCmd        `cmd:""                    help:"Back up the database to a file."`
	Config  configCmd        `cmd:""                    help:"Print config values or dump the full resolved config."`
	Version kong.VersionFlag `                          help:"Show version and exit."                                name:"version"`
}

type runCmd struct {
	DBPath    string `arg:"" optional:"" help:"SQLite database path. Pass with --demo to persist demo data."        env:"MICASA_DB_PATH"`
	Demo      bool   `                   help:"Launch with sample data in an in-memory database."`
	Years     int    `                   help:"Generate N years of simulated home ownership data. Requires --demo."`
	PrintPath bool   `                   help:"Print the resolved database path and exit."`
}

type backupCmd struct {
	Dest   string `arg:"" optional:"" help:"Destination file path. Defaults to <source>.backup."`
	Source string `                   help:"Source database path. Defaults to the standard location." default:"" env:"MICASA_DB_PATH"`
}

type configCmd struct {
	Key  string `arg:"" optional:"" help:"Dot-delimited config key (e.g. llm.model, documents.max_file_size)."`
	Dump bool   `                   help:"Print the fully resolved config as TOML and exit."`
}

func main() {
	var c cli
	kctx := kong.Parse(&c,
		kong.Name(data.AppName),
		kong.Description("A terminal UI for tracking everything about your home."),
		kong.UsageOnError(),
		kong.Vars{"version": versionString()},
	)
	if err := kctx.Run(); err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", data.AppName, err)
		os.Exit(1)
	}
}

func (cmd *runCmd) Run() error {
	dbPath, err := cmd.resolveDBPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}
	if cmd.PrintPath {
		fmt.Println(dbPath)
		return nil
	}
	if cmd.Years > 0 && !cmd.Demo {
		return fmt.Errorf("--years requires --demo")
	}
	if cmd.Years < 0 {
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
	if cmd.Demo {
		if cmd.Years > 0 {
			summary, err := store.SeedScaledData(cmd.Years)
			if err != nil {
				return fmt.Errorf("seed scaled data: %w", err)
			}
			fmt.Fprintf(
				os.Stderr,
				"seeded %d years: %d vendors, %d projects, %d appliances, %d maintenance, %d service logs, %d quotes, %d documents\n",
				cmd.Years,
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

	opts := app.Options{
		DBPath:        dbPath,
		ConfigPath:    config.Path(),
		FilePickerDir: cfg.Documents.ResolvedFilePickerDir(),
	}

	chatCfg := cfg.LLM.ChatConfig()
	opts.SetLLM(
		chatCfg.Provider,
		chatCfg.BaseURL,
		chatCfg.Model,
		chatCfg.APIKey,
		chatCfg.ExtraContext,
		chatCfg.Timeout,
		chatCfg.Thinking,
	)

	exCfg := cfg.LLM.ExtractionConfig()
	extractors := extract.DefaultExtractors(
		cfg.Extraction.MaxExtractPages,
		cfg.Extraction.TextTimeoutDuration(),
	)
	opts.SetExtraction(
		exCfg.Provider,
		exCfg.BaseURL,
		exCfg.Model,
		exCfg.APIKey,
		exCfg.Timeout,
		exCfg.Thinking,
		extractors,
		cfg.Extraction.IsEnabled(),
		cfg.Extraction.LLMTimeoutDuration(),
	)

	model, err := app.NewModel(store, opts)
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

func (cmd *runCmd) resolveDBPath() (string, error) {
	if cmd.DBPath != "" {
		return data.ExpandHome(cmd.DBPath), nil
	}
	if cmd.Demo {
		return ":memory:", nil
	}
	return data.DefaultDBPath()
}

func (cmd *configCmd) Run() error {
	if !cmd.Dump && cmd.Key == "" {
		return fmt.Errorf("provide a config key or use --dump to print the full config")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cmd.Dump {
		return cfg.ShowConfig(os.Stdout)
	}
	val, err := cfg.Get(cmd.Key)
	if err != nil {
		return err
	}
	fmt.Println(val)
	return nil
}

func (cmd *backupCmd) Run() error {
	sourcePath := cmd.Source
	if sourcePath == "" {
		var err error
		sourcePath, err = data.DefaultDBPath()
		if err != nil {
			return fmt.Errorf("resolve source path: %w", err)
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

	destPath := cmd.Dest
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
	fmt.Println(absPath)
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
