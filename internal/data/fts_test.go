// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"strings"
	"testing"
	"time"

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

// --- entities_fts tests ---

func TestSetupEntitiesFTSCreatesTable(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	var count int64
	store.db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		"entities_fts",
	).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSPopulatesProjects(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title:         "Kitchen Remodel",
		Description:   "Full kitchen renovation",
		Status:        ProjectStatusInProgress,
		ProjectTypeID: types[0].ID,
	}))

	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'project'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSPopulatesVendors(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Test Plumber", ContactName: "John"}))
	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'vendor'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSPopulatesAppliances(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&Appliance{Name: "HVAC Unit", Brand: "Carrier"}))
	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'appliance'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSPopulatesIncidents(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title:    "Roof Leak",
		Status:   IncidentStatusOpen,
		Severity: IncidentSeverityUrgent,
	}))
	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'incident'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSetupEntitiesFTSExcludesSoftDeleted(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Active Vendor"}))
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Deleted Vendor"}))

	vendors, _ := store.ListVendors(false)
	require.Len(t, vendors, 2)
	require.NoError(t, store.DeleteVendor(vendors[1].ID))

	require.NoError(t, store.setupEntitiesFTS())

	var count int64
	store.db.Raw(`SELECT COUNT(*) FROM entities_fts WHERE entity_type = 'vendor'`).Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestSearchEntitiesBasic(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Kitchen Remodel", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	require.NoError(t, store.CreateVendor(&Vendor{Name: "ABC Plumbing"}))
	require.NoError(t, store.setupEntitiesFTS())

	results, err := store.SearchEntities("kitchen")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "project", results[0].EntityType)
	assert.Equal(t, "Kitchen Remodel", results[0].EntityName)
}

func TestSearchEntitiesEmptyQuery(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	results, err := store.SearchEntities("")
	require.NoError(t, err)
	assert.Nil(t, results)

	results, err = store.SearchEntities("   ")
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestSearchEntitiesBadSyntaxGraceful(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	results, err := store.SearchEntities(`"unclosed`)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchEntitiesCrossEntityMatches(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Plumbing Overhaul", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	require.NoError(t, store.CreateVendor(&Vendor{Name: "Pro Plumbing"}))
	require.NoError(t, store.setupEntitiesFTS())

	results, err := store.SearchEntities("plumbing")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSearchEntitiesNoFTSTableGraceful(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.db.Exec("DROP TABLE IF EXISTS entities_fts").Error)

	results, err := store.SearchEntities("anything")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchEntitiesWrapsQueryError(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Replace the well-formed entities_fts with a virtual table that has a
	// different column set, so hasEntitiesFTSTable still reports true but
	// the SELECT against the expected columns errors at query time.
	require.NoError(t, store.db.Exec("DROP TABLE IF EXISTS entities_fts").Error)
	require.NoError(t, store.db.Exec(
		"CREATE VIRTUAL TABLE entities_fts USING fts5(unrelated_column)",
	).Error)

	results, err := store.SearchEntities("anything")
	require.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "search entities:")
}

func TestSearchEntitiesPorterStemming(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Professional Painting Services"}))
	require.NoError(t, store.setupEntitiesFTS())

	// "painted" should match "painting" via porter stemmer (both stem to "paint").
	results, err := store.SearchEntities("painted")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestEntitySummaryProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	budget := int64(1500000)
	require.NoError(t, store.CreateProject(&Project{
		Title: "Kitchen Remodel", ProjectTypeID: types[0].ID,
		Status: ProjectStatusInProgress, BudgetCents: &budget,
	}))
	projects, _ := store.ListProjects(false)
	require.Len(t, projects, 1)

	summary, found, err := store.EntitySummary("project", projects[0].ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, summary, "Kitchen Remodel")
	assert.Contains(t, summary, "underway")
	assert.Contains(t, summary, "$15000.00")
}

func TestEntitySummaryVendor(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{
		Name: "ABC Plumbing", ContactName: "John Smith", Phone: "555-0123",
	}))
	vendors, _ := store.ListVendors(false)

	summary, found, err := store.EntitySummary("vendor", vendors[0].ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, summary, "ABC Plumbing")
	assert.Contains(t, summary, "contact=John Smith")
	assert.Contains(t, summary, "phone=555-0123")
}

