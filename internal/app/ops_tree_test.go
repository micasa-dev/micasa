// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testOpsJSON is a raw JSON blob matching what the extraction pipeline stores.
var testOpsJSON = []byte(`[
	{"action":"create","table":"vendors","data":{"email":"info@garcia.com","name":"Garcia Plumbing","phone":"555-1234"}},
	{"action":"update","table":"documents","data":{"title":"Invoice #42"}}
]`)

// newOpsTreeModel creates a test model with a document that has extraction ops,
// navigates to the Documents tab, and reloads data so the Ops column is populated.
func newOpsTreeModel(t *testing.T) *Model {
	t.Helper()

	m := newTestModelWithStore(t)

	doc := &data.Document{
		Title:         "Test Invoice",
		FileName:      "invoice.pdf",
		MIMEType:      "application/pdf",
		ExtractionOps: testOpsJSON,
	}
	require.NoError(t, m.store.CreateDocument(doc))

	m.active = tabIndex(tabDocuments)
	require.NoError(t, m.reloadTab(m.effectiveTab()))

	return m
}

func TestOpsTreeOpenViaEnter(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	require.NotNil(t, tab)
	tab.ColCursor = int(documentColOps)

	sendKey(m, "enter")

	require.NotNil(t, m.opsTree, "enter on Ops column should open ops tree overlay")
	// Root is the "operations" wrapper with 2 children.
	require.Len(t, m.opsTree.root, 1)
	assert.Equal(t, "operations", m.opsTree.root[0].key)
	assert.Len(t, m.opsTree.root[0].children, 2)
	assert.Equal(t, "Test Invoice", m.opsTree.docTitle)
}

func TestOpsTreeDismissEsc(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	sendKey(m, "esc")
	assert.Nil(t, m.opsTree, "esc should close ops tree overlay")
}

func TestOpsTreeNavigateJK(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// "operations" is auto-expanded. With auto-expand depth<=2, full tree:
	// operations, [0], action, table, data, email, name, phone, [1], action, table, data, title
	// = 13 visible nodes.
	nodes := m.opsTree.visibleNodes()
	require.Len(t, nodes, 13)

	assert.Equal(t, 0, m.opsTree.cursor)

	sendKey(m, "j")
	assert.Equal(t, 1, m.opsTree.cursor)

	sendKey(m, "j")
	assert.Equal(t, 2, m.opsTree.cursor)

	sendKey(m, "k")
	assert.Equal(t, 1, m.opsTree.cursor)
}

func TestOpsTreeExpandCollapse(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// "operations" is auto-expanded. Navigate to [0] (node 1).
	sendKey(m, "j")
	assert.True(t, m.opsTree.expanded["operations.0"])

	sendKey(m, "h")
	assert.False(t, m.opsTree.expanded["operations.0"], "h should collapse [0]")

	// Re-expand with enter.
	sendKey(m, "enter")
	assert.True(t, m.opsTree.expanded["operations.0"], "enter should re-expand [0]")
}

func TestOpsTreeCollapseFromChild(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// Navigate to a leaf: operations(0), [0](1), action(2).
	sendKey(m, "j") // [0]
	sendKey(m, "j") // action (leaf)
	nodes := m.opsTree.visibleNodes()
	require.False(t, nodes[m.opsTree.cursor].isExpandable(), "should be on a leaf node")

	// Press h to collapse parent [0] and jump to it.
	sendKey(m, "h")
	assert.False(t, m.opsTree.expanded["operations.0"], "h on child should collapse parent [0]")
	assert.Equal(t, 1, m.opsTree.cursor, "cursor should jump to [0]")
}

func TestOpsTreeRendersInView(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	view := m.buildView()
	assert.Contains(t, view, "operations")
	assert.Contains(t, view, "action")
	assert.Contains(t, view, "vendors")
	assert.Contains(t, view, "Garcia Plumbing")
	assert.Contains(t, view, "├")
	assert.Contains(t, view, "└")
}

