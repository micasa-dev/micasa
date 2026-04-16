// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
)

type statusOpts struct {
	asJSON bool
	days   int
	isDark bool
}

const (
	statusDaysMin = 1
	statusDaysMax = 365
)

func (o *statusOpts) validate() error {
	if o.days < statusDaysMin || o.days > statusDaysMax {
		return fmt.Errorf(
			"--days must be between %d and %d, got %d",
			statusDaysMin, statusDaysMax, o.days,
		)
	}
	return nil
}

func newStatusCmd() *cobra.Command {
	opts := &statusOpts{}

	cmd := &cobra.Command{
		Use:   "status [database-path]",
		Short: "Show overdue items, open incidents, and active projects",
		Long: `Print items that need attention and exit with code 2 if any
are found. Exit 0 means everything is on track. Useful for cron jobs,
shell prompts, and status bar widgets.`,
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.validate(); err != nil {
				return err
			}
			opts.isDark = lipgloss.HasDarkBackground(os.Stdin, os.Stderr)
			store, err := openExisting(dbPathFromEnvOrArg(args))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()
			return runStatus(cmd.OutOrStdout(), opts, store, time.Now())
		},
	}

	cmd.Flags().BoolVar(&opts.asJSON, "json", false,
		"Output JSON instead of human-readable text")
	cmd.Flags().IntVar(&opts.days, "days", 30,
		"Look-ahead window for upcoming items (1-365)")

	return cmd
}

func runStatus(
	w io.Writer,
	opts *statusOpts,
	store *data.Store,
	now time.Time,
) error {
	maintenance, err := store.ListMaintenanceWithSchedule()
	if err != nil {
		return fmt.Errorf("list maintenance: %w", err)
	}

	var overdue, upcoming []maintenanceStatus
	for _, m := range maintenance {
		nextDue := data.ComputeNextDue(m.LastServicedAt, m.IntervalMonths, m.DueDate)
		if nextDue == nil {
			continue
		}
		days := data.DateDiffDays(now, *nextDue)
		entry := maintenanceStatus{
			ID:        m.ID,
			Name:      m.Name,
			Category:  m.Category.Name,
			Appliance: m.Appliance.Name,
			NextDue:   *nextDue,
			Days:      days,
		}
		if days < 0 {
			entry.Days = -days
			overdue = append(overdue, entry)
		} else if days <= opts.days {
			upcoming = append(upcoming, entry)
		}
	}

	// Sort overdue by most-overdue first, upcoming by soonest first.
	sort.Slice(overdue, func(i, j int) bool {
		return overdue[i].Days > overdue[j].Days
	})
	sort.Slice(upcoming, func(i, j int) bool {
		return upcoming[i].Days < upcoming[j].Days
	})

	incidents, err := store.ListOpenIncidents()
	if err != nil {
		return fmt.Errorf("list incidents: %w", err)
	}

	projects, err := store.ListActiveProjects()
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	needsAttention := len(overdue) > 0 ||
		len(incidents) > 0 ||
		hasDelayedProject(projects)

	if opts.asJSON {
		if err := writeStatusJSON(w, overdue, upcoming, incidents, projects, needsAttention); err != nil {
			return err
		}
	} else {
		if err := writeStatusText(w, newCLIStyles(opts.isDark), overdue, upcoming, incidents, projects, now); err != nil {
			return err
		}
	}

	if needsAttention {
		return exitError{code: 2}
	}
	return nil
}

type maintenanceStatus struct {
	ID        string
	Name      string
	Category  string
	Appliance string
	NextDue   time.Time
	Days      int
}

func hasDelayedProject(projects []data.Project) bool {
	for _, p := range projects {
		if p.Status == data.ProjectStatusDelayed {
			return true
		}
	}
	return false
}

// --- text output ---

func writeStatusText(
	w io.Writer,
	styles cliStyles,
	overdue, upcoming []maintenanceStatus,
	incidents []data.Incident,
	projects []data.Project,
	now time.Time,
) error {
	wrote := false
	if len(overdue) > 0 {
		if err := writeOverdueText(w, styles, overdue); err != nil {
			return err
		}
		wrote = true
	}
	if len(upcoming) > 0 {
		if wrote {
			if _, err := fmt.Fprintln(w); err != nil {
				return fmt.Errorf("write section separator: %w", err)
			}
		}
		if err := writeUpcomingText(w, styles, upcoming); err != nil {
			return err
		}
		wrote = true
	}
	if len(incidents) > 0 {
		if wrote {
			if _, err := fmt.Fprintln(w); err != nil {
				return fmt.Errorf("write section separator: %w", err)
			}
		}
		if err := writeIncidentsText(w, styles, incidents, now); err != nil {
			return err
		}
		wrote = true
	}
	if len(projects) > 0 {
		if wrote {
			if _, err := fmt.Fprintln(w); err != nil {
				return fmt.Errorf("write section separator: %w", err)
			}
		}
		if err := writeProjectsText(w, styles, projects, now); err != nil {
			return err
		}
	}
	return nil
}

