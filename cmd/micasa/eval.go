// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/micasa-dev/micasa/internal/config"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/ftseval"
	"github.com/micasa-dev/micasa/internal/llm"
	"github.com/spf13/cobra"
)

// evalOpts mirrors ftseval.Config plus CLI-only knobs. Populated by
// Cobra flag parsing; validated inside RunE.
type evalOpts struct {
	dbPath     string
	provider   string
	model      string
	judgeModel string
	questions  []string
	skipJudge  bool
	noAB       bool
	format     string
	output     string
	strict     bool
}

// newEvalCmd returns the `micasa eval` parent command. Sub-evals slot in
// as children (`eval fts`, future `eval extraction`, etc.).
func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "eval",
		Short:         "Run chat-quality benchmarks against a fixture or user DB",
		Long:          `Parent command for chat-quality evaluations. See subcommands.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.AddCommand(newEvalFTSCmd())
	return cmd
}

func newEvalFTSCmd() *cobra.Command {
	opts := &evalOpts{}
	cmd := &cobra.Command{
		Use:   "fts",
		Short: "Run the FTS context-enrichment chat benchmark",
		Long: `Run the FTS chat benchmark against the default fixture DB or a
user-supplied SQLite file. Each question runs twice (FTS on and FTS off) and
is graded by a deterministic regex rubric, with an optional LLM judge pass.

The eval uses the chat config from the user's config file; --provider and
--model override specific fields. Pointing --db at a real micasa DB sends
prompts derived from household data to the configured provider -- if that
provider is a cloud service, the data leaves the machine.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvalFTS(cmd.OutOrStdout(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.dbPath, "db", "",
		"path to a micasa SQLite DB (default: fixture)")
	cmd.Flags().StringVar(&opts.provider, "provider", "",
		"override chat provider from config")
	cmd.Flags().StringVar(&opts.model, "model", "",
		"override chat model from config")
	cmd.Flags().StringVar(&opts.judgeModel, "judge-model", "",
		"model for the LLM judge (default: same as --model)")
	cmd.Flags().StringSliceVar(&opts.questions, "questions", nil,
		"comma-separated names of questions to run (default: all)")
	cmd.Flags().BoolVar(&opts.skipJudge, "skip-judge", false,
		"deterministic rubric only; skip the LLM judge")
	cmd.Flags().BoolVar(&opts.noAB, "no-ab", false,
		"run each question once (FTS on) instead of twice")
	cmd.Flags().StringVar(&opts.format, "format", "",
		"report format: table (default when TTY), markdown, or json")
	cmd.Flags().StringVar(&opts.output, "output", "",
		"write report to this file instead of stdout")
	cmd.Flags().BoolVar(&opts.strict, "strict", false,
		"exit non-zero on per-question rubric regression (completed on both arms)")

	return cmd
}

