// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cpcloud/micasa/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestLLMServer creates an httptest server that returns the given response
// as an OpenAI-compatible chat completion response. This is the same
// shape that Ollama/llama.cpp serve.
func newTestLLMServer(t *testing.T, responseContent string) (*httptest.Server, *llm.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		_, _ = fmt.Fprintf(w,
			`{"choices":[{"message":{"content":%s}}]}`,
			mustMarshalJSON(t, responseContent),
		)
	}))
	t.Cleanup(srv.Close)
	client := llm.NewClient(srv.URL+"/v1", "test-model", 5*time.Second)
	return srv, client
}

func mustMarshalJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	require.NoError(t, err)
	return string(b)
}

// TestPipeline_LLMExtractsOperationsFromText exercises the full pipeline path
// a user hits when they save a text document with an LLM configured:
// text extraction runs, OCR is skipped (not a PDF/image), then the LLM
// receives the text and returns JSON operations.
func TestPipeline_LLMExtractsOperationsFromText(t *testing.T) {
	opsJSON := `[
		{"action": "update", "table": "documents", "data": {"id": 42, "title": "Garcia Plumbing Invoice", "notes": "Plumbing repair invoice"}},
		{"action": "create", "table": "vendors", "data": {"name": "Garcia Plumbing"}}
	]`
	_, client := newTestLLMServer(t, opsJSON)

	p := &Pipeline{
		LLMClient: client,
		DocID:     42,
		Schema: SchemaContext{
			Vendors: []EntityRow{{ID: 1, Name: "Existing Vendor"}},
		},
	}

	docText := "GARCIA PLUMBING LLC\nInvoice #1234\nDate: March 15, 2025\nTotal: $1,500.00"
	r := p.Run(context.Background(), []byte(docText), "invoice.txt", "text/plain")

	require.NoError(t, r.Err)
	assert.Equal(t, docText, r.Text())
	assert.False(t, r.HasSource("tesseract"))
	assert.True(t, r.LLMUsed)

	require.Len(t, r.Operations, 2)
	assert.Equal(t, "update", r.Operations[0].Action)
	assert.Equal(t, "documents", r.Operations[0].Table)
	assert.Equal(t, "create", r.Operations[1].Action)
	assert.Equal(t, "vendors", r.Operations[1].Table)
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
	assert.Empty(t, r.Operations)
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
	assert.Empty(t, r.Operations)
	assert.Error(t, r.Err)
	assert.Contains(t, r.Err.Error(), "parse llm operations")
}

// TestPipeline_LLMSkippedWithoutText verifies that the LLM step is not
// called when there's no extracted text (e.g. a binary file).
func TestPipeline_LLMSkippedWithoutText(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"[]"}}]}`)
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

// TestPipeline_LLMForbiddenAction verifies that a forbidden action from the
// LLM is caught by validation and reported as an error.
func TestPipeline_LLMForbiddenAction(t *testing.T) {
	opsJSON := `[{"action": "delete", "table": "vendors", "data": {"id": 1}}]`
	_, client := newTestLLMServer(t, opsJSON)

	p := &Pipeline{LLMClient: client, DocID: 1}
	r := p.Run(context.Background(), []byte("some text"), "doc.txt", "text/plain")

	assert.False(t, r.LLMUsed)
	assert.Empty(t, r.Operations)
	assert.Error(t, r.Err)
	assert.Contains(t, r.Err.Error(), "action must be")
}

// TestPipeline_LLMForbiddenTable verifies that writing to an unknown table
// is caught by validation.
func TestPipeline_LLMForbiddenTable(t *testing.T) {
	opsJSON := `[{"action": "create", "table": "users", "data": {"name": "hacker"}}]`
	_, client := newTestLLMServer(t, opsJSON)

	p := &Pipeline{LLMClient: client, DocID: 1}
	r := p.Run(context.Background(), []byte("some text"), "doc.txt", "text/plain")

	assert.False(t, r.LLMUsed)
	assert.Empty(t, r.Operations)
	assert.Error(t, r.Err)
	assert.Contains(t, r.Err.Error(), "not in the allowed set")
}
