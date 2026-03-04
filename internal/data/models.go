// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"time"

	"gorm.io/gorm"
)

const (
	ProjectStatusIdeating   = "ideating"
	ProjectStatusPlanned    = "planned"
	ProjectStatusQuoted     = "quoted"
	ProjectStatusInProgress = "underway"
	ProjectStatusDelayed    = "delayed"
	ProjectStatusCompleted  = "completed"
	ProjectStatusAbandoned  = "abandoned"
)

const (
	DeletionEntityProject     = "project"
	DeletionEntityQuote       = "quote"
	DeletionEntityMaintenance = "maintenance"
	DeletionEntityAppliance   = "appliance"
	DeletionEntityServiceLog  = "service_log"
	DeletionEntityVendor      = "vendor"
	DeletionEntityDocument    = "document"
	DeletionEntityIncident    = "incident"
)

// Column name constants for use in raw SQL queries. Centralising these
// prevents drift between struct fields and hand-written SQL fragments.
const (
	ColID                = "id"
	ColName              = "name"
	ColCreatedAt         = "created_at"
	ColUpdatedAt         = "updated_at"
	ColDeletedAt         = "deleted_at"
	ColStatus            = "status"
	ColPreviousStatus    = "previous_status"
	ColActualCents       = "actual_cents"
	ColBudgetCents       = "budget_cents"
	ColCostCents         = "cost_cents"
	ColTotalCents        = "total_cents"
	ColIntervalMonths    = "interval_months"
	ColDueDate           = "due_date"
	ColLastServicedAt    = "last_serviced_at"
	ColWarrantyExpiry    = "warranty_expiry"
	ColServicedAt        = "serviced_at"
	ColReceivedDate      = "received_date"
	ColRestoredAt        = "restored_at"
	ColVendorID          = "vendor_id"
	ColProjectID         = "project_id"
	ColProjectTypeID     = "project_type_id"
	ColApplianceID       = "appliance_id"
	ColMaintenanceItemID = "maintenance_item_id"
	ColEntityKind        = "entity_kind"
	ColEntityID          = "entity_id"
	ColEntity            = "entity"
	ColTargetID          = "target_id"
	ColContactName       = "contact_name"
	ColEmail             = "email"
	ColPhone             = "phone"
	ColWebsite           = "website"
	ColNotes             = "notes"
	ColTitle             = "title"
	ColFileName          = "file_name"
	ColMIMEType          = "mime_type"
	ColSizeBytes         = "size_bytes"
	ColChecksum          = "sha256"
	ColData              = "data"
	ColExtractedText     = "extracted_text"
	ColExtractData       = "ocr_data" // DB column name kept for migration compat
	ColSeverity          = "severity"
	ColLocation          = "location"
)

const (
	IncidentStatusOpen       = "open"
	IncidentStatusInProgress = "in_progress"
	IncidentStatusResolved   = "resolved"
)

const (
	IncidentSeverityUrgent   = "urgent"
	IncidentSeveritySoon     = "soon"
	IncidentSeverityWhenever = "whenever"
)

// MaxDocumentSize is the largest file that can be imported as a document
// attachment. SQLite handles arbitrarily large BLOBs, but reading a huge
// file into memory would be a bad experience.
const MaxDocumentSize uint64 = 50 << 20 // 50 MiB

// Document entity kind values for polymorphic linking.
const (
	DocumentEntityNone        = ""
	DocumentEntityProject     = "project"
	DocumentEntityQuote       = "quote"
	DocumentEntityMaintenance = "maintenance"
	DocumentEntityAppliance   = "appliance"
	DocumentEntityServiceLog  = "service_log"
	DocumentEntityVendor      = "vendor"
	DocumentEntityIncident    = "incident"
)

