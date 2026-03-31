// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sqlfmt

import (
	"strings"
	"unicode"
)

// FormatSQL pretty-prints a SQL SELECT statement for human reading.
// It tokenizes the input, uppercases keywords, aligns clauses, indents
// continuation lines, and wraps SELECT column lists one-per-line when
// there are multiple columns. maxWidth controls soft line wrapping; pass
// 0 to disable wrapping.
func FormatSQL(sql string, maxWidth int) string {
	tokens := tokenizeSQL(sql)
	if len(tokens) == 0 {
		return sql
	}
	formatted := layoutClauses(tokens)
	if maxWidth > 0 {
		formatted = wrapLongLines(formatted, maxWidth)
	}
	return formatted
}

// wrapLongLines breaks any line exceeding maxWidth at the nearest space
// boundary, continuing with extra indentation.
func wrapLongLines(s string, maxWidth int) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		if len(line) <= maxWidth {
			result = append(result, line)
			continue
		}
		result = append(result, softWrapLine(line, maxWidth)...)
	}
	return strings.Join(result, "\n")
}

// softWrapLine breaks a single line into multiple lines at space boundaries,
// indenting continuation lines 4 spaces deeper than the original.
func softWrapLine(line string, maxWidth int) []string {
	// Measure the leading whitespace of the original line.
	trimmed := strings.TrimLeft(line, " ")
	baseIndent := len(line) - len(trimmed)
	contIndent := strings.Repeat(" ", baseIndent+4)
	// minCut ensures we don't break inside the leading indent, which would
	// produce no progress and loop forever.
	minCut := len(contIndent) + 1

	var lines []string
	remaining := line

	for len(remaining) > maxWidth {
		// Find the last space within maxWidth, but not inside the indent.
		cutAt := strings.LastIndex(remaining[:maxWidth], " ")
		if cutAt < minCut {
			// No useful break point before maxWidth -- try after.
			idx := strings.Index(remaining[maxWidth:], " ")
			if idx < 0 {
				break // truly unbreakable; emit as-is
			}
			cutAt = maxWidth + idx
		}

		lines = append(lines, strings.TrimRight(remaining[:cutAt], " "))
		remaining = contIndent + strings.TrimLeft(remaining[cutAt:], " ")
	}
	lines = append(lines, remaining)
	return lines
}

// --- tokenizer ---

type sqlTokenKind int

const (
	tokWord   sqlTokenKind = iota // identifier or keyword
	tokNumber                     // numeric literal
	tokString                     // 'quoted string'
	tokSymbol                     // operator / punctuation: (, ), *, ,, ., =, <, >, etc.
	tokSpace                      // whitespace (collapsed)
)

type sqlToken struct {
	Kind sqlTokenKind
	Text string
}

// tokenizeSQL splits a SQL string into tokens, collapsing whitespace runs
// and preserving quoted strings.
func tokenizeSQL(s string) []sqlToken {
	var tokens []sqlToken
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		// Whitespace: collapse to single space.
		if unicode.IsSpace(ch) {
			for i < len(runes) && unicode.IsSpace(runes[i]) {
				i++
			}
			tokens = append(tokens, sqlToken{Kind: tokSpace, Text: " "})
			continue
		}

		// Quoted string: consume until matching quote, handling escapes.
		if ch == '\'' {
			j := i + 1
			for j < len(runes) {
				if runes[j] == '\'' {
					if j+1 < len(runes) && runes[j+1] == '\'' {
						j += 2 // escaped quote
						continue
					}
					j++
					break
				}
				j++
			}
			tokens = append(tokens, sqlToken{Kind: tokString, Text: string(runes[i:j])})
			i = j
			continue
		}

		// Number: digits, optionally with one dot.
		if unicode.IsDigit(ch) {
			j := i
			hasDot := false
			for j < len(runes) && (unicode.IsDigit(runes[j]) || (runes[j] == '.' && !hasDot)) {
				if runes[j] == '.' {
					hasDot = true
				}
				j++
			}
			tokens = append(tokens, sqlToken{Kind: tokNumber, Text: string(runes[i:j])})
			i = j
			continue
		}

		// Word: letters, digits, underscores.
		if unicode.IsLetter(ch) || ch == '_' {
			j := i
			for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_') {
				j++
			}
			tokens = append(tokens, sqlToken{Kind: tokWord, Text: string(runes[i:j])})
			i = j
			continue
		}

		// Two-character operators.
		if i+1 < len(runes) {
			pair := string(runes[i : i+2])
			if pair == "<=" || pair == ">=" || pair == "<>" || pair == "!=" || pair == "||" {
				tokens = append(tokens, sqlToken{Kind: tokSymbol, Text: pair})
				i += 2
				continue
			}
		}

		// Single-character symbol.
		tokens = append(tokens, sqlToken{Kind: tokSymbol, Text: string(ch)})
		i++
	}

	return tokens
}

