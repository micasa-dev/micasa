// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/micasa-dev/micasa/internal/sqlfmt"
)

// refreshChatViewport rebuilds the viewport content from the message history.
func (m *Model) refreshChatViewport() {
	if m.chat == nil {
		return
	}
	content := m.renderChatMessages()
	m.chat.Viewport.SetContent(content)
	m.chat.Viewport.GotoBottom()
}

// renderChatMessages formats the conversation for display in the viewport.
func (m *Model) renderChatMessages() string {
	if m.chat == nil {
		return ""
	}

	innerW := m.chatViewportWidth()
	var parts []string
	for i, msg := range m.chat.Messages {
		var rendered string
		switch msg.Role {
		case roleUser:
			label := m.styles.ChatUser().Render("›")
			// Inline compact: label then content on same line.
			textW := innerW - lipgloss.Width(label) - 1
			text := wordWrap(msg.Content, textW)
			rendered = label + " " + text
		case roleAssistant:
			label := m.styles.ChatAssistant().Render(m.llmModelLabel())
			text := msg.Content
			sql := msg.SQL
			isLastMessage := i == len(m.chat.Messages)-1

			var parts []string

			// Show SQL if toggle is on and SQL exists.
			if m.chat.ShowSQL && sql != "" {
				sqlWidth := max(innerW-8, 30)
				sqlBlock := m.chat.renderMarkdown(
					"```sql\n"+sqlfmt.FormatSQL(sql, sqlWidth)+"\n```",
					innerW-2,
				)
				parts = append(parts, sqlBlock)
			}

			// Show response if available.
			if text != "" {
				display := text
				if m.magMode {
					display = magTransformText(display, m.cur.Symbol())
				}
				parts = append(parts, m.chat.renderMarkdown(display, innerW-2))
			}

			// Join content parts, trimming glamour's leading whitespace.
			body := strings.TrimLeft(strings.Join(parts, "\n"), "\n")

			// Determine what to show on the label line.
			// Only show spinner for the currently streaming message (last one).
			if isLastMessage && m.chat.StreamingSQL && sql == "" {
				// Stage 1: generating SQL query
				rendered = label + "  " + m.chat.Spinner.View() + " " + m.styles.HeaderHint().
					Render(
						"generating query",
					)
			} else if isLastMessage && text == "" && m.chat.Streaming && !m.chat.StreamingSQL {
				// Stage 2: thinking about response (may have SQL already)
				labelLine := label + "  " + m.chat.Spinner.View() + " " + m.styles.HeaderHint().Render("thinking")
				if body != "" {
					rendered = labelLine + "\n" + body
				} else {
					rendered = labelLine
				}
			} else if body != "" {
				rendered = label + "\n" + body
			} else {
				rendered = label
			}

			// Add subtle separator after assistant response (end of Q&A pair).
			// Skip if it's the last message to avoid trailing separator.
			if i < len(m.chat.Messages)-1 && text != "" {
				sep := strings.Repeat("─", innerW)
				rendered += "\n" + m.styles.TextDim().Render(sep)
			}
		case roleError:
			rendered = m.styles.Error().Render("error: " + wordWrap(msg.Content, innerW-9))
		case roleNotice:
			// Skip "generating query" notice - status is shown inline with model label.
			if msg.Content == "generating query" {
				continue
			}
			if msg.Content == "Interrupted" || msg.Content == "Pull cancelled" {
				rendered = m.styles.ChatInterrupted().Render(msg.Content)
			} else {
				rendered = m.styles.ChatNotice().Render(msg.Content)
			}
		}
		parts = append(parts, rendered)
	}
	return strings.Join(parts, "\n")
}

func (m *Model) llmModelLabel() string {
	if m.llmClient != nil {
		return m.llmClient.Model()
	}
	return "LLM"
}

// --- Chat overlay rendering ---

func (m *Model) buildChatOverlay() string {
	if m.chat == nil {
		return ""
	}

	contentW := m.chatOverlayWidth()
	innerW := contentW - 4

	titleText := " Ask "
	if m.llmClient != nil {
		titleText = " " + m.llmClient.Model() + " "
	}
	title := m.styles.HeaderSection().Render(titleText)

	vpH := m.chatViewportHeight()
	m.chat.Viewport.SetWidth(innerW)
	m.chat.Viewport.SetHeight(vpH)
	vpView := m.chat.Viewport.View()

	m.chat.Input.SetWidth(innerW - 2)
	inputView := m.chat.Input.View()

	// Model completer list (between input and viewport).
	completerView := m.renderModelCompleter(innerW)

	var hintParts []string
	if m.chat.Completer != nil {
		hintParts = append(hintParts,
			m.helpItem(keyUp+"/"+keyDown, "navigate"),
			m.helpItem(symReturn, "select"),
			m.helpItem(keyEsc, "dismiss"),
		)
	} else {
		hintParts = append(hintParts,
			m.helpItem(symReturn, "send"),
			m.sqlHintItem(),
			m.helpItem(symUp+"/"+symDown, "history"),
			m.helpItem(keyEsc, "hide"),
		)
	}
	hints := joinWithSeparator(m.helpSeparator(), hintParts...)

	// Build layout: title, viewport, [completer], input, hints.
	sections := []string{title, "", vpView, ""}
	if completerView != "" {
		sections = append(sections, completerView, "")
	}
	sections = append(sections, inputView, "", hints)

	boxContent := lipgloss.JoinVertical(lipgloss.Left, sections...)

	maxH := max(m.effectiveHeight()*3/5, 12)

	return m.styles.OverlayBox().
		Width(contentW).
		MaxHeight(maxH).
		Render(boxContent)
}

