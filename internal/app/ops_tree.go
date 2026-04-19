// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/micasa-dev/micasa/internal/extract"
)

// Zone ID prefix for clickable tree nodes (expand/collapse toggle).
const zoneOpsNode = "ops-node-"

// Zone ID prefix for clickable table preview tabs.
const zoneOpsTab = "ops-tab-"

// Box-drawing characters for tree rendering (eza --tree style).
const (
	treeBranch = "├─ "
	treeCorner = "└─ "
	treePipe   = "│  "
	treeBlank  = "   "
)

// treeValueKind classifies a JSON value for per-type coloring.
type treeValueKind int

const (
	tvString treeValueKind = iota
	tvNumber
	tvBool
	tvNull
	tvOther
)

// jsonTreeNode represents a single node in a JSON tree.
type jsonTreeNode struct {
	path       string          // unique path for expand tracking
	key        string          // display key ("operations", "[0]", "action", etc.)
	value      string          // formatted leaf value; empty for containers
	valueKind  treeValueKind   // type for coloring
	children   []*jsonTreeNode // non-nil for object/array containers
	depth      int             // nesting level for indentation
	isArray    bool            // true if this node represents a JSON array
	treePrefix string          // precomputed box-drawing prefix ("│  ├─ ")
}

func (n *jsonTreeNode) isExpandable() bool {
	return len(n.children) > 0
}

// opsTreeState holds the state for the interactive JSON tree overlay.
type opsTreeState struct {
	root          []*jsonTreeNode     // single-element: the "operations" wrapper
	cursor        int                 // index into visibleNodes()
	expanded      map[string]bool     // keyed by node path
	docTitle      string              // document title shown in overlay header
	maxNodes      int                 // total node count when fully expanded (for stable viewport)
	previewGroups []previewTableGroup // grouped extraction ops for table preview
	previewTab    int                 // active tab index into previewGroups
}

// visibleNodes returns the flattened list of currently visible tree nodes,
// respecting the expanded state of each container.
func (s *opsTreeState) visibleNodes() []*jsonTreeNode {
	var nodes []*jsonTreeNode
	var walk func([]*jsonTreeNode)
	walk = func(children []*jsonTreeNode) {
		for _, n := range children {
			nodes = append(nodes, n)
			if n.isExpandable() && s.expanded[n.path] {
				walk(n.children)
			}
		}
	}
	walk(s.root)
	return nodes
}

