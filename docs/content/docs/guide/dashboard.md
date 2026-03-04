+++
title = "Dashboard"
weight = 2
description = "At-a-glance overview of what needs attention."
linkTitle = "Dashboard"
+++

The dashboard shows what needs attention right now.

![Dashboard overlay](/images/dashboard.webp)

## Opening the dashboard

- On launch (if you have a house profile), the dashboard opens automatically
- Press `D` in Nav mode to toggle it on/off
- Press `f` to dismiss it and switch to the next tab

## Sections

### Incidents

Open incidents, ordered by severity (urgent first). Each row shows title,
severity, location, and how long ago it was noticed. This section appears first
so urgent issues are immediately visible.

### Overdue

Maintenance items whose computed next-due date is in the past. Sorted by most
overdue first. Each row shows the item name, linked appliance (if any), how
many days overdue, and last serviced date.

### Upcoming

Maintenance items due within the next 30 days. Same columns as Overdue.

### Active Projects

Projects with status "underway" or "delayed." Shows title, status (color-coded
to match the table), and budget vs. actual cost. Over-budget projects are
highlighted.

### Expiring Soon

Two sources:

- **Appliance warranties** expiring within 90 days (or recently expired within
  30 days)
- **Insurance renewal** if it falls within the same window

Shows item name, expiry date, and days until/since expiry.

### Recent Activity

The last 5 service log entries across all maintenance items. Shows date,
maintenance item name, who performed it (Self or vendor), and cost.

## Navigation

The dashboard supports keyboard navigation:

| Key     | Action |
|---------|--------|
| `j`/`k` | Move cursor down/up through items |
| `g`/`G` | Jump to first/last item |
| `enter` | Jump to the highlighted item's tab and row |
| `D`     | Close dashboard |
| `b`/`f` | Dismiss dashboard, switch tab |
| `?`     | Open help overlay (stacks on top of dashboard) |

When you press `enter`, the dashboard closes and navigates to the
corresponding row in the appropriate tab. For example, pressing `enter` on an
overdue maintenance item takes you to that row in the <a href="/docs/guide/maintenance/" class="tab-pill">Maintenance</a> tab.
