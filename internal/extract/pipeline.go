// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"fmt"
	"strings"

	"github.com/cpcloud/micasa/internal/llm"
)

// Pipeline orchestrates the document extraction layers: text extraction,
// OCR, and LLM-powered structured extraction. Each layer is independent
// and gracefully degrades when its dependencies are unavailable.
type Pipeline struct {
	LLMClient  *llm.Client   // nil = skip LLM extraction
	Extractors []Extractor   // nil = DefaultExtractors(0, 0)
	Schema     SchemaContext // DDL + entity rows for prompt
	DocID      uint          // document ID for UPDATE operations
}

// Result holds the output of a pipeline run.
type Result struct {
	Sources    []TextSource // text from each extraction method
	Operations []Operation  // nil if LLM unavailable or failed
	LLMRaw     string       // raw LLM output (for display)
	LLMUsed    bool
	Err        error // non-fatal extraction error; document still saves
}

// Text returns the first non-empty text from the extraction sources.
func (r *Result) Text() string {
	for _, s := range r.Sources {
		if strings.TrimSpace(s.Text) != "" {
			return s.Text
		}
	}
	return ""
}

// SourceByTool returns the first source matching the given tool name,
// or nil if not found.
func (r *Result) SourceByTool(tool string) *TextSource {
	for i := range r.Sources {
		if r.Sources[i].Tool == tool {
			return &r.Sources[i]
		}
	}
	return nil
}

// HasSource reports whether any source matches the given tool name.
func (r *Result) HasSource(tool string) bool {
	return r.SourceByTool(tool) != nil
}

// Run executes the extraction pipeline on the given document data.
// It never returns a Go error -- all failures are captured in Result.Err
// so the caller can save the document regardless.
func (p *Pipeline) Run(
	ctx context.Context,
	data []byte,
	filename string,
	mime string,
) *Result {
	r := &Result{}
	if len(data) == 0 {
		return r
	}

	extractors := p.Extractors
	if extractors == nil {
		extractors = DefaultExtractors(0, 0)
	}

	// Run all matching, available extractors.
	for _, ext := range extractors {
		if !ext.Matches(mime) || !ext.Available() {
			continue
		}
		src, err := ext.Extract(ctx, data)
		if err != nil {
			r.Err = fmt.Errorf("%s: %w", ext.Tool(), err)
			continue
		}
		if strings.TrimSpace(src.Text) != "" || len(src.Data) > 0 {
			r.Sources = append(r.Sources, src)
		}
	}

	// LLM extraction if client configured and any text available.
	if p.LLMClient != nil && r.Text() != "" {
		ops, raw, llmErr := p.extractWithLLM(
			ctx,
			r.Sources,
			filename,
			mime,
			int64(len(data)),
		)
		if llmErr != nil {
			r.Err = fmt.Errorf("llm extraction: %w", llmErr)
		} else if len(ops) > 0 {
			r.Operations = ops
			r.LLMRaw = raw
			r.LLMUsed = true
		}
	}

	return r
}

// extractWithLLM runs the LLM extraction model on the text sources and
// validates the resulting operations.
func (p *Pipeline) extractWithLLM(
	ctx context.Context,
	sources []TextSource,
	filename string,
	mime string,
	sizeBytes int64,
) ([]Operation, string, error) {
	messages := BuildExtractionPrompt(ExtractionPromptInput{
		DocID:     p.DocID,
		Filename:  filename,
		MIME:      mime,
		SizeBytes: sizeBytes,
		Schema:    p.Schema,
		Sources:   sources,
	})

	raw, err := p.LLMClient.ChatComplete(ctx, messages)
	if err != nil {
		return nil, "", fmt.Errorf("llm chat: %w", err)
	}

	ops, err := ParseOperations(raw)
	if err != nil {
		return nil, raw, fmt.Errorf("parse llm operations: %w", err)
	}

	if err := ValidateOperations(ops, ExtractionAllowedOps); err != nil {
		return nil, raw, fmt.Errorf("validate llm operations: %w", err)
	}

	return ops, raw, nil
}