// clampCursor ensures cursor stays within the visible node range.
func (s *opsTreeState) clampCursor() {
	n := len(s.visibleNodes())
	if n == 0 {
		s.cursor = 0
		return
	}
	if s.cursor >= n {
		s.cursor = n - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}

// buildJSONTree parses raw JSON bytes into a tree rooted at an "operations" node.
func buildJSONTree(raw []byte) []*jsonTreeNode {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var parsed any
	if err := d.Decode(&parsed); err != nil {
		return nil
	}

	arr, ok := parsed.([]any)
	if !ok {
		arr = []any{parsed}
	}
	if len(arr) == 0 {
		return nil
	}

	// Wrap in an "operations" root node (no tree prefix for the root).
	root := &jsonTreeNode{
		path:    "operations",
		key:     "operations",
		depth:   0,
		isArray: true,
	}
	for i, elem := range arr {
		path := fmt.Sprintf("operations.%d", i)
		key := fmt.Sprintf("[%d]", i)
		isLast := i == len(arr)-1
		root.children = append(root.children, buildNode(path, key, elem, 1, nil, isLast))
	}

	return []*jsonTreeNode{root}
}

// buildNode recursively creates a tree node from a JSON value.
// guides tracks whether ancestors have more siblings (for drawing │ vs space).
func buildNode(path, key string, v any, depth int, guides []bool, isLast bool) *jsonTreeNode {
	node := &jsonTreeNode{
		path:       path,
		key:        key,
		depth:      depth,
		treePrefix: buildTreePrefix(guides, isLast),
	}
	childGuides := append(append([]bool{}, guides...), !isLast)
	switch val := v.(type) {
	case map[string]any:
		keys := slices.SortedFunc(maps.Keys(val), opsKeyCmp)
		for i, k := range keys {
			childPath := path + "." + k
			node.children = append(node.children, buildNode(childPath, k, val[k], depth+1, childGuides, i == len(keys)-1))
		}
	case []any:
		node.isArray = true
		for i, elem := range val {
			childPath := fmt.Sprintf("%s.%d", path, i)
			childKey := fmt.Sprintf("[%d]", i)
			node.children = append(node.children, buildNode(childPath, childKey, elem, depth+1, childGuides, i == len(val)-1))
		}
	default:
		node.value, node.valueKind = classifyValue(val)
	}
	return node
}

// buildTreePrefix converts guide state into the box-drawing prefix string.
func buildTreePrefix(guides []bool, isLast bool) string {
	var b strings.Builder
	for _, hasSibling := range guides {
		if hasSibling {
			b.WriteString(treePipe)
		} else {
			b.WriteString(treeBlank)
		}
	}
	if isLast {
		b.WriteString(treeCorner)
	} else {
		b.WriteString(treeBranch)
	}
	return b.String()
}

// opsKeyCmp sorts object keys with "action" and "table" first, then
// alphabetically. This keeps the operation identity fields together.
func opsKeyCmp(a, b string) int {
	pa, pb := opsKeyPriority(a), opsKeyPriority(b)
	if pa != pb {
		return cmp.Compare(pa, pb)
	}
	return cmp.Compare(a, b)
}

func opsKeyPriority(key string) int {
	switch key {
	case "action":
		return 0
	case "table":
		return 1
	default:
		return 2
	}
}

// countAllNodes returns the total number of nodes in the tree, fully expanded.
func countAllNodes(nodes []*jsonTreeNode) int {
	count := 0
	for _, n := range nodes {
		count++
		count += countAllNodes(n.children)
	}
	return count
}

// autoExpand marks all container nodes up to maxDepth as expanded.
func autoExpand(nodes []*jsonTreeNode, expanded map[string]bool, maxDepth int) {
	var walk func([]*jsonTreeNode)
	walk = func(children []*jsonTreeNode) {
		for _, n := range children {
			if n.isExpandable() && n.depth <= maxDepth {
				expanded[n.path] = true
				walk(n.children)
			}
		}
	}
	walk(nodes)
}

// parentPath returns the path of the parent node by trimming the last segment.
func parentPath(p string) string {
	i := strings.LastIndex(p, ".")
	if i < 0 {
		return ""
	}
	return p[:i]
}

// collapsedPreview builds a one-line summary for a collapsed container node.
// Objects render as {k: v, k: v, ...}, arrays as [v, v, ...].
func collapsedPreview(node *jsonTreeNode, maxW int) string {
	if len(node.children) == 0 {
		return ""
	}
	open, cls := "{", "}"
	if node.isArray {
		open, cls = "[", "]"
	}
	var b strings.Builder
	b.WriteString(open)
	for i, child := range node.children {
		if i > 0 {
			b.WriteString(", ")
		}
		var entry string
		if node.isArray {
			entry = childPreviewValue(child)
		} else {
			entry = child.key + ": " + childPreviewValue(child)
		}
		if ansi.StringWidth(b.String())+ansi.StringWidth(entry)+ansi.StringWidth(cls) > maxW {
			b.WriteString(symEllipsis)
			break
		}
		b.WriteString(entry)
	}
	b.WriteString(cls)
	return b.String()
}

// childPreviewValue returns a brief string for a node in a collapsed preview.
func childPreviewValue(n *jsonTreeNode) string {
	if n.isExpandable() {
		if n.isArray {
			return "[" + symEllipsis + "]"
		}
		return "{" + symEllipsis + "}"
	}
	return n.value
}

// openOpsTree opens the ops tree overlay for the selected document row.
func (m *Model) openOpsTree() {
	tab := m.effectiveTab()
	if tab == nil {
		return
	}
	cursor := tab.Table.Cursor()
	if cursor < 0 || cursor >= len(tab.CellRows) {
		return
	}
	row := tab.CellRows[cursor]
	opsCol := int(documentColOps)
	if opsCol >= len(row) {
		return
	}
	c := row[opsCol]
	if c.Value == "" || c.Value == "0" {
		return
	}

	meta, ok := m.selectedRowMeta()
	if !ok {
		return
	}

	doc, err := m.store.GetDocument(meta.ID)
	if err != nil || len(doc.ExtractionOps) == 0 {
		return
	}

	root := buildJSONTree(doc.ExtractionOps)
	if len(root) == 0 {
		return
	}

	expanded := make(map[string]bool)
	// Always expand "operations" root; auto-expand children for small trees.
	expanded["operations"] = true
	if len(root[0].children) <= 3 {
		autoExpand(root, expanded, 2)
	}

	m.opsTree = &opsTreeState{
		root:     root,
		expanded: expanded,
		docTitle: doc.Title,
		maxNodes: countAllNodes(root),
	}

	var ops []extract.Operation
	if err := json.Unmarshal(doc.ExtractionOps, &ops); err == nil && len(ops) > 0 {
		m.opsTree.previewGroups = groupOperationsByTable(ops, m.cur)
	}

	// Account for table preview section height in maxNodes so the overlay
	// doesn't jump when collapsing tree nodes.
	if groups := m.opsTree.previewGroups; len(groups) > 0 {
		maxRows := 0
		for _, g := range groups {
			if len(g.cells) > maxRows {
				maxRows = len(g.cells)
			}
		}
		// divider(1) + header(1) + table-divider(1) + rows
		extra := 3 + maxRows
		if len(groups) > 1 {
			// tab-bar(1) + underline(1)
			extra += 2
		}
		m.opsTree.maxNodes += extra
	}
}

// handleOpsTreeKey handles key events for the ops tree overlay.
func (m *Model) handleOpsTreeKey(msg tea.KeyPressMsg) tea.Cmd {
	tree := m.opsTree
	if tree == nil {
		return nil
	}

	switch {
	case key.Matches(msg, m.keys.OpsClose):
		m.opsTree = nil
	case key.Matches(msg, m.keys.OpsDown):
		tree.cursor++
		tree.clampCursor()
	case key.Matches(msg, m.keys.OpsUp):
		tree.cursor--
		tree.clampCursor()
	case key.Matches(msg, m.keys.OpsExpand):
		nodes := tree.visibleNodes()
		if tree.cursor >= 0 && tree.cursor < len(nodes) {
			node := nodes[tree.cursor]
			if node.isExpandable() {
				tree.expanded[node.path] = !tree.expanded[node.path]
				tree.clampCursor()
			}
		}
	case key.Matches(msg, m.keys.OpsCollapse):
		nodes := tree.visibleNodes()
		if tree.cursor >= 0 && tree.cursor < len(nodes) {
			node := nodes[tree.cursor]
			if node.isExpandable() && tree.expanded[node.path] {
				tree.expanded[node.path] = false
				tree.clampCursor()
			} else {
				pp := parentPath(node.path)
				if pp != "" {
					tree.expanded[pp] = false
					for i := tree.cursor - 1; i >= 0; i-- {
						if nodes[i].path == pp {
							tree.cursor = i
							break
						}
					}
					tree.clampCursor()
				}
			}
		}
	case key.Matches(msg, m.keys.OpsTabPrev):
		if len(tree.previewGroups) > 1 && tree.previewTab > 0 {
			tree.previewTab--
		}
	case key.Matches(msg, m.keys.OpsTabNext):
		if len(tree.previewGroups) > 1 && tree.previewTab < len(tree.previewGroups)-1 {
			tree.previewTab++
		}
	case key.Matches(msg, m.keys.OpsTop):
		tree.cursor = 0
	case key.Matches(msg, m.keys.OpsBottom):
		nodes := tree.visibleNodes()
		if len(nodes) > 0 {
			tree.cursor = len(nodes) - 1
		}
	}
	return nil
}

// buildOpsTreeOverlay renders the ops tree overlay.
func (m *Model) buildOpsTreeOverlay() string {
	tree := m.opsTree
	frameW := m.styles.OverlayBox().GetHorizontalFrameSize()

	// Build hint bar early so we can measure it for width calculation.
	hintParts := []string{
		m.helpItem(keyJ+"/"+keyK, "nav"),
		m.helpItem(symReturn, "toggle"),
		m.helpItem(keyH, "collapse"),
	}
	if len(tree.previewGroups) > 1 {
		hintParts = append(hintParts, m.helpItem(keyB+"/"+keyF, "tabs"))
	}
	hintParts = append(hintParts, m.helpItem(keyEsc, "close"))
	hints := joinWithSeparator(m.helpSeparator(), hintParts...)

	// Compute content width: widen to fit hint bar and preview tables.
	contentW := m.overlayContentWidth()
	screenW := m.effectiveWidth() - 8
	if needed := ansi.StringWidth(hints) + frameW; needed > contentW {
		contentW = needed
	}
	if groups := tree.previewGroups; len(groups) > 0 {
		sep := m.styles.TableSeparator().Render(" " + symVLine + " ")
		sepW := lipgloss.Width(sep)
		if needed := previewNaturalWidth(groups, sepW, m.cur.Symbol()) + frameW; needed > contentW {
			contentW = needed
		}
	}
	if contentW > screenW {
		contentW = screenW
	}
	innerW := contentW - frameW

	var b strings.Builder

	title := "Ops"
	if tree.docTitle != "" {
		candidate := tree.docTitle + " " + symMiddleDot + " Ops"
		if len(candidate) <= innerW-2 {
			title = candidate
		}
	}
	b.WriteString(m.styles.HeaderSection().Render(" " + title + " "))
	b.WriteString("\n\n")

	nodes := tree.visibleNodes()
	for i, node := range nodes {
		isCursor := i == tree.cursor
		line := m.renderTreeNode(node, innerW, isCursor)
		zoneID := fmt.Sprintf("%s%d", zoneOpsNode, i)
		b.WriteString(m.zones.Mark(zoneID, line))
		b.WriteString("\n")
	}
	// Pad to full expanded height so the overlay doesn't jump on collapse.
	for i := len(nodes); i < tree.maxNodes; i++ {
		b.WriteString("\n")
	}

	// Table preview section.
	if groups := tree.previewGroups; len(groups) > 0 {
		// Divider.
		b.WriteString(appStyles.TextDim().Render(strings.Repeat(symHLine, innerW)))
		b.WriteString("\n")

		// Tab bar (only if multiple groups).
		if len(groups) > 1 {
			tabParts := make([]string, 0, len(groups)*2)
			for i, g := range groups {
				var rendered string
				if i == tree.previewTab {
					rendered = m.styles.TabActive().Render(g.name)
				} else {
					rendered = m.styles.TabInactive().Render(g.name)
				}
				tabParts = append(tabParts, m.zones.Mark(
					fmt.Sprintf("%s%d", zoneOpsTab, i), rendered,
				))
				if i < len(groups)-1 {
					tabParts = append(tabParts, "   ")
				}
			}
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, tabParts...))
			b.WriteString("\n")
			b.WriteString(m.styles.TabUnderline().Render(
				strings.Repeat(symHLineHeavy, innerW),
			))
			b.WriteString("\n")
		}

		// Active table (non-interactive, dimmed).
		tabIdx := tree.previewTab
		if tabIdx >= len(groups) {
			tabIdx = 0
		}
		g := groups[tabIdx]
		sep := m.styles.TableSeparator().Render(" " + symVLine + " ")
		divSep := m.styles.TableSeparator().Render(symHLine + symCross + symHLine)
		sepW := lipgloss.Width(sep)
		tableStr := m.renderPreviewTable(g, innerW, sepW, sep, divSep, false)
		b.WriteString(appStyles.TextDim().Render(tableStr))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(hints)

	return m.styles.OverlayBox().
		Width(contentW).
		MaxHeight(m.overlayMaxHeight()).
		Render(b.String())
}

