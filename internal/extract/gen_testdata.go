// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

//go:build ignore

// gen_testdata.go creates a minimal PDF test fixture with extractable text.
// Run from repo root: go run internal/extract/gen_testdata.go
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	// Minimal valid PDF with one page containing "Invoice #1234" text.
	// Hand-crafted to avoid external dependencies.
	var b strings.Builder

	// Header
	b.WriteString("%PDF-1.4\n")

	// Object 1: Catalog
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	off1 := b.Len()
	b.WriteString(obj1)

	// Object 2: Pages
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	off2 := b.Len()
	b.WriteString(obj2)

	// Object 4: Font
	obj4 := "4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n"
	off4 := b.Len()
	b.WriteString(obj4)

	// Stream content: draw text
	stream := "BT\n/F1 12 Tf\n72 720 Td\n(Invoice #1234) Tj\n0 -20 Td\n(Date: 2025-01-15) Tj\n0 -20 Td\n(Vendor: Garcia Plumbing) Tj\n0 -20 Td\n(Total: $1,500.00) Tj\nET\n"

	// Object 5: Stream
	obj5 := fmt.Sprintf(
		"5 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n",
		len(stream),
		stream,
	)
	off5 := b.Len()
	b.WriteString(obj5)

	// Object 3: Page
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 5 0 R /Resources << /Font << /F1 4 0 R >> >> >>\nendobj\n"
	off3 := b.Len()
	b.WriteString(obj3)

	// Cross-reference table
	xrefOff := b.Len()
	b.WriteString("xref\n")
	b.WriteString("0 6\n")
	b.WriteString("0000000000 65535 f \n")
	b.WriteString(fmt.Sprintf("%010d 00000 n \n", off1))
	b.WriteString(fmt.Sprintf("%010d 00000 n \n", off2))
	b.WriteString(fmt.Sprintf("%010d 00000 n \n", off3))
	b.WriteString(fmt.Sprintf("%010d 00000 n \n", off4))
	b.WriteString(fmt.Sprintf("%010d 00000 n \n", off5))

	// Trailer
	b.WriteString("trailer\n")
	b.WriteString("<< /Size 6 /Root 1 0 R >>\n")
	b.WriteString("startxref\n")
	b.WriteString(fmt.Sprintf("%d\n", xrefOff))
	b.WriteString("%%EOF\n")

	if err := os.WriteFile("internal/extract/testdata/sample.pdf", []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote internal/extract/testdata/sample.pdf")
}
