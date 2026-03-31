// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"fmt"

	"github.com/micasa-dev/micasa/internal/data"
)

func (m *Model) activeTab() *Tab {
	if m.active < 0 || m.active >= len(m.tabs) {
		return nil
	}
	return &m.tabs[m.active]
}

// detail returns the top of the drilldown stack, or nil if no detail view
// is active.
func (m *Model) detail() *detailContext {
	if len(m.detailStack) == 0 {
		return nil
	}
	return m.detailStack[len(m.detailStack)-1]
}

// inDetail reports whether a detail drilldown is active.
func (m *Model) inDetail() bool {
	return len(m.detailStack) > 0
}

// effectiveTab returns the detail tab when a detail view is open, otherwise
// the main active tab. All interaction code should use this.
func (m *Model) effectiveTab() *Tab {
	if dc := m.detail(); dc != nil {
		return &dc.Tab
	}
	return m.activeTab()
}

// detailDef describes how to construct a detail drilldown view. Shared by the
// named openXxxDetail helpers and the table-driven openDetailForRow dispatch.
type detailDef struct {
	tabKind    TabKind
	subName    string
	specs      func() []columnSpec
	handler    func(parentID string) TabHandler
	breadcrumb func(m *Model, parentName string) string
	getName    func(store *data.Store, id string) (string, error) // resolve parent display name
}

// stdBreadcrumb returns a breadcrumb builder that produces the standard
// "prefix > parentName > subName" format. Pass "" for subName to omit it.
func stdBreadcrumb(prefix, subName string) func(*Model, string) string {
	return func(_ *Model, parentName string) string {
		if subName == "" {
			return prefix + breadcrumbSep + parentName
		}
		return prefix + breadcrumbSep + parentName + breadcrumbSep + subName
	}
}

