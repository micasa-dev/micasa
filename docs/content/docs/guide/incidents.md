+++
title = "Incidents"
weight = 6
description = "Log household issues, track severity, and link to appliances or vendors."
linkTitle = "Incidents"
+++

Log and track household issues as they arise.

![Incidents table](/images/incidents.webp)

## Adding an incident

1. Switch to the Incidents tab
2. Enter Edit mode (`i`), press `a`
3. Fill in the form

Only `Title`, `Status`, and `Severity` are required.

## Fields

| Column | Type | Description | Notes |
|-------:|------|-------------|-------|
| `ID` | auto | Auto-assigned | Read-only |
| `Title` | text | Short description | Required |
| `Status` | select | Current state | `open` or `in_progress` |
| `Severity` | select | How urgent | `urgent`, `soon`, or `whenever` |
| `Location` | text | Where in the house | E.g., "Kitchen", "Roof" |
| `Appliance` | link | Related appliance | Optional. Press `enter` to jump to the appliance |
| `Vendor` | link | Assigned vendor | Optional. Press `enter` to jump to the vendor |
| `Noticed` | date | When discovered | YYYY-MM-DD |
| `Resolved` | date | When fixed | YYYY-MM-DD. Only shown on the edit form |
| `Cost` | money | Repair cost | Dollar amount |
| `Docs` | drill | Document count | Press `enter` to view linked documents |

## Resolving incidents

Incidents use soft delete as the resolution mechanism: deleting an incident
marks it resolved. The Incidents tab defaults to showing deleted (resolved)
items so you can see your full history. Resolved incidents appear with
strikethrough styling.

To restore a resolved incident, press `d` on it in Edit mode.

## Dashboard

Open incidents appear in the dashboard's "Open Incidents" section, ordered by
severity (urgent first). Press `enter` on a dashboard row to jump to that
incident in the table.

## Inline editing

All columns except `ID` and `Docs` support inline editing. Press `e` in Edit
mode on a cell to edit just that field. Press `E` from any column to open the
full edit form.