func TestOpsTreeNoOpenOnEmptyOps(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Create a document without extraction ops.
	doc := &data.Document{
		Title:    "Empty Doc",
		FileName: "empty.pdf",
		MIMEType: "application/pdf",
	}
	require.NoError(t, m.store.CreateDocument(doc))

	m.active = tabIndex(tabDocuments)
	require.NoError(t, m.reloadTab(m.effectiveTab()))

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")

	assert.Nil(t, m.opsTree, "should not open tree when no ops exist")
}

func TestOpsTreeHasActiveOverlay(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	assert.True(t, m.hasActiveOverlay(), "ops tree should count as an active overlay")
}

func TestOpsTreeGAndShiftG(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	nodes := m.opsTree.visibleNodes()
	require.Greater(t, len(nodes), 1)

	sendKey(m, "G")
	assert.Equal(t, len(nodes)-1, m.opsTree.cursor, "G should jump to last node")

	sendKey(m, "g")
	assert.Equal(t, 0, m.opsTree.cursor, "g should jump to first node")
}

func TestOpsTreeMouseClickToggle(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// Render to populate zones including the overlay wrapper.
	m.View()
	oz := m.zones.Get(zoneOverlay)
	if oz == nil || oz.IsZero() {
		t.Skip("overlay zone not rendered")
	}

	// Click on [0] node zone (node 1, after "operations" at 0).
	z := requireZone(t, m, zoneOpsNode+"1")
	sendClick(m, z.StartX, z.StartY)
	require.NotNil(t, m.opsTree, "click inside ops node zone should not dismiss overlay")
	// [0] was expanded (auto-expand), click should collapse it.
	assert.False(t, m.opsTree.expanded["operations.0"], "click should toggle expand state")
}

func TestOpsTreeLExpandsNode(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// Navigate to [0] (node 1) and collapse it, then use l to expand.
	sendKey(m, "j")
	m.opsTree.expanded["operations.0"] = false
	assert.False(t, m.opsTree.expanded["operations.0"])

	sendKey(m, "l")
	assert.True(t, m.opsTree.expanded["operations.0"], "l should expand the node")
}

func TestOpsCellRendering(t *testing.T) {
	t.Parallel()

	c := opsCell(testOpsJSON)
	assert.Equal(t, "2", c.Value)
	assert.Equal(t, cellOps, c.Kind)
}

func TestOpsCellEmpty(t *testing.T) {
	t.Parallel()

	c := opsCell(nil)
	assert.Empty(t, c.Value)
	assert.Equal(t, cellOps, c.Kind)

	c = opsCell([]byte("[]"))
	assert.Empty(t, c.Value)
	assert.Equal(t, cellOps, c.Kind)
}

func TestOpsColumnEnterHint(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	hint := m.enterHint()
	assert.Equal(t, "ops", hint)
}

func TestClassifyValueTypes(t *testing.T) {
	t.Parallel()

	val, kind := classifyValue("hello")
	assert.Equal(t, `"hello"`, val)
	assert.Equal(t, tvString, kind)

	val, kind = classifyValue(42.0)
	assert.Equal(t, "42", val)
	assert.Equal(t, tvNumber, kind)

	val, kind = classifyValue(3.14)
	assert.Equal(t, "3.14", val)
	assert.Equal(t, tvNumber, kind)

	val, kind = classifyValue(true)
	assert.Equal(t, "true", val)
	assert.Equal(t, tvBool, kind)

	val, kind = classifyValue(false)
	assert.Equal(t, "false", val)
	assert.Equal(t, tvBool, kind)

	val, kind = classifyValue(nil)
	assert.Equal(t, "null", val)
	assert.Equal(t, tvNull, kind)
}

func TestCollapsedPreview(t *testing.T) {
	t.Parallel()
	nodes := buildJSONTree([]byte(`[{"email":"info@garcia.com","name":"Garcia Plumbing"}]`))
	require.Len(t, nodes, 1)
	// The "operations" root wraps the array; [0] is at root.children[0].
	child := nodes[0].children[0]
	preview := collapsedPreview(child, 60)
	assert.Contains(t, preview, "email:")
	assert.Contains(t, preview, "name:")
	assert.True(t, strings.HasPrefix(preview, "{"))
	assert.True(t, strings.HasSuffix(preview, "}"))
}

