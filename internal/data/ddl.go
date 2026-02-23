// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import "fmt"

// TableDDL returns the CREATE TABLE SQL for each of the named tables,
// as stored in sqlite_master. Tables that don't exist are silently omitted.
func (s *Store) TableDDL(tables ...string) (map[string]string, error) {
	if len(tables) == 0 {
		return map[string]string{}, nil
	}
	type row struct {
		Name string
		SQL  string `gorm:"column:sql"`
	}
	var rows []row
	err := s.db.Raw(
		`SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name IN ?`,
		tables,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query table ddl: %w", err)
	}
	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.Name] = r.SQL
	}
	return result, nil
}