// --- clause detection ---

// clauseLevel determines how a keyword should be indented.
// 0 = top-level clause (SELECT, FROM, WHERE, ...) -- starts at column 0.
// 1 = continuation within a clause (AND, OR) -- indented.
// -1 = not a clause keyword.
func clauseLevel(kw string) int {
	switch kw {
	case "SELECT", "FROM", "WHERE",
		"ORDER BY", "GROUP BY", "HAVING",
		"LIMIT", "OFFSET",
		"UNION", "UNION ALL", "INTERSECT", "EXCEPT",
		"INSERT", "UPDATE", "DELETE", "SET", "VALUES",
		"LEFT JOIN", "RIGHT JOIN", "INNER JOIN",
		"CROSS JOIN", "FULL JOIN", "JOIN", "ON":
		return 0
	case "AND", "OR":
		return 1
	}
	return -1
}

// multiWordClauses are keyword sequences that should be treated as a single
// clause keyword. Ordered longest-first so "ORDER BY" matches before "ORDER".
var multiWordClauses = []string{
	"UNION ALL",
	"ORDER BY",
	"GROUP BY",
	"LEFT JOIN",
	"RIGHT JOIN",
	"INNER JOIN",
	"CROSS JOIN",
	"FULL JOIN",
}

// sqlKeywords is the set of SQL reserved words that get uppercased.
var sqlKeywords = map[string]bool{
	"SELECT": true, "DISTINCT": true, "FROM": true, "WHERE": true,
	"AND": true, "OR": true, "NOT": true, "IN": true, "EXISTS": true,
	"BETWEEN": true, "LIKE": true, "IS": true, "NULL": true,
	"AS": true, "ON": true, "JOIN": true,
	"LEFT": true, "RIGHT": true, "INNER": true, "CROSS": true, "FULL": true, "OUTER": true,
	"ORDER": true, "BY": true, "ASC": true, "DESC": true,
	"GROUP": true, "HAVING": true,
	"LIMIT": true, "OFFSET": true,
	"UNION": true, "ALL": true, "INTERSECT": true, "EXCEPT": true,
	"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
	"COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true,
	"COALESCE": true, "CAST": true, "IFNULL": true,
	"INSERT": true, "INTO": true, "UPDATE": true, "DELETE": true, "SET": true, "VALUES": true,
	"TRUE": true, "FALSE": true,
}

// --- layout engine ---

// clauseToken is a processed token that may represent a multi-word keyword.
type clauseToken struct {
	sqlToken

	Keyword string // uppercased multi-word keyword (e.g. "ORDER BY"), or "" if not a clause
	Level   int    // clause level from clauseLevel, or -1
}

// buildClauseTokens merges consecutive word tokens into multi-word keywords
// and tags each token with its clause level.
func buildClauseTokens(tokens []sqlToken) []clauseToken {
	// First, uppercase keywords in-place.
	for i := range tokens {
		if tokens[i].Kind == tokWord {
			up := strings.ToUpper(tokens[i].Text)
			if sqlKeywords[up] {
				tokens[i].Text = up
			}
		}
	}

	var result []clauseToken
	i := 0
	for i < len(tokens) {
		// Skip leading space.
		if tokens[i].Kind == tokSpace {
			result = append(result, clauseToken{sqlToken: tokens[i], Level: -1})
			i++
			continue
		}

		// Try multi-word keyword matching.
		if tokens[i].Kind == tokWord {
			matched := false
			for _, mw := range multiWordClauses {
				parts := strings.Fields(mw)
				if matchesWordSequence(tokens, i, parts) {
					// Consume the tokens that form this multi-word keyword.
					ct := clauseToken{
						sqlToken: sqlToken{Kind: tokWord, Text: mw},
						Keyword:  mw,
						Level:    clauseLevel(mw),
					}
					result = append(result, ct)
					// Advance past the matched tokens + intervening spaces.
					i = advancePastSequence(tokens, i, len(parts))
					matched = true
					break
				}
			}
			if matched {
				continue
			}

			// Single keyword check.
			up := strings.ToUpper(tokens[i].Text)
			lvl := clauseLevel(up)
			result = append(result, clauseToken{
				sqlToken: tokens[i],
				Keyword:  up,
				Level:    lvl,
			})
			i++
			continue
		}

		result = append(result, clauseToken{sqlToken: tokens[i], Level: -1})
		i++
	}
	return result
}