func TestEntitySummaryAppliance(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateAppliance(&Appliance{
		Name: "Dishwasher", Brand: "LG", ModelNumber: "WM3900",
	}))
	appliances, _ := store.ListAppliances(false)

	summary, found, err := store.EntitySummary("appliance", appliances[0].ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, summary, "Dishwasher")
	assert.Contains(t, summary, "brand=LG")
	assert.Contains(t, summary, "model=WM3900")
}

func TestEntitySummaryDeletedEntity(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Gone Vendor"}))
	vendors, _ := store.ListVendors(false)
	require.NoError(t, store.DeleteVendor(vendors[0].ID))

	_, found, err := store.EntitySummary("vendor", vendors[0].ID)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestEntitySummaryUnknownType(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	_, found, err := store.EntitySummary("nonexistent", "01JFAKE")
	require.Error(t, err)
	assert.False(t, found)
}

func TestEntitySummaryRevalidatesStaleIndex(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateVendor(&Vendor{Name: "Will Be Deleted"}))
	require.NoError(t, store.setupEntitiesFTS())

	results, err := store.SearchEntities("deleted")
	require.NoError(t, err)
	require.Len(t, results, 1)

	vendors, _ := store.ListVendors(false)
	require.NoError(t, store.DeleteVendor(vendors[0].ID))

	_, found, err := store.EntitySummary(results[0].EntityType, results[0].EntityID)
	require.NoError(t, err)
	assert.False(t, found, "deleted entity should not be found via EntitySummary")
}

