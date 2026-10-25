---
description: "Use when adding a foreign key between soft-deletable entities. Covers the delete and restore guards and the tests they need."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Checklist for adding a new model or FK link between soft-deletable entities.

## Delete guards

The parent must refuse deletion while active (non-deleted) children exist.
Return an actionable error message telling the user to delete the children
first (e.g. "delete its quotes first").

## Restore guards

A child must refuse restore while its parent is deleted. Return an actionable
error message telling the user to restore the parent first (e.g. "restore the
project first").

## Nullable FKs

Nullable means "you don't have to link one", not "the link doesn't matter
once it exists". For nullable FKs, only check guards when the value is
non-nil.

## Required tests

Write composition tests covering the full lifecycle:

- Bottom-up delete succeeds (children first, then parent)
- Wrong-order restore is blocked (child before parent)
- Correct-order restore succeeds (parent first, then child)

Use these existing tests as templates:
- `TestThreeLevelDeleteRestoreChain`
- `TestRestoreMaintenanceBlockedByDeletedAppliance`
- `TestRestoreMaintenanceAllowedWithoutAppliance`