// renderTreeNode renders a single tree node line.
func (m *Model) renderTreeNode(node *jsonTreeNode, width int, isCursor bool) string {
	if node.isExpandable() {
		return m.renderContainerNode(node, width, isCursor)
	}
	return m.renderLeafNode(node, width, isCursor)
}

// renderContainerNode renders an expandable object/array node.
// The root "operations" node has no tree prefix; children use box-drawing lines.
func (m *Model) renderContainerNode(node *jsonTreeNode, width int, isCursor bool) string {
	expanded := m.opsTree.expanded[node.path]

	arrow := "+"
	if expanded {
		arrow = "-"
	}

	prefixStyle := m.styles.TextDim()
	arrowStyle := m.styles.AccentText()
	keyStyle := m.styles.Header()
	if isCursor {
		arrowStyle = m.styles.AccentBold()
		keyStyle = m.styles.AccentBold()
	}

	var b strings.Builder
	if node.treePrefix != "" {
		b.WriteString(prefixStyle.Render(node.treePrefix))
	}
	b.WriteString(arrowStyle.Render(arrow))
	b.WriteString(" ")
	b.WriteString(keyStyle.Render(node.key))

	if !expanded {
		previewBudget := width - lipgloss.Width(b.String()) - 2
		if previewBudget > 5 {
			preview := collapsedPreview(node, previewBudget)
			b.WriteString("  ")
			b.WriteString(m.styles.TextDim().Render(preview))
		}
	}

	return ansi.Truncate(b.String(), width, symEllipsis)
}