func TestCollapsedPreviewTruncates(t *testing.T) {
	t.Parallel()
	nodes := buildJSONTree(
		[]byte(`[{"email":"info@garcia.com","name":"Garcia Plumbing","phone":"555-1234"}]`),
	)
	require.Len(t, nodes, 1)
	child := nodes[0].children[0]
	preview := collapsedPreview(child, 20)
	assert.Contains(t, preview, symEllipsis)
}

func TestOpsTreeCollapsedShowsPreview(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// Collapse [0].
	m.opsTree.expanded["operations.0"] = false

	view := m.buildView()
	assert.Contains(t, view, "[0]")
	assert.Contains(t, view, "{")
}

func TestBuildJSONTree(t *testing.T) {
	t.Parallel()
	root := buildJSONTree(testOpsJSON)
	require.Len(t, root, 1)

	// Root is the "operations" wrapper.
	ops := root[0]
	assert.Equal(t, "operations", ops.key)
	assert.True(t, ops.isArray)
	assert.True(t, ops.isExpandable())
	assert.Empty(t, ops.treePrefix, "root has no tree prefix")

	// Two array elements under operations.
	require.Len(t, ops.children, 2)
	first := ops.children[0]
	assert.Equal(t, "[0]", first.key)
	assert.Equal(t, treeBranch, first.treePrefix, "[0] is not last")
	assert.Equal(t, treeCorner, ops.children[1].treePrefix, "[1] is last")

	// [0] has children: action, table, data (ops key order).
	require.Len(t, first.children, 3)
	assert.Equal(t, "action", first.children[0].key)
	assert.Equal(t, "table", first.children[1].key)
	assert.Equal(t, "data", first.children[2].key)

	// Children of [0] have │ continuation since [0] is not last.
	assert.Equal(t, treePipe+treeBranch, first.children[0].treePrefix)
	assert.Equal(t, treePipe+treeCorner, first.children[2].treePrefix)

	// data is expandable with 3 children.
	dataNode := first.children[2]
	assert.True(t, dataNode.isExpandable())
	require.Len(t, dataNode.children, 3)
	assert.Equal(t, "email", dataNode.children[0].key)
	assert.Equal(t, "name", dataNode.children[1].key)
	assert.Equal(t, "phone", dataNode.children[2].key)

	// Leaf values are classified correctly.
	assert.Equal(t, `"info@garcia.com"`, dataNode.children[0].value)
	assert.Equal(t, tvString, dataNode.children[0].valueKind)
}

func TestAutoExpand(t *testing.T) {
	t.Parallel()
	root := buildJSONTree(testOpsJSON)
	expanded := make(map[string]bool)
	autoExpand(root, expanded, 2)

	// Depth 0: operations root.
	assert.True(t, expanded["operations"])

	// Depth 1: [0] and [1].
	assert.True(t, expanded["operations.0"])
	assert.True(t, expanded["operations.1"])

	// Depth 2: data sub-objects.
	assert.True(t, expanded["operations.0.data"])
	assert.True(t, expanded["operations.1.data"])
}

func TestOpsTreePreviewGroupsPopulated(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")

	require.NotNil(t, m.opsTree)
	require.NotEmpty(t, m.opsTree.previewGroups, "preview groups should be populated")

	// testOpsJSON has vendors create + documents update -> 2 groups.
	assert.Len(t, m.opsTree.previewGroups, 2)
	assert.Equal(t, 0, m.opsTree.previewTab)
}