var (
	serviceLogDef = detailDef{
		tabKind: tabMaintenance,
		subName: "Service Log",
		specs:   serviceLogColumnSpecs,
		handler: func(id string) TabHandler { return newServiceLogHandler(id) },
		breadcrumb: func(m *Model, parentName string) string {
			// When drilled from the top-level Maintenance tab, the breadcrumb
			// starts with "Maintenance"; when nested (e.g. Appliances > ... >
			// Maint item), the parent context is already on the stack.
			bc := parentName + breadcrumbSep + "Service Log"
			if !m.inDetail() {
				bc = "Maintenance" + breadcrumbSep + bc
			}
			return bc
		},
		getName: func(s *data.Store, id string) (string, error) {
			item, err := s.GetMaintenance(id)
			if err != nil {
				return "", fmt.Errorf("load maintenance item: %w", err)
			}
			return item.Name, nil
		},
	}
	applianceMaintenanceDef = detailDef{
		tabKind:    tabAppliances,
		subName:    "Maintenance",
		specs:      applianceMaintenanceColumnSpecs,
		handler:    func(id string) TabHandler { return newApplianceMaintenanceHandler(id) },
		breadcrumb: stdBreadcrumb("Appliances", ""),
		getName: func(s *data.Store, id string) (string, error) {
			a, err := s.GetAppliance(id)
			if err != nil {
				return "", fmt.Errorf("load appliance: %w", err)
			}
			return a.Name, nil
		},
	}
	vendorQuoteDef = detailDef{
		tabKind:    tabVendors,
		subName:    tabQuotes.String(),
		specs:      vendorQuoteColumnSpecs,
		handler:    func(id string) TabHandler { return newVendorQuoteHandler(id) },
		breadcrumb: stdBreadcrumb("Vendors", tabQuotes.String()),
		getName:    getVendorName,
	}
	vendorJobsDef = detailDef{
		tabKind:    tabVendors,
		subName:    "Jobs",
		specs:      vendorJobsColumnSpecs,
		handler:    func(id string) TabHandler { return newVendorJobsHandler(id) },
		breadcrumb: stdBreadcrumb("Vendors", "Jobs"),
		getName:    getVendorName,
	}
	projectQuoteDef = detailDef{
		tabKind:    tabProjects,
		subName:    tabQuotes.String(),
		specs:      projectQuoteColumnSpecs,
		handler:    func(id string) TabHandler { return newProjectQuoteHandler(id) },
		breadcrumb: stdBreadcrumb("Projects", tabQuotes.String()),
		getName:    getProjectTitle,
	}
	projectDocumentDef = detailDef{
		tabKind:    tabProjects,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id string) TabHandler { return newEntityDocumentHandler(data.DocumentEntityProject, id) },
		breadcrumb: stdBreadcrumb("Projects", tabDocuments.String()),
		getName:    getProjectTitle,
	}
	incidentDocumentDef = detailDef{
		tabKind:    tabIncidents,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id string) TabHandler { return newEntityDocumentHandler(data.DocumentEntityIncident, id) },
		breadcrumb: stdBreadcrumb("Incidents", tabDocuments.String()),
		getName:    getIncidentTitle,
	}
	applianceDocumentDef = detailDef{
		tabKind:    tabAppliances,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id string) TabHandler { return newEntityDocumentHandler(data.DocumentEntityAppliance, id) },
		breadcrumb: stdBreadcrumb("Appliances", tabDocuments.String()),
		getName: func(s *data.Store, id string) (string, error) {
			a, err := s.GetAppliance(id)
			if err != nil {
				return "", fmt.Errorf("load appliance: %w", err)
			}
			return a.Name, nil
		},
	}
	serviceLogDocumentDef = detailDef{
		tabKind: tabMaintenance,
		subName: tabDocuments.String(),
		specs:   entityDocumentColumnSpecs,
		handler: func(id string) TabHandler {
			return newEntityDocumentHandler(data.DocumentEntityServiceLog, id)
		},
		breadcrumb: func(_ *Model, parentName string) string {
			// Service log docs are always opened from within a service log
			// detail, so the parent breadcrumb is already on the stack.
			return parentName + breadcrumbSep + tabDocuments.String()
		},
		getName: func(s *data.Store, id string) (string, error) {
			entry, err := s.GetServiceLog(id)
			if err != nil {
				return "", fmt.Errorf("load service log: %w", err)
			}
			return entry.ServicedAt.Format(data.DateLayout), nil
		},
	}
	maintenanceDocumentDef = detailDef{
		tabKind:    tabMaintenance,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id string) TabHandler { return newEntityDocumentHandler(data.DocumentEntityMaintenance, id) },
		breadcrumb: stdBreadcrumb("Maintenance", tabDocuments.String()),
		getName:    getMaintenanceName,
	}
	quoteDocumentDef = detailDef{
		tabKind:    tabQuotes,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id string) TabHandler { return newEntityDocumentHandler(data.DocumentEntityQuote, id) },
		breadcrumb: stdBreadcrumb("Quotes", tabDocuments.String()),
		getName:    getQuoteDisplayName,
	}
	vendorDocumentDef = detailDef{
		tabKind:    tabVendors,
		subName:    tabDocuments.String(),
		specs:      entityDocumentColumnSpecs,
		handler:    func(id string) TabHandler { return newEntityDocumentHandler(data.DocumentEntityVendor, id) },
		breadcrumb: stdBreadcrumb("Vendors", tabDocuments.String()),
		getName:    getVendorName,
	}
)

// Shared getName helpers for defs that resolve the same entity type.
func getVendorName(s *data.Store, id string) (string, error) {
	v, err := s.GetVendor(id)
	if err != nil {
		return "", fmt.Errorf("load vendor: %w", err)
	}
	return v.Name, nil
}

func getIncidentTitle(s *data.Store, id string) (string, error) {
	inc, err := s.GetIncident(id)
	if err != nil {
		return "", fmt.Errorf("load incident: %w", err)
	}
	return inc.Title, nil
}

func getProjectTitle(s *data.Store, id string) (string, error) {
	p, err := s.GetProject(id)
	if err != nil {
		return "", fmt.Errorf("load project: %w", err)
	}
	return p.Title, nil
}

func getMaintenanceName(s *data.Store, id string) (string, error) {
	item, err := s.GetMaintenance(id)
	if err != nil {
		return "", fmt.Errorf("load maintenance item: %w", err)
	}
	return item.Name, nil
}

func getQuoteDisplayName(s *data.Store, id string) (string, error) {
	q, err := s.GetQuote(id)
	if err != nil {
		return "", fmt.Errorf("load quote: %w", err)
	}
	return q.Vendor.Name + " #" + q.ID, nil
}