func TestEntitySummaryIncident(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	require.NoError(t, store.CreateIncident(&Incident{
		Title: "Roof Leak", Status: IncidentStatusOpen,
		Severity: IncidentSeverityUrgent, Location: "attic",
	}))
	incidents, _ := store.ListIncidents(false)

	summary, found, err := store.EntitySummary("incident", incidents[0].ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Contains(t, summary, "Roof Leak")
	assert.Contains(t, summary, "status=open")
	assert.Contains(t, summary, "severity=urgent")
	assert.Contains(t, summary, "location=attic")
}

func TestTruncateField(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "short", truncateField("short"))
	long := strings.Repeat("a", 300)
	result := truncateField(long)
	assert.Len(t, result, 203) // 200 + "..."
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestTruncateFieldUnicode(t *testing.T) {
	t.Parallel()
	// 201 runes of multi-byte characters should truncate at rune boundary.
	s := strings.Repeat("\u00e9", 201) // e-acute
	result := truncateField(s)
	runes := []rune(result)
	// 200 runes + "..." = 203 runes
	assert.Len(t, runes, 203)
}

func TestHasEntitiesFTSTable(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	assert.True(t, store.hasEntitiesFTSTable())
}

// TestPopulateEntitiesFTSFiltersSoftDeletedQuoteParents verifies the JOIN
// filters in populateEntitiesFTS. FK RESTRICT prevents soft-deleting a
// project or vendor through the normal API while a quote references them,
// so we set deleted_at directly to simulate the scenario a bulk import,
// manual migration, or future API change could produce -- the index must
// not leak the deleted parent's text regardless.
func TestPopulateEntitiesFTSFiltersSoftDeletedQuoteParents(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Kitchen Remodel", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	projects, _ := store.ListProjects(false)
	require.Len(t, projects, 1)

	require.NoError(t, store.CreateQuote(
		&Quote{ProjectID: projects[0].ID, TotalCents: 10000, Notes: "plumbing"},
		Vendor{Name: "Pacific Plumbing"},
	))
	require.NoError(t, store.setupEntitiesFTS())

	var name string
	store.db.Raw(
		`SELECT entity_name FROM entities_fts WHERE entity_type = 'quote'`,
	).Scan(&name)
	require.Contains(t, name, "Kitchen Remodel")
	require.Contains(t, name, "Pacific Plumbing")

	// Bypass RESTRICT: set deleted_at directly.
	now := time.Now()
	require.NoError(t, store.db.Exec(
		`UPDATE projects SET deleted_at = ? WHERE id = ?`, now, projects[0].ID,
	).Error)
	require.NoError(t, store.setupEntitiesFTS())
	store.db.Raw(
		`SELECT entity_name FROM entities_fts WHERE entity_type = 'quote'`,
	).Scan(&name)
	assert.NotContains(t, name, "Kitchen Remodel",
		"soft-deleted project title must not appear in quote's entity_name")
	assert.Contains(t, name, "Pacific Plumbing",
		"vendor name should still appear")

	vendors, _ := store.ListVendors(false)
	require.Len(t, vendors, 1)
	require.NoError(t, store.db.Exec(
		`UPDATE vendors SET deleted_at = ? WHERE id = ?`, now, vendors[0].ID,
	).Error)
	require.NoError(t, store.setupEntitiesFTS())
	store.db.Raw(
		`SELECT entity_name FROM entities_fts WHERE entity_type = 'quote'`,
	).Scan(&name)
	assert.NotContains(t, name, "Pacific Plumbing",
		"soft-deleted vendor name must not appear in quote's entity_name")
}

// TestPopulateEntitiesFTSFiltersSoftDeletedMaintenanceForServiceLog verifies
// the JOIN filter for service_log. See comment on the quote-parent test for
// why we bypass the normal API.
func TestPopulateEntitiesFTSFiltersSoftDeletedMaintenanceForServiceLog(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, _ := store.MaintenanceCategories()
	require.NoError(t, store.CreateMaintenance(&MaintenanceItem{
		Name: "HVAC Filter", CategoryID: cats[0].ID,
	}))
	items, _ := store.ListMaintenance(false)
	require.Len(t, items, 1)

	require.NoError(t, store.CreateServiceLog(&ServiceLogEntry{
		MaintenanceItemID: items[0].ID,
		ServicedAt:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Notes:             "swapped filter",
	}, Vendor{}))
	require.NoError(t, store.setupEntitiesFTS())

	var name string
	store.db.Raw(
		`SELECT entity_name FROM entities_fts WHERE entity_type = 'service_log'`,
	).Scan(&name)
	require.Contains(t, name, "HVAC Filter")

	require.NoError(t, store.db.Exec(
		`UPDATE maintenance_items SET deleted_at = ? WHERE id = ?`,
		time.Now(), items[0].ID,
	).Error)
	require.NoError(t, store.setupEntitiesFTS())
	store.db.Raw(
		`SELECT entity_name FROM entities_fts WHERE entity_type = 'service_log'`,
	).Scan(&name)
	assert.NotContains(t, name, "HVAC Filter",
		"soft-deleted maintenance name must not appear in service_log's entity_name")
}

// TestRebuildFTSIndexRefreshesEntities verifies that RebuildFTSIndex
// repopulates entities_fts, not just the document index.
func TestRebuildFTSIndexRefreshesEntities(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title: "Initial Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))
	require.NoError(t, store.RebuildFTSIndex())

	results, err := store.SearchEntities("initial")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Initial Project", results[0].EntityName)

	// Add a second project after rebuild; it's not in the index yet.
	require.NoError(t, store.CreateProject(&Project{
		Title: "Later Project", ProjectTypeID: types[0].ID, Status: ProjectStatusPlanned,
	}))

	// RebuildFTSIndex must pick it up.
	require.NoError(t, store.RebuildFTSIndex())
	results, err = store.SearchEntities("later")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Later Project", results[0].EntityName)
}

// ---------------------------------------------------------------------------
// Trigger tests: verify that AI / AU / AD triggers keep entities_fts in sync
// with source-table writes without a manual setupEntitiesFTS rebuild.
// ---------------------------------------------------------------------------

func TestFTSTriggerInsertSurfacesProject(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	require.NoError(t, store.CreateProject(&Project{
		Title:         "Greenhouse Build",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}))

	results, err := store.SearchEntities("greenhouse")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, DeletionEntityProject, results[0].EntityType)
	assert.Equal(t, "Greenhouse Build", results[0].EntityName)
}

func TestFTSTriggerUpdateSurfacesNewTitle(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Old Title",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	p.Title = "Fresh Greenhouse"
	require.NoError(t, store.UpdateProject(*p))

	// Old token no longer surfaces.
	oldResults, err := store.SearchEntities("old")
	require.NoError(t, err)
	assert.Empty(t, oldResults, "old title should be gone from FTS")

	// New token surfaces.
	newResults, err := store.SearchEntities("greenhouse")
	require.NoError(t, err)
	require.Len(t, newResults, 1)
	assert.Equal(t, "Fresh Greenhouse", newResults[0].EntityName)
}