// matchesWordSequence checks if tokens starting at pos match the given
// word parts (ignoring intervening spaces).
func matchesWordSequence(tokens []sqlToken, pos int, parts []string) bool {
	ti := pos
	for _, part := range parts {
		// Skip spaces.
		for ti < len(tokens) && tokens[ti].Kind == tokSpace {
			ti++
		}
		if ti >= len(tokens) || tokens[ti].Kind != tokWord {
			return false
		}
		if strings.ToUpper(tokens[ti].Text) != part {
			return false
		}
		ti++
	}
	return true
}

// advancePastSequence returns the index after consuming n words and their
// intervening spaces starting from pos.
func advancePastSequence(tokens []sqlToken, pos, wordCount int) int {
	found := 0
	i := pos
	for i < len(tokens) && found < wordCount {
		if tokens[i].Kind == tokSpace {
			i++
			continue
		}
		found++
		i++
	}
	return i
}

// trimTrailingSpace removes a single trailing space from the builder if
// present. Used before inserting a newline so lines don't end with whitespace.
func trimTrailingSpace(b *strings.Builder) {
	n := b.Len()
	if n > 0 {
		s := b.String()
		if s[n-1] == ' ' {
			b.Reset()
			b.WriteString(s[:n-1])
		}
	}
}

// layoutClauses formats the token stream into indented, line-broken SQL.
// Handles nested queries by tracking parenthesis depth and indenting
// subqueries appropriately.
func layoutClauses(rawTokens []sqlToken) string {
	tokens := buildClauseTokens(rawTokens)

	var b strings.Builder
	const indentUnit = "  "
	atLineStart := true
	inSelect := false
	parenDepth := 0
	baseIndent := 0 // Indentation level for current scope

	for i, ct := range tokens {
		// Track parenthesis depth to detect subqueries.
		if ct.Kind == tokSymbol && ct.Text == "(" {
			// Check if this is the start of a subquery (preceded by FROM, JOIN, IN, etc.)
			// For simplicity, we treat all ( as potential subquery starts.
			b.WriteString(ct.Text)
			parenDepth++

			// Peek ahead to see if next non-space token is SELECT (indicating subquery)
			nextIdx := i + 1
			for nextIdx < len(tokens) && tokens[nextIdx].Kind == tokSpace {
				nextIdx++
			}
			if nextIdx < len(tokens) && tokens[nextIdx].Keyword == "SELECT" {
				baseIndent++
			}
			atLineStart = false
			continue
		}

		if ct.Kind == tokSymbol && ct.Text == ")" {
			if parenDepth > 0 {
				parenDepth--
				// Check if we're closing a subquery scope
				if baseIndent > 0 {
					// Look back to see if we had a SELECT at this level
					baseIndent--
				}
			}
			b.WriteString(ct.Text)
			atLineStart = false
			continue
		}

		// Clause keyword at top level of current scope: start a new line.
		if ct.Level >= 0 && parenDepth == 0 {
			kw := ct.Keyword

			// SELECT: new line with proper indentation
			if kw == "SELECT" {
				if b.Len() > 0 {
					trimTrailingSpace(&b)
					b.WriteString("\n")
					b.WriteString(strings.Repeat(indentUnit, baseIndent))
				}
				b.WriteString(ct.Text)
				atLineStart = false
				inSelect = true
				continue
			}

			// AND/OR get one extra indent from their clause.
			if ct.Level == 1 {
				trimTrailingSpace(&b)
				b.WriteString("\n")
				b.WriteString(strings.Repeat(indentUnit, baseIndent+1))
				b.WriteString(ct.Text)
				atLineStart = false
				continue
			}

			// Other top-level clauses: newline with base indentation.
			trimTrailingSpace(&b)
			b.WriteString("\n")
			b.WriteString(strings.Repeat(indentUnit, baseIndent))
			b.WriteString(ct.Text)
			atLineStart = false
			inSelect = false
			continue
		}

		// In a SELECT column list, break on commas at current paren depth.
		// Each column gets its own line with proper indentation.
		if inSelect && ct.Kind == tokSymbol && ct.Text == "," && parenDepth == 0 {
			b.WriteString(",")
			b.WriteString("\n")
			b.WriteString(strings.Repeat(indentUnit, baseIndent+1))
			atLineStart = true
			continue
		}

		// Skip spaces at line start (we handle indentation ourselves).
		if ct.Kind == tokSpace && atLineStart {
			continue
		}

		b.WriteString(ct.Text)
		atLineStart = false
	}

	return strings.TrimSpace(b.String())
}
