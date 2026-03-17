<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Scaled seed data (`--years N`)

## Problem

The `demo` subcommand seeds ~40 entities -- too small to surface real
performance characteristics. Service log entries (the primary growth table)
have only 4-12 rows. Benchmarks and profiling need production-like scale.

## Solution

Add `--years N` flag to the `demo` subcommand. It generates N years of
simulated home ownership instead of the small fixed demo. `demo` alone
keeps current behavior. A summary prints to stderr.

```
micasa demo --years 10 /tmp/perf.db   # persistent, 10 years of data
micasa demo --years 20                 # in-memory, 20 years
micasa demo                            # existing small demo (unchanged)
```

## Entity growth model

| Entity        | Year 0 (base) | Per year after | 10yr total | 20yr total |
|---------------|---------------|----------------|------------|------------|
| Vendors       | 10            | 1-2            | ~25        | ~35        |
| Projects      | 5             | 2-4            | ~35        | ~65        |
| Appliances    | 8             | 0-2            | ~18        | ~28        |
| Maintenance   | 20            | 1-3            | ~40        | ~50 (cap)  |
| Service logs  | 30            | 50-100         | ~700       | ~1500      |
| Quotes        | 8             | 3-8            | ~60        | ~110       |
| Documents     | 6             | 5-10           | ~80        | ~160       |

### Service log spreading

For each maintenance item, compute services/year from `12 / IntervalMonths`.
Space dates evenly across the year with random jitter (~7 days). Skip ~15%
randomly to simulate missed services. Batch insert accumulated logs with
`CreateInBatches`.

### Project status aging

Older projects are weighted toward completed status. The helper
`projectStatusForAge` uses weighted random selection where "completed" weight
increases with age.

## Files

| File | Action |
|------|--------|
| `internal/fake/fake.go` | Add `DateInYear` method |
| `internal/data/seed_scaled.go` | New: `SeedSummary`, `SeedScaledData`, `SeedScaledDataFrom` |
| `internal/data/seed_scaled_test.go` | New: comprehensive tests |
| `cmd/micasa/main.go` | Add `Years` field, validation, branching |
| `plans/seed-at-scale.md` | This file |

## Decisions

- Reuse `SeedDemoDataFrom`'s idempotency pattern (skip if HouseProfile exists)
- Reuse existing `typeID`/`catID` lookup helper pattern
- `SeedScaledData(years)` calls `SeedScaledDataFrom(fake.New(42), years)`
  for deterministic output
- Service logs batch-inserted for performance at scale
- Documents use small placeholder content (no large BLOBs in seed data)
