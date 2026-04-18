// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchDocumentsBasic(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create documents with extracted text.
	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Plumber Receipt",
		FileName:      "receipt.pdf",
		ExtractedText: "Invoice from ABC Plumbing for kitchen sink repair",
		Notes:         "paid in full",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title:         "HVAC Manual",
		FileName:      "manual.pdf",
		ExtractedText: "Installation guide for central air conditioning unit",
	}))

	results, err := store.SearchDocuments("plumb")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Plumber Receipt", results[0].Title)
	assert.Contains(t, results[0].Snippet, "Plumb")
}

func TestSearchDocumentsMatchesTitle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Kitchen Renovation Quote",
		FileName: "quote.pdf",
	}))

	results, err := store.SearchDocuments("kitchen")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Kitchen Renovation Quote", results[0].Title)
}

func TestSearchDocumentsMatchesNotes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Receipt",
		FileName: "r.pdf",
		Notes:    "emergency plumbing repair on Sunday",
	}))

	results, err := store.SearchDocuments("emergency")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Receipt", results[0].Title)
}

func TestSearchDocumentsExcludesSoftDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Deleted Doc",
		FileName:      "deleted.pdf",
		ExtractedText: "plumber invoice",
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)

	require.NoError(t, store.DeleteDocument(docs[0].ID))

	results, err := store.SearchDocuments("plumber")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchDocumentsEmptyQuery(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Something",
		FileName: "s.pdf",
	}))

	results, err := store.SearchDocuments("")
	require.NoError(t, err)
	assert.Nil(t, results)

	results, err = store.SearchDocuments("   ")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSearchDocumentsMultipleMatches(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Receipt 1",
		FileName:      "r1.pdf",
		ExtractedText: "plumber fixed the kitchen sink",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Receipt 2",
		FileName:      "r2.pdf",
		ExtractedText: "plumber replaced bathroom faucet",
	}))
	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Unrelated",
		FileName: "u.pdf",
	}))

	results, err := store.SearchDocuments("plumber")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSearchDocumentsPorterStemming(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Painting Invoice",
		FileName:      "inv.pdf",
		ExtractedText: "Professional painting services rendered",
	}))

	// "painted" should match "painting" via porter stemmer (both stem to "paint").
	results, err := store.SearchDocuments("painted")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestSearchDocumentsUpdateReflected(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Old Title",
		FileName:      "doc.pdf",
		ExtractedText: "original text about gardening",
	}))
	docs, err := store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	id := docs[0].ID

	// Update extraction text.
	require.NoError(t, store.UpdateDocumentExtraction(id, "new text about plumbing", nil, "", nil))

	results, err := store.SearchDocuments("plumbing")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, id, results[0].ID)

	// Old text should no longer match.
	results, err = store.SearchDocuments("gardening")
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestSearchDocumentsMalformedTokenizes pins the design intent of the
// phrase-wrap escape: when a user types something with stray delimiters
// like "(kitchen" or "kitchen)", the FTS5 tokenizer extracts the inner
// word and the prefix match still works. This is desirable for type-as-
// you-go search where partial input should still surface relevant
// results, not an accidental matching bug.
func TestSearchDocumentsMalformedTokenizes(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Kitchen Renovation",
		FileName:      "k.pdf",
		ExtractedText: "plumber notes",
	}))

	for _, q := range []string{`(kitchen`, `kitchen)`, `"kitchen`, `kitchen*`} {
		t.Run(q, func(t *testing.T) {
			results, err := store.SearchDocuments(q)
			require.NoError(t, err)
			require.Len(t, results, 1, "delimiters around %q should not block tokenization", q)
			assert.Equal(t, "Kitchen Renovation", results[0].Title)
		})
	}
}

// TestSearchDocumentsBadSyntaxGraceful verifies that inputs which would
// be malformed FTS5 expressions if passed verbatim do not error out and
// also do not accidentally match real documents. A document is inserted
// so the no-match assertion is meaningful (an empty store would pass
// even if the query rewrite broadened matches).
func TestSearchDocumentsBadSyntaxGraceful(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Document content uses tokens that don't share any prefix with
	// the test queries below, so any spurious match indicates a bug
	// in the rewrite (e.g., a query collapsing to a bare wildcard).
	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Zebra",
		FileName:      "z.pdf",
		ExtractedText: "rhinoceros giraffe leopard",
	}))

	bad := []string{
		`"unclosed`,
		`unclosed"`,
		`(kitchen`,
		`kitchen)`,
		`((nested`,
		`"phrase with "" inside`,
		`***`,
		`:::`,
		`+++---`,
		`(b AND)`,
	}
	for _, q := range bad {
		t.Run(q, func(t *testing.T) {
			results, err := store.SearchDocuments(q)
			require.NoError(t, err)
			assert.Empty(t, results)
		})
	}
}

func TestSearchDocumentsEntityFields(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Project Doc",
		FileName:      "pd.pdf",
		EntityKind:    DocumentEntityProject,
		EntityID:      "01JTEST00000000000000042",
		ExtractedText: "kitchen renovation details",
	}))

	results, err := store.SearchDocuments("kitchen")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, DocumentEntityProject, results[0].EntityKind)
	assert.Equal(t, "01JTEST00000000000000042", results[0].EntityID)
}

func TestSearchDocumentsSnippetFromBestColumn(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Match is in title only -- snippet should reflect the title.
	require.NoError(t, store.CreateDocument(&Document{
		Title:    "Plumber Receipt",
		FileName: "receipt.pdf",
	}))

	results, err := store.SearchDocuments("plumber")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(
		t,
		results[0].Snippet,
		"Plumb",
		"snippet should come from title when that's the matching column",
	)
}

func TestSearchDocumentsCaseInsensitive(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "HVAC Manual",
		FileName:      "hvac.pdf",
		ExtractedText: "Central Air Conditioning INSTALLATION Guide",
	}))

	// All case variants should match.
	for _, q := range []string{"hvac", "HVAC", "Hvac", "installation", "GUIDE"} {
		results, err := store.SearchDocuments(q)
		require.NoError(t, err, "query %q should not error", q)
		assert.Len(t, results, 1, "query %q should match", q)
	}
}

func TestPrepareFTSQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"hello", `"hello"*`},
		{"hello world", `"hello"* "world"*`},
		// Operators become literal phrase tokens, not FTS5 operators.
		{"a AND b", `"a"* "AND"* "b"*`},
		{`"exact phrase"`, `"""exact"* "phrase"""*`},
		// Internal " is doubled per FTS5's escape rule.
		{`say "hi"`, `"say"* """hi"""*`},
		// All-special tokens stay wrapped; FTS5 tokenizes them to nothing
		// and the phrase matches no documents (verified in integration tests).
		{"***", `"***"*`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, prepareFTSQuery(tt.in))
		})
	}
}

func TestRebuildFTSIndex(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateDocument(&Document{
		Title:         "Test Doc",
		FileName:      "t.pdf",
		ExtractedText: "searchable content here",
	}))

	require.NoError(t, store.RebuildFTSIndex())

	results, err := store.SearchDocuments("searchable")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestHasFTSTable(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	assert.True(t, store.hasFTSTable())
}