func TestFTSTriggerSoftDeleteRemovesRow(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Transient Project",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	// Sanity: it's indexed.
	before, err := store.SearchEntities("transient")
	require.NoError(t, err)
	require.Len(t, before, 1)

	require.NoError(t, store.DeleteProject(p.ID))

	after, err := store.SearchEntities("transient")
	require.NoError(t, err)
	assert.Empty(t, after, "soft-deleted project must not surface")
}

func TestFTSTriggerCascadeOnProjectRename(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Kitchen Remodel",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	v := &Vendor{Name: "Pacific Plumbing"}
	require.NoError(t, store.CreateVendor(v))

	require.NoError(t, store.CreateQuote(&Quote{
		ProjectID:  p.ID,
		VendorID:   v.ID,
		TotalCents: 1000,
	}, *v))

	// Rename the project.
	p.Title = "Greenhouse Build"
	require.NoError(t, store.UpdateProject(*p))

	// The quote should now be findable by the new project name.
	results, err := store.SearchEntities("greenhouse")
	require.NoError(t, err)

	var quoteFound bool
	for _, r := range results {
		if r.EntityType == DeletionEntityQuote {
			quoteFound = true
			break
		}
	}
	assert.True(
		t,
		quoteFound,
		"cascade should rebuild quote FTS with new project title; got %+v",
		results,
	)
}

func TestFTSTriggerCascadeOnVendorRename(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Basement Refinish",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	v := &Vendor{Name: "Old Vendor Name"}
	require.NoError(t, store.CreateVendor(v))

	require.NoError(t, store.CreateQuote(&Quote{
		ProjectID:  p.ID,
		VendorID:   v.ID,
		TotalCents: 2000,
	}, *v))

	v.Name = "Aurora Plumbing"
	require.NoError(t, store.UpdateVendor(*v))

	results, err := store.SearchEntities("aurora")
	require.NoError(t, err)

	var quoteFound bool
	for _, r := range results {
		if r.EntityType == DeletionEntityQuote {
			quoteFound = true
			break
		}
	}
	assert.True(
		t,
		quoteFound,
		"cascade should rebuild quote FTS with new vendor name; got %+v",
		results,
	)
}

func TestFTSTriggerCascadeOnMaintenanceRename(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	m := &MaintenanceItem{
		Name:           "Old Name",
		CategoryID:     cats[0].ID,
		IntervalMonths: 6,
	}
	require.NoError(t, store.CreateMaintenance(m))

	sle := &ServiceLogEntry{
		MaintenanceItemID: m.ID,
		ServicedAt:        time.Now(),
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{}))

	m.Name = "Quarterly Furnace Check"
	require.NoError(t, store.UpdateMaintenance(*m))

	results, err := store.SearchEntities("furnace")
	require.NoError(t, err)

	var sleFound bool
	for _, r := range results {
		if r.EntityType == DeletionEntityServiceLog {
			sleFound = true
			break
		}
	}
	assert.True(
		t,
		sleFound,
		"cascade should rebuild SLE FTS with new maintenance item name; got %+v",
		results,
	)
}

func TestFTSTriggerCascadeOnProjectSoftDelete(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Attic Insulation",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))

	v := &Vendor{Name: "Summit Insulators"}
	require.NoError(t, store.CreateVendor(v))

	require.NoError(t, store.CreateQuote(&Quote{
		ProjectID:  p.ID,
		VendorID:   v.ID,
		TotalCents: 3000,
	}, *v))

	// App-level DeleteProject refuses soft-delete when a project has live
	// quotes. The trigger's cascade path is still reachable via sync and
	// future app changes, so exercise it via raw DML that bypasses the
	// validation — the goal is to prove the DB trigger behaves correctly
	// when the scenario arises, not to test DeleteProject's gating.
	require.NoError(t, store.db.Exec(
		"UPDATE "+TableProjects+" SET "+ColDeletedAt+" = ? WHERE "+ColID+" = ?",
		time.Now(), p.ID,
	).Error)

	// Searching by vendor name should still surface the quote (with a
	// degraded entity_name now that the project title is gone).
	results, err := store.SearchEntities("summit")
	require.NoError(t, err)

	var quoteFound bool
	for _, r := range results {
		if r.EntityType == DeletionEntityQuote {
			quoteFound = true
			assert.NotContains(t, r.EntityName, "Attic Insulation",
				"soft-deleted project title must not be in child entity_name")
		}
	}
	assert.True(t, quoteFound, "quote should still surface via vendor name; got %+v", results)

	// And searching by the now-gone project title should NOT find the quote.
	attic, err := store.SearchEntities("attic")
	require.NoError(t, err)
	assert.Empty(t, attic, "soft-deleted project title should not surface via any entity")
}

