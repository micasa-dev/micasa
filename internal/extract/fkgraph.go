// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package extract

import (
	"fmt"
	"sync"

	"github.com/cpcloud/micasa/internal/data"
	"gorm.io/gorm/schema"
)

// fkGraph encapsulates the FK dependency ordering and ID remapping
// information for shadow DB commits. Both fields are derived together
// from GORM schema introspection, so they cannot drift out of sync.
type fkGraph struct {
	// order is the topologically sorted commit order. Dependencies
	// appear before their dependents.
	order []string
	// remaps maps each creatable table to the FK columns that reference
	// other creatables and need shadow->real ID remapping during commit.
	remaps map[string][]shadowFKRemap
	// entityKindToTable maps document entity_kind (polymorphicValue)
	// to creatable table names, for polymorphic entity_id remapping.
	// Derived from GORM polymorphic HasMany relationships filtered to
	// the creatable set.
	entityKindToTable map[string]string
}

// creatableFKs is the package-level FK graph, computed once at init
// from GORM model metadata. All models are parsed (so cross-table FK
// references resolve), but only tables with Insert=true in
// ExtractionAllowedOps appear in the output.
var creatableFKs = func() fkGraph {
	g, err := buildFKGraph(data.Models(), ExtractionAllowedOps)
	if err != nil {
		panic(fmt.Sprintf("buildFKGraph: %v", err))
	}
	return g
}()

// buildFKGraph parses GORM models and derives FK dependency information.
// All models are parsed so cross-table FK references resolve correctly,
// but only tables present in allowed with Insert=true are included in
// the output graph. For each creatable model it inspects BelongsTo
// relationships; if the referenced model is also creatable, it records
// an fkRemap entry and a dependency edge. The tables are then
// topologically sorted so every dependency is committed before its
// dependents.
//
// Polymorphic HasMany relationships to the documents table are detected
// via GORM schema introspection. Tables with such relationships contribute
// to the entity-kind-to-table mapping, and the documents table is forced
// to depend on all polymorphic targets so entity_id remapping always
// finds the real ID.
func buildFKGraph(models []any, allowed map[string]AllowedOps) (fkGraph, error) {
	namer := schema.NamingStrategy{}
	cacheStore := &sync.Map{}

	// Parse all models so cross-model FK references resolve.
	allSchemas := make(map[string]*schema.Schema, len(models))
	for _, model := range models {
		s, err := schema.Parse(model, cacheStore, namer)
		if err != nil {
			return fkGraph{}, fmt.Errorf("parse %T: %w", model, err)
		}
		allSchemas[s.Table] = s
	}

	// Filter to creatable tables only.
	creatableSet := make(map[string]bool, len(allowed))
	for table, ops := range allowed {
		if ops.Insert {
			creatableSet[table] = true
		}
	}

	remaps := make(map[string][]shadowFKRemap, len(creatableSet))
	// deps[A] = {B, C} means A depends on B and C (B, C must be committed first).
	deps := make(map[string]map[string]bool, len(creatableSet))

	for table := range creatableSet {
		remaps[table] = nil
		deps[table] = make(map[string]bool)
	}

	// Derive FK remaps from BelongsTo relationships between creatables.
	for table := range creatableSet {
		s, ok := allSchemas[table]
		if !ok {
			continue
		}
		for _, rel := range s.Relationships.BelongsTo {
			refTable := rel.FieldSchema.Table
			if !creatableSet[refTable] || refTable == table {
				continue
			}
			for _, ref := range rel.References {
				if ref.OwnPrimaryKey {
					continue
				}
				remaps[table] = append(remaps[table], shadowFKRemap{
					Column: ref.ForeignKey.DBName,
					Table:  refTable,
				})
				deps[table][refTable] = true
			}
		}
	}

	// Discover polymorphic HasMany -> documents relationships from schema
	// metadata. Each owning model's polymorphicValue maps to its table name.
	// The documents table is forced to depend on every polymorphic target
	// so entity_id remapping always finds the real ID.
	ekToTable := make(map[string]string)
	for table := range creatableSet {
		s, ok := allSchemas[table]
		if !ok {
			continue
		}
		for _, rel := range s.Relationships.HasMany {
			if rel.Polymorphic == nil || rel.FieldSchema.Table != data.TableDocuments {
				continue
			}
			if !creatableSet[rel.FieldSchema.Table] {
				continue
			}
			ekToTable[rel.Polymorphic.Value] = table
			deps[data.TableDocuments][table] = true
		}
	}

	order, err := toposort(creatableSet, deps)
	if err != nil {
		return fkGraph{}, err
	}

	return fkGraph{order: order, remaps: remaps, entityKindToTable: ekToTable}, nil
}

// toposort performs a topological sort (Kahn's algorithm) on the given
// tables and dependency edges. Returns an error if a cycle is detected.
func toposort(tables map[string]bool, deps map[string]map[string]bool) ([]string, error) {
	// Compute in-degree (number of dependencies within the creatable set).
	inDeg := make(map[string]int, len(tables))
	// Reverse adjacency: dependents[A] = tables that depend on A.
	dependents := make(map[string][]string, len(tables))

	for table := range tables {
		inDeg[table] = len(deps[table])
		for dep := range deps[table] {
			dependents[dep] = append(dependents[dep], table)
		}
	}

	// Seed the queue with tables that have no dependencies.
	var queue []string
	for table := range tables {
		if inDeg[table] == 0 {
			queue = append(queue, table)
		}
	}

	var order []string
	for len(queue) > 0 {
		// Pop from front.
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)

		for _, dep := range dependents[cur] {
			inDeg[dep]--
			if inDeg[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(tables) {
		return nil, fmt.Errorf("cycle detected in FK dependencies")
	}

	return order, nil
}
