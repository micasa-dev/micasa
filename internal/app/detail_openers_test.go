// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

// Test-only convenience methods for opening drilldown detail views.
//
// Production code reaches detailDef instances through the detailRoutes
// dispatch table in model_tabs.go (the table maps a current tab kind +
// column title to the right detailDef and parent name resolver). Tests
// frequently bypass that dispatch -- they want to open one specific
// detail view from a fixed parent ID without simulating the table
// navigation that would have triggered it. These wrappers exist solely
// to make those tests read like the production call path.
//
// Each method is a thin shim around openDetailFromDef. Adding a new
// detailDef does NOT require adding a wrapper here unless tests need it.

func (m *Model) openServiceLogDetail(maintID string, maintName string) error {
	return m.openDetailFromDef(serviceLogDef, maintID, maintName)
}

func (m *Model) openApplianceMaintenanceDetail(
	applianceID string, //nolint:unparam // signature mirrors the other openXxx helpers; tests always pass the same fixture ID
	applianceName string, //nolint:unparam // signature mirrors the other openXxx helpers; tests always pass the same fixture name
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