func TestFTSTriggerHardDeleteMaintenanceCascadesSLE(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	m := &MaintenanceItem{
		Name:           "Gutter Cleaning",
		CategoryID:     cats[0].ID,
		IntervalMonths: 12,
	}
	require.NoError(t, store.CreateMaintenance(m))

	sle := &ServiceLogEntry{
		MaintenanceItemID: m.ID,
		ServicedAt:        time.Now(),
		Notes:             "fall cleanup",
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{}))

	require.NoError(t, store.HardDeleteMaintenance(m.ID))

	gutterResults, err := store.SearchEntities("gutter")
	require.NoError(t, err)
	assert.Empty(t, gutterResults, "maintenance item FTS row should be gone after hard delete")

	fallResults, err := store.SearchEntities("fall")
	require.NoError(t, err)
	assert.Empty(t, fallResults, "child SLE FTS row should be gone via FK cascade + _ad trigger")
}

func TestFTSPopulateFiltersSoftDeletedMaintenanceInSLEJoin(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	cats, err := store.MaintenanceCategories()
	require.NoError(t, err)
	require.NotEmpty(t, cats)

	m := &MaintenanceItem{
		Name:           "Rebuild Maintenance Name",
		CategoryID:     cats[0].ID,
		IntervalMonths: 12,
	}
	require.NoError(t, store.CreateMaintenance(m))

	sle := &ServiceLogEntry{
		MaintenanceItemID: m.ID,
		ServicedAt:        time.Now(),
		Notes:             "still-alive notes",
	}
	require.NoError(t, store.CreateServiceLog(sle, Vendor{}))

	// App-level DeleteMaintenance validation would reject this with a
	// live SLE, so bypass via raw SQL to simulate the sync / future
	// scenario where the parent arrives soft-deleted.
	require.NoError(t, store.db.Exec(
		"UPDATE "+TableMaintenanceItems+" SET "+ColDeletedAt+" = ? WHERE "+ColID+" = ?",
		time.Now(), m.ID,
	).Error)

	// Force the initial-rebuild path.
	require.NoError(t, store.setupEntitiesFTS())

	results, err := store.SearchEntities("rebuild")
	require.NoError(t, err)
	for _, r := range results {
		if r.EntityType == DeletionEntityServiceLog {
			assert.NotContains(t, r.EntityName, "Rebuild Maintenance Name",
				"initial rebuild must not carry soft-deleted maintenance name into SLE FTS")
		}
	}
}

func TestFTSPopulateFiltersSoftDeletedParentsInQuoteJoin(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)

	// Create project + vendor + quote, soft-delete the project via raw
	// SQL (the app-level DeleteProject rejects parents with live quotes),
	// then run the initial rebuild path. The quote's FTS row must not
	// carry the deleted project's title.
	types, _ := store.ProjectTypes()
	p := &Project{
		Title:         "Rebuild Project Title",
		ProjectTypeID: types[0].ID,
		Status:        ProjectStatusPlanned,
	}
	require.NoError(t, store.CreateProject(p))
	v := &Vendor{Name: "Rebuild Vendor Name"}
	require.NoError(t, store.CreateVendor(v))
	require.NoError(t, store.CreateQuote(&Quote{
		ProjectID:  p.ID,
		VendorID:   v.ID,
		TotalCents: 1000,
	}, *v))

	require.NoError(t, store.db.Exec(
		"UPDATE "+TableProjects+" SET "+ColDeletedAt+" = ? WHERE "+ColID+" = ?",
		time.Now(), p.ID,
	).Error)

	// Force the initial-rebuild path (mirrors what happens on app open).
	require.NoError(t, store.setupEntitiesFTS())

	rebuild, err := store.SearchEntities("rebuild")
	require.NoError(t, err)
	for _, r := range rebuild {
		if r.EntityType == DeletionEntityQuote {
			assert.NotContains(t, r.EntityName, "Rebuild Project Title",
				"initial rebuild must not carry soft-deleted project title into quote FTS")
		}
	}
}