func TestOpsTreeCollapseNestedContainer(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// Navigate to the "data" container node under [0].
	// Nodes: operations=0, [0]=1, action=2, table=3, data=4, ...
	sendKey(m, "j") // [0]
	sendKey(m, "j") // action
	sendKey(m, "j") // table
	sendKey(m, "j") // data
	nodes := m.opsTree.visibleNodes()
	require.Equal(t, "operations.0.data", nodes[m.opsTree.cursor].path)
	require.True(t, nodes[m.opsTree.cursor].isExpandable())

	// Collapse data with h.
	sendKey(m, "h")
	assert.False(t, m.opsTree.expanded["operations.0.data"], "h should collapse data node")
	assert.Equal(t, 4, m.opsTree.cursor, "cursor stays on data node")
}

func TestOpsTreeTablePreviewRendersInView(t *testing.T) {
	t.Parallel()
	m := newOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	require.NotEmpty(t, m.opsTree.previewGroups)

	view := m.buildView()

	// Table preview should show column headers from the vendor preview.
	assert.Contains(t, view, "Name")
	assert.Contains(t, view, "Email")
	assert.Contains(t, view, "Phone")

	// And the data values.
	assert.Contains(t, view, "Garcia Plumbing")
	assert.Contains(t, view, "info@garcia.com")
	assert.Contains(t, view, "555-1234")
}

func TestOpsTreeNoTablePreviewForUnknownTables(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Ops targeting a table with no previewColumns mapping.
	unknownOps := []byte(`[{"action":"create","table":"unknown_table","data":{"foo":"bar"}}]`)
	doc := &data.Document{
		Title:         "Unknown Table Doc",
		FileName:      "unknown.pdf",
		MIMEType:      "application/pdf",
		ExtractionOps: unknownOps,
	}
	require.NoError(t, m.store.CreateDocument(doc))

	m.active = tabIndex(tabDocuments)
	require.NoError(t, m.reloadTab(m.effectiveTab()))

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)

	// previewGroups should be empty since unknown_table has no column defs.
	assert.Empty(t, m.opsTree.previewGroups)

	// Should still render without crashing.
	view := m.buildView()
	assert.Contains(t, view, "operations")
}

var testMultiTableOpsJSON = []byte(`[
	{"action":"create","table":"vendors","data":{"name":"Garcia Plumbing","email":"info@garcia.com","phone":"555-1234"}},
	{"action":"create","table":"appliances","data":{"name":"Dishwasher","brand":"Bosch","model_number":"SHP65"}},
	{"action":"update","table":"documents","data":{"title":"Invoice #42"}}
]`)

func newMultiTableOpsTreeModel(t *testing.T) *Model {
	t.Helper()

	m := newTestModelWithStore(t)

	doc := &data.Document{
		Title:         "Multi-Table Invoice",
		FileName:      "invoice.pdf",
		MIMEType:      "application/pdf",
		ExtractionOps: testMultiTableOpsJSON,
	}
	require.NoError(t, m.store.CreateDocument(doc))

	m.active = tabIndex(tabDocuments)
	require.NoError(t, m.reloadTab(m.effectiveTab()))

	return m
}

func TestOpsTreeTabSwitchBF(t *testing.T) {
	t.Parallel()
	m := newMultiTableOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	// vendors + appliances + documents = 3 groups.
	require.Len(t, m.opsTree.previewGroups, 3)

	assert.Equal(t, 0, m.opsTree.previewTab)

	sendKey(m, "f")
	assert.Equal(t, 1, m.opsTree.previewTab)

	sendKey(m, "f")
	assert.Equal(t, 2, m.opsTree.previewTab)

	// f at last should clamp (no wrap).
	sendKey(m, "f")
	assert.Equal(t, 2, m.opsTree.previewTab)

	sendKey(m, "b")
	assert.Equal(t, 1, m.opsTree.previewTab)

	sendKey(m, "b")
	assert.Equal(t, 0, m.opsTree.previewTab)

	// b at 0 should clamp (no wrap).
	sendKey(m, "b")
	assert.Equal(t, 0, m.opsTree.previewTab)
}