func runEvalFTS(defaultOut interface {
	Write([]byte) (int, error)
}, opts *evalOpts,
) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	chatLLM := cfg.Chat.LLM
	provider := opts.provider
	if provider == "" {
		provider = chatLLM.Provider
	}
	model := opts.model
	if model == "" {
		model = chatLLM.Model
	}
	judgeModel := opts.judgeModel
	if judgeModel == "" {
		judgeModel = model
	}
	timeout := chatLLM.TimeoutDuration()
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	// Privacy warning when running against a real DB on a non-local
	// provider.
	if opts.dbPath != "" && !isLocalLLMProvider(provider) {
		fmt.Fprintf(os.Stderr,
			"warning: eval will send prompts derived from %s to %s.\n"+
				"Press Ctrl-C within 5s to abort.\n",
			opts.dbPath, provider,
		)
		time.Sleep(5 * time.Second)
	}

	// Open (or build) the store.
	store, fixture, cleanup, err := openEvalStore(opts.dbPath)
	if err != nil {
		return err
	}
	defer cleanup()

	// Build LLM clients.
	client, err := llm.NewClient(provider, chatLLM.BaseURL, model, chatLLM.APIKey, timeout)
	if err != nil {
		return fmt.Errorf("build chat client: %w", err)
	}
	judge := client
	if judgeModel != model {
		judge, err = llm.NewClient(provider, chatLLM.BaseURL, judgeModel, chatLLM.APIKey, timeout)
		if err != nil {
			return fmt.Errorf("build judge client: %w", err)
		}
	}

	harnessCfg := ftseval.Config{
		DBPath:     opts.dbPath,
		Provider:   provider,
		Model:      model,
		JudgeModel: judgeModel,
		APIKey:     chatLLM.APIKey,
		Timeout:    timeout,
		Questions:  opts.questions,
		SkipJudge:  opts.skipJudge,
		NoAB:       opts.noAB,
		Format:     opts.format,
		Strict:     opts.strict,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	results, err := ftseval.Run(ctx, harnessCfg, store, fixture, client, judge)
	if err != nil {
		return fmt.Errorf("run eval: %w", err)
	}

	// Write report. Default format: "table" when writing to a TTY,
	// "markdown" otherwise (pipes, files, CI). --format overrides.
	out := io.Writer(defaultOut)
	if opts.output != "" {
		f, err := os.Create(opts.output)
		if err != nil {
			return fmt.Errorf("open report file: %w", err)
		}
		defer func() { _ = f.Close() }()
		out = f
	}
	if harnessCfg.Format == "" {
		if writerIsTerminal(out) {
			harnessCfg.Format = "table"
		} else {
			harnessCfg.Format = "markdown"
		}
	}
	if err := ftseval.WriteReport(out, harnessCfg, results); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	if code := ftseval.ExitCode(harnessCfg, results); code != 0 {
		os.Exit(code)
	}
	return nil
}

// openEvalStore returns either the user-supplied SQLite store or a
// freshly-seeded fixture. The returned cleanup closes the store and, for
// the fixture path, removes the tempdir the fixture lives in.
func openEvalStore(
	dbPath string,
) (*data.Store, ftseval.SeededFixture, func(), error) {
	if dbPath != "" {
		s, err := data.Open(dbPath)
		if err != nil {
			return nil, ftseval.SeededFixture{}, nil, fmt.Errorf("open %s: %w", dbPath, err)
		}
		cleanup := func() { _ = s.Close() }
		return s, ftseval.SeededFixture{}, cleanup, nil
	}

	tmp, err := os.MkdirTemp("", "micasa-eval-*")
	if err != nil {
		return nil, ftseval.SeededFixture{}, nil, fmt.Errorf("create fixture tempdir: %w", err)
	}
	removeTmp := func() { _ = os.RemoveAll(tmp) }

	path := tmp + "/fixture.db"
	s, err := data.Open(path)
	if err != nil {
		removeTmp()
		return nil, ftseval.SeededFixture{}, nil, fmt.Errorf("open fixture: %w", err)
	}
	closeStore := func() { _ = s.Close() }
	if err := s.AutoMigrate(); err != nil {
		closeStore()
		removeTmp()
		return nil, ftseval.SeededFixture{}, nil, fmt.Errorf("migrate fixture: %w", err)
	}
	if err := s.SeedDefaults(); err != nil {
		closeStore()
		removeTmp()
		return nil, ftseval.SeededFixture{}, nil, fmt.Errorf("seed fixture defaults: %w", err)
	}
	fx, err := ftseval.SeedFixture(s)
	if err != nil {
		closeStore()
		removeTmp()
		return nil, ftseval.SeededFixture{}, nil, fmt.Errorf("seed fixture entities: %w", err)
	}

	cleanup := func() {
		closeStore()
		removeTmp()
	}
	return s, fx, cleanup, nil
}

// isLocalLLMProvider reports whether the named provider runs on the same
// machine (so no household data leaves the machine).
func isLocalLLMProvider(provider string) bool {
	switch strings.ToLower(provider) {
	case "ollama", "llamacpp", "llamafile":
		return true
	}
	return false
}