// openDetailFromDef opens a drilldown using a detail definition.
func (m *Model) openDetailFromDef(def detailDef, parentID string, parentName string) error {
	specs := def.specs()
	return m.openDetailWith(detailContext{
		ParentTabIndex: m.active,
		ParentRowID:    parentID,
		Breadcrumb:     def.breadcrumb(m, parentName),
		Tab: Tab{
			Kind:    def.tabKind,
			Name:    def.subName,
			Handler: def.handler(parentID),
			Specs:   specs,
			Table:   newTable(specsToColumns(specs)),
		},
	})
}

func (m *Model) openServiceLogDetail(maintID string, maintName string) error {
	return m.openDetailFromDef(serviceLogDef, maintID, maintName)
}

func (m *Model) openApplianceMaintenanceDetail(
	applianceID string, //nolint:unparam // varies at runtime; tests use a fixed ID
	applianceName string, //nolint:unparam // varies at runtime; tests use a fixed name
) error {
	return m.openDetailFromDef(applianceMaintenanceDef, applianceID, applianceName)
}

func (m *Model) openVendorQuoteDetail(vendorID string, vendorName string) error {
	return m.openDetailFromDef(vendorQuoteDef, vendorID, vendorName)
}

func (m *Model) openVendorJobsDetail(vendorID string, vendorName string) error {
	return m.openDetailFromDef(vendorJobsDef, vendorID, vendorName)
}

func (m *Model) openProjectQuoteDetail(projectID string, projectTitle string) error {
	return m.openDetailFromDef(projectQuoteDef, projectID, projectTitle)
}

func (m *Model) openProjectDocumentDetail(projectID string, projectTitle string) error {
	return m.openDetailFromDef(projectDocumentDef, projectID, projectTitle)
}

func (m *Model) openApplianceDocumentDetail(applianceID string, applianceName string) error {
	return m.openDetailFromDef(applianceDocumentDef, applianceID, applianceName)
}

// detailRoute maps a (TabKind, colTitle) pair to a detailDef for dispatch.
// When formKind is set (non-zero), the route only matches if the tab handler's
// FormKind equals it. This disambiguates nested detail views that share a
// parent tabKind but belong to different entity types (e.g. appliance
// maintenance detail has tabKind=tabAppliances but formKind=formMaintenance).
type detailRoute struct {
	tabKinds []TabKind
	colTitle string
	def      detailDef
	formKind FormKind
}

var detailRoutes = []detailRoute{
	{tabKinds: []TabKind{tabMaintenance, tabAppliances}, colTitle: "Log", def: serviceLogDef},
	{tabKinds: []TabKind{tabAppliances}, colTitle: "Maint", def: applianceMaintenanceDef},
	{tabKinds: []TabKind{tabVendors}, colTitle: tabQuotes.String(), def: vendorQuoteDef},
	{tabKinds: []TabKind{tabVendors}, colTitle: "Jobs", def: vendorJobsDef},
	{tabKinds: []TabKind{tabProjects}, colTitle: tabQuotes.String(), def: projectQuoteDef},
	// Handler-scoped document routes: match nested detail views where the
	// parent tabKind is shared but the handler identifies the entity type.
	{
		tabKinds: []TabKind{tabMaintenance, tabAppliances},
		colTitle: tabDocuments.String(),
		def:      serviceLogDocumentDef,
		formKind: formServiceLog,
	},
	{
		tabKinds: []TabKind{tabMaintenance, tabAppliances},
		colTitle: tabDocuments.String(),
		def:      maintenanceDocumentDef,
		formKind: formMaintenance,
	},
	{
		tabKinds: []TabKind{tabQuotes, tabVendors, tabProjects},
		colTitle: tabDocuments.String(),
		def:      quoteDocumentDef,
		formKind: formQuote,
	},
	// Generic document routes: match top-level tabs (no formKind filter).
	{tabKinds: []TabKind{tabProjects}, colTitle: tabDocuments.String(), def: projectDocumentDef},
	{tabKinds: []TabKind{tabIncidents}, colTitle: tabDocuments.String(), def: incidentDocumentDef},
	{
		tabKinds: []TabKind{tabAppliances},
		colTitle: tabDocuments.String(),
		def:      applianceDocumentDef,
	},
	{tabKinds: []TabKind{tabVendors}, colTitle: tabDocuments.String(), def: vendorDocumentDef},
}

