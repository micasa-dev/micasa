+++
title = "Projects"
weight = 3
description = "Track home improvement projects from idea to completion."
linkTitle = "Projects"
+++

Track planned and in-progress work on your house.

![Projects table](/images/projects.webp)

## Adding a project

1. Switch to the Projects tab (`f` to cycle forward)
2. Enter Edit mode (`i`)
3. Press `a` to open the add form
4. Fill in the fields and save (`ctrl+s`)

The `Title` field is required. Everything else is optional or has a default.

## Fields

| Column | Type | Description | Notes |
|-------:|------|-------------|-------|
| `ID` | auto | Auto-assigned primary key | Read-only |
| `Type` | select | Project category | Pre-seeded types (Renovation, Repair, etc.) |
| `Title` | text | Project name | Required |
| `Status` | select | Lifecycle stage | See [status lifecycle](#status-lifecycle) below |
| `Budget` | money | Planned cost | Dollar amount (e.g., 1250.00) |
| `Actual` | money | Real cost | Over-budget is highlighted on the dashboard |
| `Start` | date | Start date | [Date input]({{< ref "/docs/using/date-input" >}}) |
| `End` | date | End date | [Date input]({{< ref "/docs/using/date-input" >}}) |
| `Quotes` | drill | Number of linked quotes | Press `enter` to view linked quotes |
| `Docs` | drill | Number of linked documents | Press `enter` to view linked documents |

## Status lifecycle

Projects move through these statuses. Each has a distinct color in the table:

- <span class="status-ideating">**ideating**</span> -- just an idea, not committed
- <span class="status-planned">**planned**</span> -- decided to do it, working out details
- <span class="status-quoted">**quoted**</span> -- have vendor quotes, comparing options
- <span class="status-underway">**underway**</span> -- work in progress
- <span class="status-delayed">**delayed**</span> -- stalled for some reason
- <span class="status-completed">**completed**</span> -- done
- <span class="status-abandoned">**abandoned**</span> -- decided not to do it

## Settled filter

In Nav mode on the Projects tab, press `t` to toggle hiding **settled
projects** (`completed` + `abandoned`). A `◀` triangle appears to the right of
the tab when the filter is active.

## Description

The edit form includes a `Description` textarea (in the "Timeline" group) for
longer notes about the project. The description is stored on the project record
but doesn't appear as a table column.

## Inline editing

In Edit mode, press `e` on any non-`ID` column to edit just that cell inline.
Press `e` on the `ID` column (or any read-only column) to open the full edit
form, which includes the description field. Press `E` from any column to jump
straight to the full form.

## Linked quotes

The `Quotes` column shows how many quotes are linked to this project. In
Nav mode, press `enter` to drill into a detail view of those quotes.

On the Quotes tab, the `Project` column links back -- press `enter` to jump
to the project.
