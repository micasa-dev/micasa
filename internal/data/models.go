// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"fmt"
	"sync"
	"time"

	"github.com/micasa-dev/micasa/internal/uid"

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
	ID               string     `gorm:"primaryKey;size:26" json:"id"`
	Nickname         string     `                          json:"nickname"`
	AddressLine1     string     `                          json:"address_line1"`
	AddressLine2     string     `                          json:"address_line2"`
	City             string     `                          json:"city"`
	State            string     `                          json:"state"`
	PostalCode       string     `                          json:"postal_code"`
	YearBuilt        int        `                          json:"year_built"`
	SquareFeet       int        `                          json:"square_feet"`
	LotSquareFeet    int        `                          json:"lot_square_feet"`
	Bedrooms         int        `                          json:"bedrooms"`
	Bathrooms        float64    `                          json:"bathrooms"`
	FoundationType   string     `                          json:"foundation_type"`
	WiringType       string     `                          json:"wiring_type"`
	RoofType         string     `                          json:"roof_type"`
	ExteriorType     string     `                          json:"exterior_type"`
	HeatingType      string     `                          json:"heating_type"`
	CoolingType      string     `                          json:"cooling_type"`
	WaterSource      string     `                          json:"water_source"`
	SewerType        string     `                          json:"sewer_type"`
	ParkingType      string     `                          json:"parking_type"`
	BasementType     string     `                          json:"basement_type"`
	InsuranceCarrier string     `                          json:"insurance_carrier"`
	InsurancePolicy  string     `                          json:"insurance_policy"`
	InsuranceRenewal *time.Time `                          json:"insurance_renewal"`
	PropertyTaxCents *int64     `                          json:"property_tax_cents"`
	HOAName          string     `                          json:"hoa_name"`
	HOAFeeCents      *int64     `                          json:"hoa_fee_cents"`
	CreatedAt        time.Time  `                          json:"created_at"`
	UpdatedAt        time.Time  `                          json:"updated_at"`
}

type ProjectType struct {
	ID        string    `gorm:"primaryKey;size:26" json:"id"`
	Name      string    `gorm:"uniqueIndex"        json:"name"`
	CreatedAt time.Time `                          json:"created_at"`
	UpdatedAt time.Time `                          json:"updated_at"`
}

type Vendor struct {
	ID          string         `gorm:"primaryKey;size:26"                                                    json:"id"`
	Name        string         `gorm:"uniqueIndex"                                                           json:"name"`
	ContactName string         `                                                                             json:"contact_name"`
	Email       string         `                                                                             json:"email"`
	Phone       string         `                                                                             json:"phone"`
	Website     string         `                                                                             json:"website"`
	Notes       string         `                                                                             json:"notes"`
	Documents   []Document     `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:vendor" json:"-"`
	CreatedAt   time.Time      `                                                                             json:"created_at"`
	UpdatedAt   time.Time      `                                                                             json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"                                                                 json:"-"`
}

type Project struct {
	ID            string         `gorm:"primaryKey;size:26"                                                     json:"id"`
	Title         string         `                                                                              json:"title"`
	ProjectTypeID string         `                                                                              json:"project_type_id"`
	ProjectType   ProjectType    `gorm:"constraint:OnDelete:RESTRICT;"                                          json:"-"`
	Status        string         `                                                                              json:"status"          default:"planned"`
	Description   string         `                                                                              json:"description"`
	StartDate     *time.Time     `                                                                              json:"start_date"                        extract:"-"`
	EndDate       *time.Time     `                                                                              json:"end_date"                          extract:"-"`
	BudgetCents   *int64         `                                                                              json:"budget_cents"`
	ActualCents   *int64         `                                                                              json:"actual_cents"                      extract:"-"`
	Documents     []Document     `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:project" json:"-"`
	CreatedAt     time.Time      `                                                                              json:"created_at"`
	UpdatedAt     time.Time      `                                                                              json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index"                                                                  json:"-"`
}

