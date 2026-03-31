// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDBPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path    string
		wantErr string // substring of error, "" means no error
	}{
		// Valid paths.
		{path: ":memory:"},
		{path: "/home/user/micasa.db"},
		{path: "relative/path.db"},
		{path: "./local.db"},
		{path: "../parent/db.sqlite"},
		{path: "/tmp/micasa test.db"},
		{path: "C:\\Users\\me\\micasa.db"},

		// URI schemes -- must be rejected.
		{path: "https://evil.com/db", wantErr: "looks like a URI"},
		{path: "http://localhost/db", wantErr: "looks like a URI"},
		{path: "ftp://files.example.com/data.db", wantErr: "looks like a URI"},
		{path: "file://localhost/tmp/test.db", wantErr: "looks like a URI"},

		// file: without // -- SQLite still interprets this as URI.
		{path: "file:/tmp/test.db", wantErr: "file: scheme"},
		{path: "file:test.db", wantErr: "file: scheme"},
		{path: "file:test.db?mode=ro", wantErr: "file: scheme"},

		// Query parameters -- trigger url.ParseQuery in driver.
		{path: "/tmp/test.db?_pragma=journal_mode(wal)", wantErr: "contains '?'"},
		{path: "test.db?cache=shared", wantErr: "contains '?'"},

		// Empty path.
		{path: "", wantErr: "must not be empty"},

		// Not a scheme: no letters before "://".
		{path: "/path/with://in/middle"},

		// Numeric prefix before :// is not a scheme.
		{path: "123://not-a-scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := ValidateDBPath(tt.path)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr,
				"ValidateDBPath(%q) = %v, want error containing %q", tt.path, err, tt.wantErr)
		})
	}
}

func TestValidateDBPathRejectsRandomURLs(t *testing.T) {
	t.Parallel()
	f := gofakeit.New(testSeed)
	for i := range 100 {
		u := f.URL()
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Error(t, ValidateDBPath(u), "ValidateDBPath(%q) should reject", u)
		})
	}
}

func TestValidateDBPathRejectsRandomURLsWithQueryParams(t *testing.T) {
	t.Parallel()
	f := gofakeit.New(testSeed)
	for i := range 50 {
		u := fmt.Sprintf("%s?%s=%s", f.URL(), f.Word(), f.Word())
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Error(t, ValidateDBPath(u), "ValidateDBPath(%q) should reject", u)
		})
	}
}

func TestExpandHome(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	t.Run("tilde slash prefix", func(t *testing.T) {
		assert.Equal(t, filepath.Join(home, "foo.pdf"), ExpandHome("~/foo.pdf"))
	})
	t.Run("nested path", func(t *testing.T) {
		assert.Equal(
			t,
			filepath.Join(home, "docs", "invoice.pdf"),
			ExpandHome("~/docs/invoice.pdf"),
		)
	})
	t.Run("bare tilde", func(t *testing.T) {
		assert.Equal(t, home, ExpandHome("~"))
	})
	t.Run("absolute path unchanged", func(t *testing.T) {
		assert.Equal(t, "/tmp/foo.pdf", ExpandHome("/tmp/foo.pdf"))
	})
	t.Run("relative path unchanged", func(t *testing.T) {
		assert.Equal(t, "foo.pdf", ExpandHome("foo.pdf"))
	})
	t.Run("empty string unchanged", func(t *testing.T) {
		assert.Empty(t, ExpandHome(""))
	})
	t.Run("tilde other user unchanged", func(t *testing.T) {
		assert.Equal(t, "~otheruser/foo", ExpandHome("~otheruser/foo"))
	})
}

func TestOpenRejectsURIs(t *testing.T) {
	t.Parallel()
	f := gofakeit.New(testSeed)
	for i := range 10 {
		u := f.URL()
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			_, err := Open(u)
			require.Error(t, err, "Open(%q) should reject URI paths", u)
		})
	}
}
