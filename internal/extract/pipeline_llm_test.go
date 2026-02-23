// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestLLMServer creates an httptest server that returns the given JSON
// as an OpenAI-compatible chat completion response. This is the same
// shape that Ollama/llama.cpp serve.
func newTestLLMServer(t *testing.T, responseJSON string) (*httptest.Server, *llm.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		_, _ = fmt.Fprintf(w,
			`{"choices":[{"message":{"content":%q}}]}`,
			responseJSON,
		)
	}))
	t.Cleanup(srv.Close)
	client := llm.NewClient(srv.URL+"/v1", "test-model", 5*time.Second)
	return srv, client
}

// TestPipeline_LLMExtractsHintsFromText exercises the full pipeline path
// a user hits when they save a text document with an LLM configured:
// text extraction runs, OCR is skipped (not a PDF/image), then the LLM
// receives the text and returns structured hints.
func TestPipeline_LLMExtractsHintsFromText(t *testing.T) {
	extractionJSON := `{
		"document_type": "invoice",
		"title_suggestion": "Garcia Plumbing Invoice",
		"vendor_hint": "Garcia Plumbing",
		"total_cents": 150000,
		"date": "2025-03-15",
		"entity_kind_hint": "vendor",
		"entity_name_hint": "Garcia Plumbing"
	}`
	_, client := newTestLLMServer(t, extractionJSON)

	p := &Pipeline{
		LLMClient: client,
		EntityContext: EntityContext{
			Vendors: []string{"Garcia Plumbing", "Acme Electric"},
		},
	}

	docText := "GARCIA PLUMBING LLC\nInvoice #1234\nDate: March 15, 2025\nTotal: $1,500.00"
	r := p.Run(context.Background(), []byte(docText), "invoice.txt", "text/plain")

	require.NoError(t, r.Err)
	assert.Equal(t, docText, r.Text())
	assert.False(t, r.HasSource("tesseract"))
	assert.True(t, r.LLMUsed)

	require.NotNil(t, r.Hints)
	assert.Equal(t, "invoice", r.Hints.DocumentType)
	assert.Equal(t, "Garcia Plumbing Invoice", r.Hints.TitleSugg)
	assert.Equal(t, "Garcia Plumbing", r.Hints.VendorHint)
	require.NotNil(t, r.Hints.TotalCents)
	assert.Equal(t, int64(150000), *r.Hints.TotalCents)
	require.NotNil(t, r.Hints.Date)
	assert.Equal(t, 2025, r.Hints.Date.Year())
}

// TestPipeline_LLMServerDown verifies that when the LLM server is
// unreachable, the pipeline still returns the extracted text -- the
// error is captured in Result.Err but doesn't prevent the document
// from being saved.
func TestPipeline_LLMServerDown(t *testing.T) {
	// Point at a port that's not listening.
	client := llm.NewClient("http://127.0.0.1:1/v1", "test-model", time.Second)

	p := &Pipeline{LLMClient: client}
	r := p.Run(context.Background(), []byte("Some invoice text"), "invoice.txt", "text/plain")

	// Text extraction succeeded.
	assert.Equal(t, "Some invoice text", r.Text())
	// LLM failed gracefully.
	assert.False(t, r.LLMUsed)
	assert.Nil(t, r.Hints)
	assert.Error(t, r.Err)
	assert.Contains(t, r.Err.Error(), "llm extraction")
}

// TestPipeline_LLMGarbageResponse verifies that when the LLM returns
// unparseable JSON, the pipeline captures the error without crashing.
func TestPipeline_LLMGarbageResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w,
			`{"choices":[{"message":{"content":"I don't understand the question"}}]}`,
		)
	}))
	t.Cleanup(srv.Close)
	client := llm.NewClient(srv.URL+"/v1", "test-model", 5*time.Second)

	p := &Pipeline{LLMClient: client}
	r := p.Run(context.Background(), []byte("invoice text"), "doc.txt", "text/plain")

	assert.Equal(t, "invoice text", r.Text())
	assert.False(t, r.LLMUsed)
	assert.Nil(t, r.Hints)
	assert.Error(t, r.Err)
	assert.Contains(t, r.Err.Error(), "parse llm response")
}

// TestPipeline_LLMSkippedWithoutText verifies that the LLM step is not
// called when there's no extracted text (e.g. a binary file).
func TestPipeline_LLMSkippedWithoutText(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	t.Cleanup(srv.Close)
	client := llm.NewClient(srv.URL+"/v1", "test-model", 5*time.Second)

	p := &Pipeline{LLMClient: client}
	r := p.Run(context.Background(), []byte{0xFF, 0xD8}, "photo.bin", "application/octet-stream")

	assert.NoError(t, r.Err)
	assert.Empty(t, r.Text())
	assert.False(t, r.LLMUsed)
	assert.False(t, called, "LLM should not be called when there's no text to analyze")
}
