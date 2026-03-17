// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"sync"
	"time"

	"github.com/cpcloud/micasa/internal/uid"

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
	ID               string `gorm:"primaryKey;size:26"`
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
	ID        string `gorm:"primaryKey;size:26"`
	Name      string `gorm:"uniqueIndex"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Vendor struct {
	ID          string `gorm:"primaryKey;size:26"`
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
	ID            string `gorm:"primaryKey;size:26"`
	Title         string
	ProjectTypeID string
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
	ID             string  `gorm:"primaryKey;size:26"`
	ProjectID      string  `gorm:"index"`
	Project        Project `gorm:"constraint:OnDelete:RESTRICT;"`
	VendorID       string  `gorm:"index"`
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
	ID        string `gorm:"primaryKey;size:26"`
	Name      string `gorm:"uniqueIndex"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Appliance struct {
	ID             string `gorm:"primaryKey;size:26"`
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
	ID             string `gorm:"primaryKey;size:26"`
	Name           string
	CategoryID     string              `gorm:"index"`
	Category       MaintenanceCategory `gorm:"constraint:OnDelete:RESTRICT;"`
	ApplianceID    *string             `gorm:"index"`
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
	ID             string `gorm:"primaryKey;size:26"`
	Title          string
	Description    string
	Status         string     `                                                                               default:"open"`
	PreviousStatus string     `                                                                                              extract:"-"`
	Severity       string     `                                                                               default:"soon"`
	DateNoticed    time.Time  `                                                                               default:"now"`
	DateResolved   *time.Time `                                                                                              extract:"-"`
	Location       string
	CostCents      *int64
	ApplianceID    *string   `gorm:"index"`
	Appliance      Appliance `gorm:"constraint:OnDelete:SET NULL;"`
	VendorID       *string   `gorm:"index"`
	Vendor         Vendor    `gorm:"constraint:OnDelete:SET NULL;"`
	Notes          string
	Documents      []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:incident"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

type ServiceLogEntry struct {
	ID                string          `gorm:"primaryKey;size:26"`
	MaintenanceItemID string          `gorm:"index"`
	MaintenanceItem   MaintenanceItem `gorm:"constraint:OnDelete:CASCADE;"`
	ServicedAt        time.Time
	VendorID          *string `gorm:"index"`
	Vendor            Vendor  `gorm:"constraint:OnDelete:SET NULL;"`
	CostCents         *int64
	Notes             string
	Documents         []Document `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:service_log"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
}

type Document struct {
	ID              string `gorm:"primaryKey;size:26"`
	Title           string
	FileName        string `gorm:"column:file_name"`
	EntityKind      string `gorm:"index:idx_doc_entity"`
	EntityID        string `gorm:"index:idx_doc_entity"`
	MIMEType        string `                             extract:"-"`
	SizeBytes       int64  `                             extract:"-"`
	ChecksumSHA256  string `gorm:"column:sha256"         extract:"-"`
	Data            []byte
	ExtractedText   string `                             extract:"-"`
	ExtractData     []byte `gorm:"column:ocr_data"`
	ExtractionModel string `                             extract:"-"`
	ExtractionOps   []byte `gorm:"column:extraction_ops" extract:"-"`
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

type DeletionRecord struct {
	ID         string     `gorm:"primaryKey;size:26"`
	Entity     string     `gorm:"index:idx_entity_restored,priority:1"`
	TargetID   string     `gorm:"index"`
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

// deletionEntityToTable maps DeletionEntity constants to table names.
// Used by the oplog to write "restore" entries from restoreSoftDeleted.
var deletionEntityToTable = map[string]string{
	DeletionEntityProject:     TableProjects,
	DeletionEntityQuote:       TableQuotes,
	DeletionEntityMaintenance: TableMaintenanceItems,
	DeletionEntityAppliance:   TableAppliances,
	DeletionEntityServiceLog:  TableServiceLogEntries,
	DeletionEntityVendor:      TableVendors,
	DeletionEntityDocument:    TableDocuments,
	DeletionEntityIncident:    TableIncidents,
}

// Oplog operation types.
const (
	OpInsert  = "insert"
	OpUpdate  = "update"
	OpDelete  = "delete"
	OpRestore = "restore"
)

// SyncOplogEntry records a single mutation to a syncable entity.
// Local mutations have applied_at set immediately; synced_at is set after
// successful push to the relay. Remote ops have both set on receipt.
type SyncOplogEntry struct {
	ID        string `gorm:"primaryKey;size:26"`
	TableName string `gorm:"not null;index:idx_oplog_table_row"`
	RowID     string `gorm:"not null;index:idx_oplog_table_row"`
	OpType    string `gorm:"not null"`
	Payload   string `gorm:"type:text;not null"`
	DeviceID  string `gorm:"not null"`
	CreatedAt time.Time
	AppliedAt *time.Time
	SyncedAt  *time.Time `gorm:"index"`
}

// SyncDevice stores this device's identity. Only one row exists locally.
type SyncDevice struct {
	ID          string `gorm:"primaryKey;size:26"`
	Name        string `gorm:"not null"`
	HouseholdID string
	LastSeq     int64 `gorm:"default:0"`
	CreatedAt   time.Time
}

func (x *SyncOplogEntry) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *SyncDevice) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *HouseProfile) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *ProjectType) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Vendor) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Project) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Quote) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *MaintenanceCategory) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Appliance) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *MaintenanceItem) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Incident) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *ServiceLogEntry) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Document) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *DeletionRecord) BeforeCreate(tx *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

// --- Oplog hooks: AfterCreate only ---
// AfterCreate hooks work reliably because tx.Create(&item) always passes a
// populated model. Updates and deletes use explicit oplog writes in Store
// methods because GORM's Model().Updates() and Where().Delete() pass empty
// models to hooks, preventing correct payload capture.

func (x *HouseProfile) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableHouseProfiles, x.ID, OpInsert, x)
}

func (x *ProjectType) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableProjectTypes, x.ID, OpInsert, x)
}

func (x *Vendor) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableVendors, x.ID, OpInsert, x)
}

func (x *Project) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableProjects, x.ID, OpInsert, x)
}

func (x *Quote) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableQuotes, x.ID, OpInsert, x)
}

func (x *MaintenanceCategory) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableMaintenanceCategories, x.ID, OpInsert, x)
}

func (x *Appliance) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableAppliances, x.ID, OpInsert, x)
}

func (x *MaintenanceItem) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableMaintenanceItems, x.ID, OpInsert, x)
}

func (x *Incident) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableIncidents, x.ID, OpInsert, x)
}

func (x *ServiceLogEntry) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableServiceLogEntries, x.ID, OpInsert, x)
}

func (x *Document) AfterCreate(tx *gorm.DB) error {
	if isSyncApplying(tx) {
		return nil
	}
	return writeOplogEntry(tx, TableDocuments, x.ID, OpInsert, newDocumentOplogPayload(*x))
}
