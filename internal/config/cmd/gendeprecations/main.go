// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// gendeprecations produces docs/data/deprecations.json from Config struct tags.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/micasa-dev/micasa/internal/config"
)

type entry struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Hint    string `json:"hint,omitempty"`
}

func main() {
	output := flag.String("output", "", "output file path")
	flag.Parse()

	if *output == "" {
		fmt.Fprintln(os.Stderr, "usage: gendeprecations -output <path>")
		os.Exit(1)
	}

	deps := config.Deprecations()
	entries := make([]entry, len(deps))
	for i, d := range deps {
		entries[i] = entry{
			OldPath: d.OldPath,
			NewPath: d.NewPath,
			Hint:    d.HintText(),
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(*output), 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*output, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
}
