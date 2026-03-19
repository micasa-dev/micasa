// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cpcloud/micasa/internal/crypto"
	"github.com/cpcloud/micasa/internal/sync"
)

type syncStatus int

const (
	syncIdle     syncStatus = iota // not configured
	syncSynced                     // last sync ok
	syncSyncing                    // in progress
	syncOffline                    // last sync failed
	syncConflict                   // ok but conflicts detected
)

// syncConfig holds resolved Pro credentials. Unexported — main.go uses
// the Options.SetSync() setter to pass it in.
type syncConfig struct {
	relayURL    string
	token       string
	householdID string
	key         crypto.HouseholdKey
}

// --- tea.Msg types ---

type syncDoneMsg struct {
	Pulled    int
	Pushed    int
	Conflicts int
	BlobErrs  int
}

type syncErrorMsg struct{ Err error }

type syncTickMsg time.Time

// syncDebounceMsg fires 2s after a local mutation.
// gen is the generation counter at dispatch time; if it doesn't match
// the model's current counter, a newer mutation superseded this one.
type syncDebounceMsg struct{ gen int }

// --- tea.Cmd constructors ---

func doSync(ctx context.Context, engine *sync.Engine) tea.Cmd {
	return func() tea.Msg {
		result, err := engine.Sync(ctx)
		if err != nil {
			return syncErrorMsg{Err: err}
		}
		return syncDoneMsg{
			Pulled:    result.Pulled,
			Pushed:    result.Pushed,
			Conflicts: result.Conflicts,
			BlobErrs:  result.BlobErrs,
		}
	}
}

func syncTick() tea.Cmd {
	return tea.Tick(60*time.Second, func(t time.Time) tea.Msg {
		return syncTickMsg(t)
	})
}

func syncDebounce(gen int) tea.Cmd {
	return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
		return syncDebounceMsg{gen: gen}
	})
}

// syncIndicator renders the status bar glyph.
func (m *Model) syncIndicator() string {
	if m.syncCfg == nil {
		return ""
	}
	switch m.syncStatus {
	case syncSynced:
		return appStyles.SyncSynced().Render("\u25c8")
	case syncSyncing:
		return appStyles.SyncSyncing().Render("\u25c9")
	case syncOffline:
		return appStyles.SyncOffline().Render("\u25cb")
	case syncConflict:
		return appStyles.SyncConflict().Render("!")
	default:
		return ""
	}
}

// withSyncIndicator prepends the sync glyph to the status bar output.
func (m *Model) withSyncIndicator(statusOutput string) string {
	ind := m.zones.Mark("sync-indicator", m.syncIndicator())
	if ind == "" {
		return statusOutput
	}
	return ind + " " + statusOutput
}