type Quote struct {
	ID             string         `gorm:"primaryKey;size:26"                                                   json:"id"`
	ProjectID      string         `gorm:"index"                                                                json:"project_id"`
	Project        Project        `gorm:"constraint:OnDelete:RESTRICT;"                                        json:"-"`
	VendorID       string         `gorm:"index"                                                                json:"vendor_id"`
	Vendor         Vendor         `gorm:"constraint:OnDelete:RESTRICT;"                                        json:"-"`
	TotalCents     int64          `                                                                            json:"total_cents"`
	LaborCents     *int64         `                                                                            json:"labor_cents"`
	MaterialsCents *int64         `                                                                            json:"materials_cents"`
	OtherCents     *int64         `                                                                            json:"other_cents"     extract:"-"`
	ReceivedDate   *time.Time     `                                                                            json:"received_date"   extract:"-"`
	Notes          string         `                                                                            json:"notes"`
	Documents      []Document     `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:quote" json:"-"`
	CreatedAt      time.Time      `                                                                            json:"created_at"`
	UpdatedAt      time.Time      `                                                                            json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index"                                                                json:"-"`
}

type MaintenanceCategory struct {
	ID        string    `gorm:"primaryKey;size:26" json:"id"`
	Name      string    `gorm:"uniqueIndex"        json:"name"`
	CreatedAt time.Time `                          json:"created_at"`
	UpdatedAt time.Time `                          json:"updated_at"`
}

type Appliance struct {
	ID             string         `gorm:"primaryKey;size:26"                                                       json:"id"`
	Name           string         `                                                                                json:"name"`
	Brand          string         `                                                                                json:"brand"`
	ModelNumber    string         `                                                                                json:"model_number"`
	SerialNumber   string         `                                                                                json:"serial_number"`
	PurchaseDate   *time.Time     `                                                                                json:"purchase_date"   extract:"-"`
	WarrantyExpiry *time.Time     `gorm:"index"                                                                    json:"warranty_expiry" extract:"-"`
	Location       string         `                                                                                json:"location"`
	CostCents      *int64         `                                                                                json:"cost_cents"`
	Notes          string         `                                                                                json:"notes"`
	Documents      []Document     `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:appliance" json:"-"`
	CreatedAt      time.Time      `                                                                                json:"created_at"`
	UpdatedAt      time.Time      `                                                                                json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index"                                                                    json:"-"`
}

type MaintenanceItem struct {
	ID             string              `gorm:"primaryKey;size:26"                                                         json:"id"`
	Name           string              `                                                                                  json:"name"`
	CategoryID     string              `gorm:"index"                                                                      json:"category_id"`
	Category       MaintenanceCategory `gorm:"constraint:OnDelete:RESTRICT;"                                              json:"-"`
	ApplianceID    *string             `gorm:"index"                                                                      json:"appliance_id"`
	Appliance      Appliance           `gorm:"constraint:OnDelete:SET NULL;"                                              json:"-"`
	Season         string              `                                                                                  json:"season"`
	LastServicedAt *time.Time          `                                                                                  json:"last_serviced_at" extract:"-"`
	IntervalMonths int                 `                                                                                  json:"interval_months"`
	DueDate        *time.Time          `                                                                                  json:"due_date"         extract:"-"`
	ManualURL      string              `                                                                                  json:"manual_url"       extract:"-"`
	ManualText     string              `                                                                                  json:"manual_text"      extract:"-"`
	Notes          string              `                                                                                  json:"notes"`
	CostCents      *int64              `                                                                                  json:"cost_cents"`
	Documents      []Document          `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:maintenance" json:"-"`
	CreatedAt      time.Time           `                                                                                  json:"created_at"`
	UpdatedAt      time.Time           `                                                                                  json:"updated_at"`
	DeletedAt      gorm.DeletedAt      `gorm:"index"                                                                      json:"-"`
}

