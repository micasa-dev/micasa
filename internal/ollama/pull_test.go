// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullModelSuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/pull", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"status":"pulling manifest"}`)
		_, _ = fmt.Fprintln(
			w,
			`{"status":"downloading","digest":"sha256:abc","total":1000,"completed":500}`,
		)
		_, _ = fmt.Fprintln(w, `{"status":"success"}`)
	}))
	defer srv.Close()

	scanner, err := PullModel(t.Context(), srv.URL, "qwen3")
	require.NoError(t, err)

	chunk, err := scanner.Next()
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "pulling manifest", chunk.Status)

	chunk, err = scanner.Next()
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "downloading", chunk.Status)
	assert.Equal(t, "sha256:abc", chunk.Digest)
	assert.Equal(t, int64(1000), chunk.Total)
	assert.Equal(t, int64(500), chunk.Completed)

	chunk, err = scanner.Next()
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "success", chunk.Status)

	// EOF
	chunk, err = scanner.Next()
	require.NoError(t, err)
	assert.Nil(t, chunk)
}

func TestPullModelServerDown(t *testing.T) {
	t.Parallel()
	_, err := PullModel(t.Context(), "http://127.0.0.1:1", "qwen3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot reach")
	assert.Contains(t, err.Error(), "ollama serve")
}

func TestPullModelServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `model not found`)
	}))
	defer srv.Close()

	_, err := PullModel(t.Context(), srv.URL, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pull failed (404)")
	assert.Contains(t, err.Error(), "model not found")
}

func TestPullModelTrailingSlash(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/pull", r.URL.Path,
			"trailing slash should be stripped from base URL")
		_, _ = fmt.Fprintln(w, `{"status":"success"}`)
	}))
	defer srv.Close()

	scanner, err := PullModel(t.Context(), srv.URL+"/", "model")
	require.NoError(t, err)

	chunk, err := scanner.Next()
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "success", chunk.Status)
}

func TestPullScannerSkipsBlankLines(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, `{"status":"done"}`)
		_, _ = fmt.Fprintln(w, "  ")
	}))
	defer srv.Close()

	scanner, err := PullModel(t.Context(), srv.URL, "model")
	require.NoError(t, err)

	chunk, err := scanner.Next()
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "done", chunk.Status)

	// EOF after blank lines
	chunk, err = scanner.Next()
	require.NoError(t, err)
	assert.Nil(t, chunk)
}

func TestPullScannerSkipsMalformedJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `not json`)
		_, _ = fmt.Fprintln(w, `{"status":"ok"}`)
	}))
	defer srv.Close()

	scanner, err := PullModel(t.Context(), srv.URL, "model")
	require.NoError(t, err)

	chunk, err := scanner.Next()
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "ok", chunk.Status)
}

func TestPullChunkErrorField(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `{"status":"error","error":"pull failed: unauthorized"}`)
	}))
	defer srv.Close()

	scanner, err := PullModel(t.Context(), srv.URL, "model")
	require.NoError(t, err)

	chunk, err := scanner.Next()
	require.NoError(t, err)
	require.NotNil(t, chunk)
	assert.Equal(t, "error", chunk.Status)
	assert.Equal(t, "pull failed: unauthorized", chunk.Error)
}

func TestPullModelCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := PullModel(ctx, "http://localhost:11434", "model")
	require.Error(t, err)
}