func TestOpsTreeMouseClickTab(t *testing.T) {
	t.Parallel()
	m := newMultiTableOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	require.GreaterOrEqual(t, len(m.opsTree.previewGroups), 2)

	// Render to populate zones, including the outer overlay wrapper. The
	// overlay zone must be checked first so the click dispatcher can
	// recognize the click as inside the overlay.
	m.View()
	if oz := m.zones.Get(zoneOverlay); oz == nil || oz.IsZero() {
		t.Skip("overlay zone not rendered")
	}
	z := requireZone(t, m, fmt.Sprintf("%s%d", zoneOpsTab, 1))

	// Click on second tab.
	sendClick(m, z.StartX, z.StartY)

	require.NotNil(t, m.opsTree, "click on tab inside overlay should not dismiss the overlay")
	assert.Equal(t, 1, m.opsTree.previewTab)
}

// TestOpsTreeMouseClickTabZoneRace deterministically reproduces a flake seen
// on slow CI runners (e.g. windows-11-arm) where the click dispatcher would
// dismiss the ops tree overlay because the outer overlay zone hadn't been
// processed by the bubblezone async worker yet, even though the inner tab
// zone had. The scanner emits inner zones first (they close first) and the
// overlay zone last, so a partial drain can leave the inner zone visible
// while the overlay zone is still nil.
//
// We simulate this exact partial-drain state by waiting for both zones to
// be populated, then explicitly clearing the overlay zone before sending
// the click. Without the fix, handleLeftClick treats the missing overlay
// zone as "click outside overlay" and dismisses the overlay, niling out
// m.opsTree and panicking when the test reads previewTab.
func TestOpsTreeMouseClickTabZoneRace(t *testing.T) {
	t.Parallel()
	m := newMultiTableOpsTreeModel(t)

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	require.GreaterOrEqual(t, len(m.opsTree.previewGroups), 2)

	// Render once to enqueue zone updates on the async worker.
	m.View()

	// Drain the worker: poll until both the inner tab zone and the outer
	// overlay zone are populated. Because the scanner emits the overlay
	// last, waiting on it guarantees all inner zones are processed too.
	tabID := fmt.Sprintf("%s%d", zoneOpsTab, 1)
	var z *zone.ZoneInfo
	require.Eventually(t, func() bool {
		z = m.zones.Get(tabID)
		oz := m.zones.Get(zoneOverlay)
		return z != nil && !z.IsZero() && oz != nil && !oz.IsZero()
	}, 2*time.Second, time.Millisecond, "zones never populated")

	// Recreate the partial-drain race: inner zone present, overlay missing.
	m.zones.Clear(zoneOverlay)
	require.Nil(t, m.zones.Get(zoneOverlay))
	require.NotNil(t, m.zones.Get(tabID))

	sendClick(m, z.StartX, z.StartY)

	require.NotNil(t, m.opsTree,
		"click inside overlay must not dismiss it when overlay zone is unflushed")
	assert.Equal(t, 1, m.opsTree.previewTab)
}

func TestOpsTreeSingleGroupNoTabBar(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)

	// Single-table ops (vendors only).
	singleOps := []byte(
		`[{"action":"create","table":"vendors","data":{"name":"Solo Vendor","email":"solo@test.com"}}]`,
	)
	doc := &data.Document{
		Title:         "Single Table Doc",
		FileName:      "single.pdf",
		MIMEType:      "application/pdf",
		ExtractionOps: singleOps,
	}
	require.NoError(t, m.store.CreateDocument(doc))

	m.active = tabIndex(tabDocuments)
	require.NoError(t, m.reloadTab(m.effectiveTab()))

	tab := m.effectiveTab()
	tab.ColCursor = int(documentColOps)
	sendKey(m, "enter")
	require.NotNil(t, m.opsTree)
	require.Len(t, m.opsTree.previewGroups, 1)

	view := m.buildView()
	// Should show the table data but no tab bar.
	assert.Contains(t, view, "Solo Vendor")
	// b/f hint should not appear for single group.
	assert.NotContains(t, view, "tabs")
}