func writeOverdueText(w io.Writer, styles cliStyles, items []maintenanceStatus) error {
	_ = styles
	if _, err := fmt.Fprintln(w, "=== OVERDUE ==="); err != nil {
		return fmt.Errorf("write overdue header: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tOVERDUE"); err != nil {
		return fmt.Errorf("write overdue columns: %w", err)
	}
	for _, m := range items {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", m.Name, data.DaysText(m.Days)); err != nil {
			return fmt.Errorf("write overdue row: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush overdue table: %w", err)
	}
	return nil
}

func writeUpcomingText(w io.Writer, styles cliStyles, items []maintenanceStatus) error {
	_ = styles
	if _, err := fmt.Fprintln(w, "=== UPCOMING ==="); err != nil {
		return fmt.Errorf("write upcoming header: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tDUE"); err != nil {
		return fmt.Errorf("write upcoming columns: %w", err)
	}
	for _, m := range items {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", m.Name, data.DaysText(m.Days)); err != nil {
			return fmt.Errorf("write upcoming row: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush upcoming table: %w", err)
	}
	return nil
}

func writeIncidentsText(
	w io.Writer,
	styles cliStyles,
	incidents []data.Incident,
	now time.Time,
) error {
	_ = styles
	if _, err := fmt.Fprintln(w, "=== INCIDENTS ==="); err != nil {
		return fmt.Errorf("write incidents header: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "TITLE\tSEVERITY\tREPORTED"); err != nil {
		return fmt.Errorf("write incidents columns: %w", err)
	}
	for _, inc := range incidents {
		days := data.DateDiffDays(now, inc.DateNoticed)
		if days < 0 {
			days = -days
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", inc.Title, inc.Severity, data.DaysText(days)); err != nil {
			return fmt.Errorf("write incidents row: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush incidents table: %w", err)
	}
	return nil
}

func writeProjectsText(
	w io.Writer,
	styles cliStyles,
	projects []data.Project,
	now time.Time,
) error {
	_ = styles
	if _, err := fmt.Fprintln(w, "=== ACTIVE PROJECTS ==="); err != nil {
		return fmt.Errorf("write projects header: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "TITLE\tSTATUS\tSTARTED"); err != nil {
		return fmt.Errorf("write projects columns: %w", err)
	}
	for _, p := range projects {
		started := "-"
		if p.StartDate != nil {
			days := data.DateDiffDays(now, *p.StartDate)
			if days < 0 {
				days = -days
			}
			started = data.DaysText(days)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Title, p.Status, started); err != nil {
			return fmt.Errorf("write projects row: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush projects table: %w", err)
	}
	return nil
}

// --- JSON output ---

func writeStatusJSON(
	w io.Writer,
	overdue, upcoming []maintenanceStatus,
	incidents []data.Incident,
	projects []data.Project,
	needsAttention bool,
) error {
	result := statusJSON{
		Overdue:        make([]overdueJSON, 0, len(overdue)),
		Upcoming:       make([]upcomingJSON, 0, len(upcoming)),
		Incidents:      make([]incidentJSON, 0, len(incidents)),
		ActiveProjects: make([]projectJSON, 0, len(projects)),
		NeedsAttention: needsAttention,
	}

	for _, m := range overdue {
		result.Overdue = append(result.Overdue, overdueJSON{
			ID:          m.ID,
			Name:        m.Name,
			Category:    m.Category,
			Appliance:   m.Appliance,
			NextDue:     m.NextDue.Format("2006-01-02"),
			DaysOverdue: m.Days,
		})
	}
	for _, m := range upcoming {
		result.Upcoming = append(result.Upcoming, upcomingJSON{
			ID:           m.ID,
			Name:         m.Name,
			Category:     m.Category,
			Appliance:    m.Appliance,
			NextDue:      m.NextDue.Format("2006-01-02"),
			DaysUntilDue: m.Days,
		})
	}
	for _, inc := range incidents {
		result.Incidents = append(result.Incidents, incidentJSON{
			ID:          inc.ID,
			Title:       inc.Title,
			Status:      inc.Status,
			Severity:    inc.Severity,
			DateNoticed: inc.DateNoticed.Format("2006-01-02"),
		})
	}
	for _, p := range projects {
		pj := projectJSON{
			ID:     p.ID,
			Title:  p.Title,
			Status: p.Status,
		}
		if p.StartDate != nil {
			pj.StartDate = p.StartDate.Format("2006-01-02")
		}
		result.ActiveProjects = append(result.ActiveProjects, pj)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode status JSON: %w", err)
	}
	return nil
}

type statusJSON struct {
	Overdue        []overdueJSON  `json:"overdue"`
	Upcoming       []upcomingJSON `json:"upcoming"`
	Incidents      []incidentJSON `json:"incidents"`
	ActiveProjects []projectJSON  `json:"active_projects"`
	NeedsAttention bool           `json:"needs_attention"`
}

type overdueJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Appliance   string `json:"appliance"`
	NextDue     string `json:"next_due"`
	DaysOverdue int    `json:"days_overdue"`
}

type upcomingJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	Appliance    string `json:"appliance"`
	NextDue      string `json:"next_due"`
	DaysUntilDue int    `json:"days_until_due"`
}

type incidentJSON struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	DateNoticed string `json:"date_noticed"`
}

type projectJSON struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	StartDate string `json:"start_date,omitempty"`
}
