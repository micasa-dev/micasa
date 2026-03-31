// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- gormColumnName ---

func TestGormColumnName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{"empty", "", ""},
		{"no column", "primaryKey", ""},
		{"column only", "column:sha256", "sha256"},
		{"column with other keys", "column:ocr_data;type:blob", "ocr_data"},
		{"other key before column", "index;column:file_name", "file_name"},
		{"index only", "index:idx_doc_entity", ""},
		{"constraint", "constraint:OnDelete:RESTRICT;", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, gormColumnName(tt.tag))
		})
	}
}

// --- isAssociation ---

func TestIsAssociation(t *testing.T) {
	t.Parallel()
	localStructs := map[string]bool{"Vendor": true, "Project": true}

	tests := []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{
			"local struct",
			&ast.Ident{Name: "Vendor"},
			true,
		},
		{
			"pointer to local struct",
			&ast.StarExpr{X: &ast.Ident{Name: "Project"}},
			true,
		},
		{
			"basic type",
			&ast.Ident{Name: "string"},
			false,
		},
		{
			"pointer to basic type",
			&ast.StarExpr{X: &ast.Ident{Name: "int64"}},
			false,
		},
		{
			"pointer to uint",
			&ast.StarExpr{X: &ast.Ident{Name: "uint"}},
			false,
		},
		{
			"selector expr (gorm.DeletedAt)",
			&ast.SelectorExpr{
				X:   &ast.Ident{Name: "gorm"},
				Sel: &ast.Ident{Name: "DeletedAt"},
			},
			false,
		},
		{
			"selector expr (time.Time)",
			&ast.SelectorExpr{
				X:   &ast.Ident{Name: "time"},
				Sel: &ast.Ident{Name: "Time"},
			},
			false,
		},
		{
			"pointer to selector expr (*time.Time)",
			&ast.StarExpr{X: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "time"},
				Sel: &ast.Ident{Name: "Time"},
			}},
			false,
		},
		{
			"array type ([]byte)",
			&ast.ArrayType{Elt: &ast.Ident{Name: "byte"}},
			false,
		},
		{
			"unknown struct not in set",
			&ast.Ident{Name: "UnknownType"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isAssociation(tt.expr, localStructs))
		})
	}
}

// --- fieldGormTag ---

func TestFieldGormTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  *ast.BasicLit
		want string
	}{
		{"nil tag", nil, ""},
		{"gorm tag", &ast.BasicLit{Value: "`gorm:\"primaryKey\"`"}, "primaryKey"},
		{"column tag", &ast.BasicLit{Value: "`gorm:\"column:sha256\"`"}, "column:sha256"},
		{"json only", &ast.BasicLit{Value: "`json:\"name\"`"}, ""},
		{
			"mixed tags",
			&ast.BasicLit{Value: "`json:\"name\" gorm:\"uniqueIndex\"`"},
			"uniqueIndex",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			field := &ast.Field{Tag: tt.tag}
			assert.Equal(t, tt.want, fieldGormTag(field))
		})
	}
}

// --- collectStructs ---

func TestCollectStructs(t *testing.T) {
	t.Parallel()
	src := `package example

type Exported struct {
	Name string
}

type unexported struct {
	Value int
}

type AlsoExported struct {
	ID uint
}

type NotAStruct = string
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	structs, names := collectStructs(f)

	assert.Len(t, structs, 2)
	assert.Equal(t, "Exported", structs[0].name)
	assert.Equal(t, "AlsoExported", structs[1].name)

	assert.True(t, names["Exported"])
	assert.True(t, names["AlsoExported"])
	assert.False(t, names["unexported"])
	assert.False(t, names["NotAStruct"])
}

func TestCollectStructsEmpty(t *testing.T) {
	t.Parallel()
	src := `package example

var x = 1
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	structs, names := collectStructs(f)
	assert.Empty(t, structs)
	assert.Empty(t, names)
}

// --- writeConstBlock ---

func TestWriteConstBlock(t *testing.T) {
	t.Parallel()
	consts := map[string]string{
		"TableVendors":    "vendors",
		"TableProjects":   "projects",
		"TableAppliances": "appliances",
	}

	var buf bytes.Buffer
	writeConstBlock(&buf, "// Tables.", consts)

	out := buf.String()
	assert.Contains(t, out, "// Tables.")
	assert.Contains(t, out, `TableAppliances = "appliances"`)
	assert.Contains(t, out, `TableProjects = "projects"`)
	assert.Contains(t, out, `TableVendors = "vendors"`)

	// Verify sorted order: Appliances < Projects < Vendors.
	aIdx := strings.Index(out, "TableAppliances")
	pIdx := strings.Index(out, "TableProjects")
	vIdx := strings.Index(out, "TableVendors")
	assert.Less(t, aIdx, pIdx)
	assert.Less(t, pIdx, vIdx)
}

func TestWriteConstBlockEmpty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	writeConstBlock(&buf, "// Empty.", map[string]string{})
	assert.Contains(t, buf.String(), "// Empty.\nconst (\n)\n")
}
