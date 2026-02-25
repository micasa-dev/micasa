// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"time"

	"gorm.io/gorm"
)

// ListMaintenanceWithSchedule returns all non-deleted maintenance items that
// have a positive interval or an explicit due date, preloading Category and
// Appliance. These are the items eligible for overdue/upcoming computation.
func (s *Store) ListMaintenanceWithSchedule() ([]MaintenanceItem, error) {
	var items []MaintenanceItem
	err := s.db.
		Where(ColIntervalMonths+" > 0 OR "+ColDueDate+" IS NOT NULL").
		Preload("Category").
		Preload("Appliance", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Order(ColUpdatedAt + " desc, " + ColID + " desc").
		Find(&items).Error
	return items, err
}

// ListActiveProjects returns non-deleted projects with status "underway" or
// "delayed", preloading ProjectType.
func (s *Store) ListActiveProjects() ([]Project, error) {
	var projects []Project
	err := s.db.
		Where(ColStatus+" IN ?", []string{ProjectStatusInProgress, ProjectStatusDelayed}).
		Preload("ProjectType").
		Order(ColUpdatedAt + " desc, " + ColID + " desc").
		Find(&projects).Error
	return projects, err
}

// ListOpenIncidents returns non-deleted incidents (open or in-progress),
// preloading Appliance and Vendor. Ordered by severity (urgent first) then
// most recently updated.
func (s *Store) ListOpenIncidents() ([]Incident, error) {
	var incidents []Incident
	err := s.db.
		Where(ColStatus+" IN ?", []string{IncidentStatusOpen, IncidentStatusInProgress}).
		Preload("Appliance", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Preload("Vendor", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Order("CASE " + ColSeverity +
			" WHEN 'urgent' THEN 0" +
			" WHEN 'soon' THEN 1" +
			" WHEN 'whenever' THEN 2" +
			" ELSE 3 END, " + ColUpdatedAt + " desc, " + ColID + " desc").
		Find(&incidents).Error
	return incidents, err
}

// ListExpiringWarranties returns non-deleted appliances whose warranty expires
// between (now - lookBack) and (now + horizon).
func (s *Store) ListExpiringWarranties(
	now time.Time,
	lookBack, horizon time.Duration,
) ([]Appliance, error) {
	var appliances []Appliance
	from := now.Add(-lookBack)
	to := now.Add(horizon)
	err := s.db.
		Where(ColWarrantyExpiry+" IS NOT NULL AND "+ColWarrantyExpiry+" BETWEEN ? AND ?", from, to).
		Order(ColWarrantyExpiry + " asc, " + ColID + " desc").
		Find(&appliances).Error
	return appliances, err
}

// ListRecentServiceLogs returns the most recent service log entries across all
// maintenance items, preloading MaintenanceItem and Vendor.
func (s *Store) ListRecentServiceLogs(limit int) ([]ServiceLogEntry, error) {
	var entries []ServiceLogEntry
	err := s.db.
		Preload("MaintenanceItem", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Preload("Vendor", func(q *gorm.DB) *gorm.DB {
			return q.Unscoped()
		}).
		Order(ColServicedAt + " desc, " + ColID + " desc").
		Limit(limit).
		Find(&entries).Error
	return entries, err
}

// YTDServiceSpendCents returns the total cost of service log entries with
// ServicedAt on or after the given start-of-year.
func (s *Store) YTDServiceSpendCents(yearStart time.Time) (int64, error) {
	var total *int64
	err := s.db.Model(&ServiceLogEntry{}).
		Select("COALESCE(SUM("+ColCostCents+"), 0)").
		Where(ColServicedAt+" >= ?", yearStart).
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// TotalProjectSpendCents returns the total actual spend across all non-deleted
// projects. Unlike service log entries (which have a serviced_at date),
// projects have no per-transaction date, so YTD filtering is not meaningful.
// The previous updated_at filter was incorrect: editing any project field
// (e.g. description) would cause its spend to appear/disappear from the total.
func (s *Store) TotalProjectSpendCents() (int64, error) {
	var total *int64
	err := s.db.Model(&Project{}).
		Select("COALESCE(SUM(" + ColActualCents + "), 0)").
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}
