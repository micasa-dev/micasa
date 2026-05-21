// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/extract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMagicAddWorksInDocumentDrilldowns verifies that pressing Shift+A
// (magic-add / QuickAdd) inside any document drill-down opens the
// deferred-create document form, matching behavior on the top-level
// Documents tab. Without the fix, the dispatch falls through because the
// drill-down's Tab.Kind is the parent tab's kind (e.g. tabAppliances),
// not tabDocuments.
func TestMagicAddWorksInDocumentDrilldowns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entityKind string
		openDetail func(t *testing.T, m *Model) string // returns the scoped entity ID
	}{
		{
			name:       "project",
			entityKind: data.DocumentEntityProject,
			openDetail: func(t *testing.T, m *Model) string {
				t.Helper()
				types, err := m.store.ProjectTypes()
				require.NoError(t, err)
				require.NoError(t, m.store.CreateProject(&data.Project{
					Title:         "Magic Drill Proj",
					ProjectTypeID: types[0].ID,
					Status:        data.ProjectStatusPlanned,
				}))
				projects, err := m.store.ListProjects(false)
				require.NoError(t, err)
				require.NotEmpty(t, projects)
				require.NoError(t, m.openProjectDocumentDetail(projects[0].ID, "Magic Drill Proj"))
				return projects[0].ID
			},
		},
		{
			name:       "appliance",
			entityKind: data.DocumentEntityAppliance,
			openDetail: func(t *testing.T, m *Model) string {
				t.Helper()
				require.NoError(
					t,
					m.store.CreateAppliance(&data.Appliance{Name: "Magic Dishwasher"}),
				)
				appls, err := m.store.ListAppliances(false)
				require.NoError(t, err)
				require.NotEmpty(t, appls)
				require.NoError(t, m.openApplianceDocumentDetail(appls[0].ID, "Magic Dishwasher"))
				return appls[0].ID
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newTestModelWithStore(t)
			entityID := tc.openDetail(t, m)
			require.True(t, m.inDetail())
			require.True(
				t,
				m.effectiveTab().isDocumentTab(),
				"drill-down tab should be detected as a document tab",
			)

			m.enterEditMode()
			require.Equal(t, modeEdit, m.mode)

			sendKey(m, keyShiftA)
			assert.Equal(t, modeForm, m.mode,
				"Shift+A in docs drill-down should open the quick-add form")

			fd, ok := m.fs.formData.(*documentFormData)
			require.True(t, ok, "form data should be documentFormData")
			assert.True(t, fd.DeferCreate, "quick-add form must set DeferCreate")
			assert.Equal(t, tc.entityKind, fd.EntityRef.Kind,
				"quick-add in drill-down must pre-scope to the parent entity kind")
			assert.Equal(t, entityID, fd.EntityRef.ID,
				"quick-add in drill-down must pre-scope to the parent entity ID")
		})
	}
}

// TestMagicAddOnTopLevelDocumentsTabLeavesScopeEmpty ensures the fix for
// drill-down parity does not regress the top-level Documents tab, where
// the entity is chosen by the user (or by LLM extraction) rather than
// pre-scoped.
func TestMagicAddOnTopLevelDocumentsTabLeavesScopeEmpty(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	m.active = tabIndex(tabDocuments)
	require.False(t, m.inDetail())

	m.enterEditMode()
	sendKey(m, keyShiftA)
	require.Equal(t, modeForm, m.mode)

	fd, ok := m.fs.formData.(*documentFormData)
	require.True(t, ok)
	assert.True(t, fd.DeferCreate)
	assert.Empty(t, fd.EntityRef.Kind, "top-level quick-add must not pre-scope")
	assert.Empty(t, fd.EntityRef.ID, "top-level quick-add must not pre-scope")
}

// TestAcceptDeferredExtraction_PreservesPreScopedEntity ensures that
// when magic-add is triggered in a drill-down (so the pending doc
// already has EntityKind/EntityID set), the subsequent extraction
// accept does NOT let the LLM override the user-chosen scope.
func TestAcceptDeferredExtraction_PreservesPreScopedEntity(t *testing.T) {
	t.Parallel()
	m := newTestModelWithStore(t)
	require.NoError(t, m.store.CreateAppliance(&data.Appliance{Name: "Scoped Appl"}))
	appls, err := m.store.ListAppliances(false)
	require.NoError(t, err)
	require.NotEmpty(t, appls)
	applID := appls[0].ID

	ex := &extractionLogState{
		ID:         42,
		Visible:    true,
		Done:       true,
		toolCursor: -1,
		expanded:   make(map[extractionStep]bool),
		pendingDoc: &data.Document{
			FileName:   "manual.pdf",
			MIMEType:   "application/pdf",
			Data:       []byte("pdf-bytes"),
			EntityKind: data.DocumentEntityAppliance,
			EntityID:   applID,
		},
	}
	// LLM proposes a different entity; the drill-down scope must win.
	ex.operations = []extract.Operation{
		{Action: "create", Table: data.TableDocuments, Data: map[string]any{
			"title":       "Dishwasher Manual",
			"notes":       "warranty info",
			"entity_kind": data.DocumentEntityVendor,
			"entity_id":   "01JNOTEXIST0000000VENDOR",
		}},
	}
	m.ex.extraction = ex

	require.NoError(t, m.acceptDeferredExtraction())

	docs, err := m.store.ListDocuments(false)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Dishwasher Manual", docs[0].Title, "LLM title should still apply")
	assert.Equal(t, data.DocumentEntityAppliance, docs[0].EntityKind,
		"pre-scoped entity kind must survive LLM override")
	assert.Equal(t, applID, docs[0].EntityID,
		"pre-scoped entity ID must survive LLM override")
}