type Incident struct {
	ID             string         `gorm:"primaryKey;size:26"                                                      json:"id"`
	Title          string         `                                                                               json:"title"`
	Description    string         `                                                                               json:"description"`
	Status         string         `                                                                               json:"status"          default:"open"`
	PreviousStatus string         `                                                                               json:"previous_status"                extract:"-"`
	Severity       string         `                                                                               json:"severity"        default:"soon"`
	DateNoticed    time.Time      `                                                                               json:"date_noticed"    default:"now"`
	DateResolved   *time.Time     `                                                                               json:"date_resolved"                  extract:"-"`
	Location       string         `                                                                               json:"location"`
	CostCents      *int64         `                                                                               json:"cost_cents"`
	ApplianceID    *string        `gorm:"index"                                                                   json:"appliance_id"`
	Appliance      Appliance      `gorm:"constraint:OnDelete:SET NULL;"                                           json:"-"`
	VendorID       *string        `gorm:"index"                                                                   json:"vendor_id"`
	Vendor         Vendor         `gorm:"constraint:OnDelete:SET NULL;"                                           json:"-"`
	Notes          string         `                                                                               json:"notes"`
	Documents      []Document     `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:incident" json:"-"`
	CreatedAt      time.Time      `                                                                               json:"created_at"`
	UpdatedAt      time.Time      `                                                                               json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index"                                                                   json:"-"`
}

type ServiceLogEntry struct {
	ID                string          `gorm:"primaryKey;size:26"                                                         json:"id"`
	MaintenanceItemID string          `gorm:"index"                                                                      json:"maintenance_item_id"`
	MaintenanceItem   MaintenanceItem `gorm:"constraint:OnDelete:CASCADE;"                                               json:"-"`
	ServicedAt        time.Time       `                                                                                  json:"serviced_at"`
	VendorID          *string         `gorm:"index"                                                                      json:"vendor_id"`
	Vendor            Vendor          `gorm:"constraint:OnDelete:SET NULL;"                                              json:"-"`
	CostCents         *int64          `                                                                                  json:"cost_cents"`
	Notes             string          `                                                                                  json:"notes"`
	Documents         []Document      `gorm:"polymorphic:Entity;polymorphicType:EntityKind;polymorphicValue:service_log" json:"-"`
	CreatedAt         time.Time       `                                                                                  json:"created_at"`
	UpdatedAt         time.Time       `                                                                                  json:"updated_at"`
	DeletedAt         gorm.DeletedAt  `gorm:"index"                                                                      json:"-"`
}

type Document struct {
	ID              string         `gorm:"primaryKey;size:26"    json:"id"`
	Title           string         `                             json:"title"`
	FileName        string         `gorm:"column:file_name"      json:"file_name"`
	EntityKind      string         `gorm:"index:idx_doc_entity"  json:"entity_kind"`
	EntityID        string         `gorm:"index:idx_doc_entity"  json:"entity_id"`
	MIMEType        string         `                             json:"mime_type"        extract:"-"`
	SizeBytes       int64          `                             json:"size_bytes"       extract:"-"`
	ChecksumSHA256  string         `gorm:"column:sha256"         json:"sha256"           extract:"-"`
	Data            []byte         `                             json:"-"`
	ExtractedText   string         `                             json:"extracted_text"   extract:"-"`
	ExtractData     []byte         `gorm:"column:ocr_data"       json:"-"`
	ExtractionModel string         `                             json:"extraction_model" extract:"-"`
	ExtractionOps   []byte         `gorm:"column:extraction_ops" json:"-"`
	Notes           string         `                             json:"notes"`
	CreatedAt       time.Time      `                             json:"created_at"`
	UpdatedAt       time.Time      `                             json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index"                 json:"-"`
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
// Ordered by creation time, newest last. Intentionally uses uint PK
// (not ULID) because chat history is local-only and not synced.
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
	RelayURL    string
	LastSeq     int64 `gorm:"default:0"`
	CreatedAt   time.Time
}

func (x *SyncOplogEntry) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *SyncDevice) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *HouseProfile) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *ProjectType) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Vendor) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Project) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Quote) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *MaintenanceCategory) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Appliance) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *MaintenanceItem) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Incident) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *ServiceLogEntry) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *Document) BeforeCreate(_ *gorm.DB) error {
	if x.ID == "" {
		x.ID = uid.New()
	}
	return nil
}

func (x *DeletionRecord) BeforeCreate(_ *gorm.DB) error {
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
