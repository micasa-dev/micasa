// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
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

const (
	SeasonSpring = "spring"
	SeasonSummer = "summer"
	SeasonFall   = "fall"
	SeasonWinter = "winter"
)
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

// EntityKindToTable maps document entity_kind values (polymorphicValue)
// to their corresponding table names. Derived from GORM polymorphic
// tags via schema introspection at init time.
var EntityKindToTable = BuildEntityKindToTable(Models())

// BuildEntityKindToTable derives the entity_kind-to-table mapping from
// GORM polymorphic tags on the given models. Each model with a polymorphic
// HasMany to the documents table contributes one entry:
// polymorphicValue -> owner table name.
func BuildEntityKindToTable(models []any) map[string]string {
	namer := schema.NamingStrategy{}
	cacheStore := &sync.Map{}

	result := make(map[string]string)

	for _, model := range models {
		s, err := schema.Parse(model, cacheStore, namer)
		if err != nil {
			panic(fmt.Sprintf("BuildEntityKindToTable: parse %T: %v", model, err))
		}

		for _, rel := range s.Relationships.HasMany {
			if rel.Polymorphic == nil {
				continue
			}
			if rel.FieldSchema.Table != TableDocuments {
				continue
			}
			result[rel.Polymorphic.Value] = s.Table
		}
	}

	return result
}

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
	Documents   []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:vendor"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

type Project struct {
	ID            uint `gorm:"primaryKey"`
	Title         string
	ProjectTypeID uint
	ProjectType   ProjectType `gorm:"constraint:OnDelete:RESTRICT;"`
	Status        string      `                                                                              default:"planned"`
	Description   string
	StartDate     *time.Time `                                                                                                extract:"-"`
	EndDate       *time.Time `                                                                                                extract:"-"`
	BudgetCents   *int64
	ActualCents   *int64     `                                                                                                extract:"-"`
	Documents     []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:project"`
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
	OtherCents     *int64     `                                                                            extract:"-"`
	ReceivedDate   *time.Time `                                                                            extract:"-"`
	Notes          string
	Documents      []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:quote"`
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
	PurchaseDate   *time.Time `                                                                                extract:"-"`
	WarrantyExpiry *time.Time `gorm:"index"                                                                    extract:"-"`
	Location       string
	CostCents      *int64
	Notes          string
	Documents      []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:appliance"`
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
	Season         string
	LastServicedAt *time.Time `                                                                                  extract:"-"`
	IntervalMonths int
	DueDate        *time.Time `                                                                                  extract:"-"`
	ManualURL      string     `                                                                                  extract:"-"`
	ManualText     string     `                                                                                  extract:"-"`
	Notes          string
	CostCents      *int64
	Documents      []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:maintenance"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

type Incident struct {
	ID             uint `gorm:"primaryKey"`
	Title          string
	Description    string
	Status         string     `                                                                               default:"open"`
	PreviousStatus string     `                                                                                              extract:"-"`
	Severity       string     `                                                                               default:"soon"`
	DateNoticed    time.Time  `                                                                               default:"now"`
	DateResolved   *time.Time `                                                                                              extract:"-"`
	Location       string
	CostCents      *int64
	ApplianceID    *uint     `gorm:"index"`
	Appliance      Appliance `gorm:"constraint:OnDelete:SET NULL;"`
	VendorID       *uint     `gorm:"index"`
	Vendor         Vendor    `gorm:"constraint:OnDelete:SET NULL;"`
	Notes          string
	Documents      []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:incident"`
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
	Documents         []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:service_log"`
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
	MIMEType       string `                            extract:"-"`
	SizeBytes      int64  `                            extract:"-"`
	ChecksumSHA256 string `gorm:"column:sha256"        extract:"-"`
	Data           []byte
	ExtractedText  string `                            extract:"-"`
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

// Setting is a simple key-value store for app preferences that persist
// across sessions (e.g. last-used LLM model). Stored in SQLite so a
// single "micasa backup backup.db" captures everything.
type Setting struct {
	Key       string `gorm:"primaryKey"`
	Value     string
	UpdatedAt time.Time
}

// ChatInput stores a single chat prompt for cross-session history.
// Ordered by creation time, newest last.
type ChatInput struct {
	ID        uint   `gorm:"primaryKey"`
	Input     string `gorm:"not null"`
	CreatedAt time.Time
}