// openDetailForRow dispatches a drilldown based on the current tab kind and the
// column that was activated. Supports nested drilldowns (e.g. Appliance ->
// Maintenance -> Service Log).
func (m *Model) openDetailForRow(tab *Tab, rowID string, colTitle string) error {
	for _, route := range detailRoutes {
		if route.colTitle != colTitle {
			continue
		}
		if route.formKind != formNone &&
			(tab.Handler == nil || tab.Handler.FormKind() != route.formKind) {
			continue
		}
		for _, kind := range route.tabKinds {
			if tab.Kind == kind {
				name, err := route.def.getName(m.store, rowID)
				if err != nil {
					return err
				}
				return m.openDetailFromDef(route.def, rowID, name)
			}
		}
	}
	return nil
}

func (m *Model) openDetailWith(dc detailContext) error {
	m.detailStack = append(m.detailStack, &dc)
	if err := m.reloadDetailTab(); err != nil {
		m.detailStack = m.detailStack[:len(m.detailStack)-1]
		return err
	}
	m.resizeTables()
	m.status = statusMsg{}
	return nil
}

func (m *Model) closeDetail() {
	if len(m.detailStack) == 0 {
		return
	}
	top := m.detailStack[len(m.detailStack)-1]
	m.detailStack = m.detailStack[:len(m.detailStack)-1]

	// If there's still a parent detail view on the stack, reload it and
	// restore the cursor to the row that opened the now-closed child.
	if parent := m.detail(); parent != nil {
		if m.store != nil {
			m.surfaceError(m.reloadTab(&parent.Tab))
		}
		selectRowByID(&parent.Tab, top.ParentRowID)
	} else {
		// Back to a top-level tab.
		m.active = top.ParentTabIndex
		if m.store != nil {
			tab := m.activeTab()
			if tab != nil && tab.Stale {
				m.surfaceError(m.reloadIfStale(tab))
			} else {
				m.surfaceError(m.reloadActiveTab())
			}
		}
		if tab := m.activeTab(); tab != nil {
			selectRowByID(tab, top.ParentRowID)
		}
	}

	// When closing a mutated service log detail, move the column cursor
	// to the "Last" column so the user sees the synced date.
	if top.Mutated {
		if _, ok := top.Tab.Handler.(serviceLogHandler); ok {
			if tab := m.effectiveTab(); tab != nil && tab.Kind == tabMaintenance {
				tab.ColCursor = int(maintenanceColLast)
				m.updateTabViewport(tab)
				m.setStatusInfo("Last serviced date synced from service log.")
			}
		}
	}

	m.resizeTables()
	if !top.Mutated {
		m.status = statusMsg{}
	}
}

// closeAllDetails collapses the entire drilldown stack back to the top-level tab.
// Unlike calling closeDetail() in a loop, this pops the whole stack and only
// reloads the final destination tab once, avoiding wasted intermediate reloads.
func (m *Model) closeAllDetails() {
	if len(m.detailStack) == 0 {
		return
	}
	// The bottom entry holds the original top-level tab that we're returning to.
	bottom := m.detailStack[0]
	m.detailStack = nil
	m.active = bottom.ParentTabIndex
	if m.store != nil {
		tab := m.activeTab()
		if tab != nil && tab.Stale {
			m.surfaceError(m.reloadIfStale(tab))
		} else {
			m.surfaceError(m.reloadActiveTab())
		}
	}
	if tab := m.activeTab(); tab != nil {
		selectRowByID(tab, bottom.ParentRowID)
	}
	m.resizeTables()
	m.status = statusMsg{}
}

func (m *Model) reloadDetailTab() error {
	dc := m.detail()
	if dc == nil || m.store == nil {
		return nil
	}
	return m.reloadTab(&dc.Tab)
}

// switchToTab sets the active tab index, reloads it (lazy if stale), and
// clears the status message. Centralizes the reload-after-switch pattern.
func (m *Model) switchToTab(idx int) {
	m.active = idx
	m.status = statusMsg{}
	tab := m.activeTab()
	if tab != nil && tab.Stale {
		m.surfaceError(m.reloadIfStale(tab))
	} else {
		m.surfaceError(m.reloadActiveTab())
	}
}

func (m *Model) nextTab() {
	if len(m.tabs) == 0 {
		return
	}
	next := m.active + 1
	if next >= len(m.tabs) {
		return
	}
	m.switchToTab(next)
}

func (m *Model) prevTab() {
	if len(m.tabs) == 0 {
		return
	}
	if m.active <= 0 {
		return
	}
	m.switchToTab(m.active - 1)
}