// renderModelCompleter renders the inline model completion list with a
// fixed height of completerMaxLines so the overlay doesn't shift.
func (m *Model) renderModelCompleter(innerW int) string {
	query, _ := m.completerQuery()
	return m.renderModelCompleterFor(m.chat.Completer, query, innerW)
}

// renderModelCompleterFor renders the model completion list for any completer.
func (m *Model) renderModelCompleterFor(mc *modelCompleter, query string, innerW int) string {
	if mc == nil {
		return ""
	}

	lines := make([]string, completerMaxLines)

	if mc.Loading {
		lines[0] = m.styles.HeaderHint().Render("  loading models" + symEllipsis)
		for i := 1; i < completerMaxLines; i++ {
			lines[i] = ""
		}
		return strings.Join(lines, "\n")
	}

	if len(mc.Matches) == 0 {
		if query != "" {
			lines[0] = m.styles.Empty().Render("  no matching models")
		} else {
			lines[0] = m.styles.Empty().Render("  no models available")
		}
		for i := 1; i < completerMaxLines; i++ {
			lines[i] = ""
		}
		return strings.Join(lines, "\n")
	}

	maxVisible := min(completerMaxLines, len(mc.Matches))
	start := max(mc.Cursor-maxVisible/2, 0)
	end := start + maxVisible
	if end > len(mc.Matches) {
		end = len(mc.Matches)
		start = max(end-maxVisible, 0)
	}

	pointer := m.styles.AccentBold()
	lineIdx := 0
	for i := start; i < end; i++ {
		entry := mc.Matches[i]
		selected := i == mc.Cursor

		label := m.highlightModelMatch(entry)

		var line string
		if selected {
			line = pointer.Render("▸ ") + label
		} else {
			line = "  " + label
		}

		if lipgloss.Width(line) > innerW {
			line = m.styles.Base().MaxWidth(innerW).Render(line)
		}
		lines[lineIdx] = line
		lineIdx++
	}
	// Pad remaining lines to maintain fixed height.
	for i := lineIdx; i < completerMaxLines; i++ {
		lines[i] = ""
	}

	return strings.Join(lines, "\n")
}

// highlightModelMatch renders a model name styled by its state:
//   - Active:                accent (sky blue) + bold
//   - Local but inactive:   bright text (clearly available)
//   - Not downloaded:        dim text + italic (needs fetching)
//
// Fuzzy-matched characters get the accent highlight regardless.
func (m *Model) highlightModelMatch(match modelCompleterMatch) string {
	var baseStyle, highlightStyle lipgloss.Style
	switch {
	case match.Active:
		baseStyle = m.styles.ModelActive()
		highlightStyle = m.styles.ModelActiveHL()
	case match.Local:
		baseStyle = m.styles.ModelLocal()
		highlightStyle = m.styles.ModelLocalHL()
	default:
		baseStyle = m.styles.ModelRemote()
		highlightStyle = m.styles.ModelRemoteHL()
	}
	return highlightFuzzyPositions(match.Name, match.Positions, baseStyle, highlightStyle)
}

// --- Layout helpers ---

func (m *Model) chatOverlayWidth() int {
	w := min(m.effectiveWidth()-8, 90)
	w = max(w, 40)
	return w
}

func (m *Model) chatViewportWidth() int {
	return m.chatOverlayWidth() - 8
}

func (m *Model) chatViewportHeight() int {
	maxOverlay := m.effectiveHeight() * 3 / 5
	// Chrome: border(2) + padding(2) + title(1) + blanks(3)
	// + input(1) + blank(1) + hints(1) = 11 lines.
	chrome := 11
	if m.chat != nil && m.chat.Completer != nil {
		// Reserve space for the completer + its surrounding blank line.
		chrome += completerMaxLines + 1
	}
	h := max(maxOverlay-chrome, 4)
	return h
}

func (m *Model) chatInputWidth() int {
	return m.chatOverlayWidth() - 10
}
