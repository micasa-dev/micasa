// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import "time"

// ExtractionHints holds structured data extracted from a document by the
// LLM. Every field is optional -- the model fills what it can. These hints
// pre-fill form fields; the user confirms before saving.
type ExtractionHints struct {
	DocumentType   string            `json:"document_type"`
	TitleSugg      string            `json:"title_suggestion"`
	Summary        string            `json:"summary"`
	VendorHint     string            `json:"vendor_hint"`
	TotalCents     *int64            `json:"total_cents"`
	LaborCents     *int64            `json:"labor_cents"`
	MaterialsCents *int64            `json:"materials_cents"`
	Date           *time.Time        `json:"date"`
	WarrantyExpiry *time.Time        `json:"warranty_expiry"`
	EntityKindHint string            `json:"entity_kind_hint"`
	EntityNameHint string            `json:"entity_name_hint"`
	Maintenance    []MaintenanceHint `json:"maintenance_items"`
	Notes          string            `json:"notes"`
}

// MaintenanceHint is a maintenance schedule item extracted from a document
// (typically an appliance manual).
type MaintenanceHint struct {
	Name           string `json:"name"`
	IntervalMonths int    `json:"interval_months"`
}

// EntityContext provides existing entity names so the LLM can match
// extracted references against known data instead of hallucinating.
type EntityContext struct {
	Vendors    []string
	Projects   []string
	Appliances []string
}

// Document type constants for ExtractionHints.DocumentType.
const (
	DocTypeQuote      = "quote"
	DocTypeInvoice    = "invoice"
	DocTypeReceipt    = "receipt"
	DocTypeManual     = "manual"
	DocTypeWarranty   = "warranty"
	DocTypePermit     = "permit"
	DocTypeInspection = "inspection"
	DocTypeContract   = "contract"
	DocTypeOther      = "other"
)

// ValidDocumentTypes is the set of recognized document type values.
var ValidDocumentTypes = map[string]bool{
	DocTypeQuote:      true,
	DocTypeInvoice:    true,
	DocTypeReceipt:    true,
	DocTypeManual:     true,
	DocTypeWarranty:   true,
	DocTypePermit:     true,
	DocTypeInspection: true,
	DocTypeContract:   true,
	DocTypeOther:      true,
}

// Entity kind hint constants for ExtractionHints.EntityKindHint.
const (
	EntityHintProject     = "project"
	EntityHintAppliance   = "appliance"
	EntityHintVendor      = "vendor"
	EntityHintMaintenance = "maintenance"
	EntityHintQuote       = "quote"
	EntityHintServiceLog  = "service_log"
)

// ValidEntityKindHints is the set of recognized entity kind hint values.
var ValidEntityKindHints = map[string]bool{
	EntityHintProject:     true,
	EntityHintAppliance:   true,
	EntityHintVendor:      true,
	EntityHintMaintenance: true,
	EntityHintQuote:       true,
	EntityHintServiceLog:  true,
}