type HouseProfile struct {
	ID               uint `gorm:"primaryKey"`
	Nickname         string
	AddressLine1     string
	AddressLine2     string
	City             string
	State            string
	PostalCode       string
	YearBuilt        int
	SquareFeet       int
	LotSquareFeet    int
	Bedrooms         int
	Bathrooms        float64
	FoundationType   string
	WiringType       string
	RoofType         string
	ExteriorType     string
	HeatingType      string
	CoolingType      string
	WaterSource      string
	SewerType        string
	ParkingType      string
	BasementType     string
	InsuranceCarrier string
	InsurancePolicy  string
	InsuranceRenewal *time.Time
	PropertyTaxCents *int64
	HOAName          string
	HOAFeeCents      *int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ProjectType struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Vendor struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex"`
	ContactName string
	Email       string
	Phone       string
	Website     string
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

type Project struct {
	ID            uint `gorm:"primaryKey"`
	Title         string
	ProjectTypeID uint
	ProjectType   ProjectType `gorm:"constraint:OnDelete:RESTRICT;"`
	Status        string
	Description   string
	StartDate     *time.Time
	EndDate       *time.Time
	BudgetCents   *int64
	ActualCents   *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

type Quote struct {
	ID             uint    `gorm:"primaryKey"`
	ProjectID      uint    `gorm:"index"`
	Project        Project `gorm:"constraint:OnDelete:RESTRICT;"`
	VendorID       uint    `gorm:"index"`
	Vendor         Vendor  `gorm:"constraint:OnDelete:RESTRICT;"`
	TotalCents     int64
	LaborCents     *int64
	MaterialsCents *int64
	OtherCents     *int64
	ReceivedDate   *time.Time
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

type MaintenanceCategory struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Appliance struct {
	ID             uint `gorm:"primaryKey"`
	Name           string
	Brand          string
	ModelNumber    string
	SerialNumber   string
	PurchaseDate   *time.Time
	WarrantyExpiry *time.Time `gorm:"index"`
	Location       string
	CostCents      *int64
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

type MaintenanceItem struct {
	ID             uint `gorm:"primaryKey"`
	Name           string
	CategoryID     uint                `gorm:"index"`
	Category       MaintenanceCategory `gorm:"constraint:OnDelete:RESTRICT;"`
	ApplianceID    *uint               `gorm:"index"`
	Appliance      Appliance           `gorm:"constraint:OnDelete:SET NULL;"`
	LastServicedAt *time.Time
	IntervalMonths int
	DueDate        *time.Time
	ManualURL      string
	ManualText     string
	Notes          string
	CostCents      *int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

type Incident struct {
	ID             uint `gorm:"primaryKey"`
	Title          string
	Description    string
	Status         string
	PreviousStatus string
	Severity       string
	DateNoticed    time.Time
	DateResolved   *time.Time
	Location       string
	CostCents      *int64
	ApplianceID    *uint     `gorm:"index"`
	Appliance      Appliance `gorm:"constraint:OnDelete:SET NULL;"`
	VendorID       *uint     `gorm:"index"`
	Vendor         Vendor    `gorm:"constraint:OnDelete:SET NULL;"`
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

type ServiceLogEntry struct {
	ID                uint            `gorm:"primaryKey"`
	MaintenanceItemID uint            `gorm:"index"`
	MaintenanceItem   MaintenanceItem `gorm:"constraint:OnDelete:CASCADE;"`
	ServicedAt        time.Time
	VendorID          *uint  `gorm:"index"`
	Vendor            Vendor `gorm:"constraint:OnDelete:SET NULL;"`
	CostCents         *int64
	Notes             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

type Document struct {
	ID             uint `gorm:"primaryKey"`
	Title          string
	FileName       string `gorm:"column:file_name"`
	EntityKind     string `gorm:"index:idx_doc_entity"`
	EntityID       uint   `gorm:"index:idx_doc_entity"`
	MIMEType       string
	SizeBytes      int64
	ChecksumSHA256 string `gorm:"column:sha256"`
	Data           []byte
	ExtractedText  string
	ExtractData    []byte `gorm:"column:ocr_data"`
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

type DeletionRecord struct {
	ID         uint       `gorm:"primaryKey"`
	Entity     string     `gorm:"index:idx_entity_restored,priority:1"`
	TargetID   uint       `gorm:"index"`
	DeletedAt  time.Time  `gorm:"index"`
	RestoredAt *time.Time `gorm:"index:idx_entity_restored,priority:2"`
}