// renderLeafNode renders a key-value leaf node with per-type coloring.
func (m *Model) renderLeafNode(node *jsonTreeNode, width int, isCursor bool) string {
	prefixStyle := m.styles.TextDim()
	keyStyle := m.styles.TreeKey()
	colonStyle := m.styles.TextDim()
	valStyle := m.valueStyle(node.valueKind)

	if isCursor {
		keyStyle = keyStyle.Bold(true).Underline(true)
		valStyle = valStyle.Bold(true).Underline(true)
	}

	var b strings.Builder
	b.WriteString(prefixStyle.Render(node.treePrefix))
	b.WriteString(keyStyle.Render(node.key))
	b.WriteString(colonStyle.Render(": "))
	b.WriteString(valStyle.Render(node.value))

	return ansi.Truncate(b.String(), width, symEllipsis)
}

// valueStyle returns the lipgloss style for a tree value based on its type.
func (m *Model) valueStyle(kind treeValueKind) lipgloss.Style {
	switch kind {
	case tvString:
		return m.styles.TreeString()
	case tvNumber:
		return m.styles.TreeNumber()
	case tvBool:
		return m.styles.TreeBool()
	case tvNull:
		return m.styles.TreeNull()
	case tvOther:
		return m.styles.Base()
	}
	panic(fmt.Sprintf("unhandled treeValueKind: %d", kind))
}

// classifyValue formats and classifies a JSON value for display and coloring.
func classifyValue(v any) (string, treeValueKind) {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val), tvString
	case json.Number:
		return val.String(), tvNumber
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10), tvNumber
		}
		return fmt.Sprintf("%g", val), tvNumber
	case bool:
		return strconv.FormatBool(val), tvBool
	case nil:
		return "null", tvNull
	default:
		return fmt.Sprintf("%v", val), tvOther
	}
}
