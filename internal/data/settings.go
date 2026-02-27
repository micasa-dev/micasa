// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"errors"
	"fmt"
	"time"

	"github.com/cpcloud/micasa/internal/locale"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

const (
	settingLLMModel          = "llm.model"
	settingShowDashboard     = "ui.show_dashboard"
	settingUnitSystem        = "ui.unit_system"
	settingTesseractHintSeen = "hint.tesseract_shown"
	settingCurrency          = "locale.currency"

	// chatHistoryMax is the maximum number of chat inputs retained.
	chatHistoryMax = 200
)

// GetSetting retrieves a setting by key. Returns ("", nil) if not found.
func (s *Store) GetSetting(key string) (string, error) {
	var setting Setting
	err := s.db.Where("key = ?", key).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return setting.Value, nil
}

// PutSetting upserts a setting.
func (s *Store) PutSetting(key, value string) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&Setting{Key: key, Value: value, UpdatedAt: time.Now()}).Error
}

// GetLastModel returns the persisted LLM model name, or "" if none.
func (s *Store) GetLastModel() (string, error) {
	return s.GetSetting(settingLLMModel)
}

// PutLastModel persists the LLM model name.
func (s *Store) PutLastModel(model string) error {
	return s.PutSetting(settingLLMModel, model)
}

// GetShowDashboard returns whether the dashboard should be shown on
// startup. Defaults to true when no preference has been saved.
func (s *Store) GetShowDashboard() (bool, error) {
	val, err := s.GetSetting(settingShowDashboard)
	if err != nil {
		return true, err
	}
	if val == "" {
		return true, nil
	}
	return val == "true", nil
}

// PutShowDashboard persists the user's dashboard visibility preference.
func (s *Store) PutShowDashboard(show bool) error {
	val := "false"
	if show {
		val = "true"
	}
	return s.PutSetting(settingShowDashboard, val)
}

// GetUnitSystem returns the persisted unit system preference, falling
// back to locale-based detection if no preference has been saved.
func (s *Store) GetUnitSystem() (UnitSystem, error) {
	val, err := s.GetSetting(settingUnitSystem)
	if err != nil {
		return UnitSystemForLocale(locale.DetectLocale()), err
	}
	if val == "" {
		return UnitSystemForLocale(locale.DetectLocale()), nil
	}
	return ParseUnitSystem(val), nil
}

// PutUnitSystem persists the user's unit system preference.
func (s *Store) PutUnitSystem(u UnitSystem) error {
	return s.PutSetting(settingUnitSystem, u.String())
}

// TesseractHintSeen returns whether the one-time "install tesseract" hint
// has already been shown to the user.
func (s *Store) TesseractHintSeen() bool {
	val, err := s.GetSetting(settingTesseractHintSeen)
	return err == nil && val == "true"
}

// MarkTesseractHintSeen records that the tesseract hint was shown.
func (s *Store) MarkTesseractHintSeen() error {
	return s.PutSetting(settingTesseractHintSeen, "true")
}

// GetCurrency returns the persisted currency code (e.g. "USD", "EUR"),
// or "" if none has been set.
func (s *Store) GetCurrency() (string, error) {
	return s.GetSetting(settingCurrency)
}

// PutCurrency persists the currency code to the database.
func (s *Store) PutCurrency(code string) error {
	return s.PutSetting(settingCurrency, code)
}

// AppendChatInput adds a prompt to the persistent history, deduplicating
// consecutive repeats. Trims old entries beyond chatHistoryMax.
func (s *Store) AppendChatInput(input string) error {
	// Deduplicate: skip if the most recent entry matches.
	var last ChatInput
	if err := s.db.Order("id DESC").First(&last).Error; err == nil {
		if last.Input == input {
			return nil
		}
	}

	if err := s.db.Create(&ChatInput{Input: input}).Error; err != nil {
		return err
	}

	// Trim old entries.
	var count int64
	if err := s.db.Model(&ChatInput{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count chat inputs: %w", err)
	}
	if count > chatHistoryMax {
		excess := count - chatHistoryMax
		if err := s.db.Exec(
			"DELETE FROM chat_inputs WHERE id IN (SELECT id FROM chat_inputs ORDER BY id ASC LIMIT ?)",
			excess,
		).Error; err != nil {
			return fmt.Errorf("trim chat inputs: %w", err)
		}
	}
	return nil
}

// LoadChatHistory returns all persisted chat inputs, oldest first.
func (s *Store) LoadChatHistory() ([]string, error) {
	var entries []ChatInput
	if err := s.db.Order("id ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.Input
	}
	return result, nil
}
