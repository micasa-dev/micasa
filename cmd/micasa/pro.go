// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/micasa-dev/micasa/internal/crypto"
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/spf13/cobra"
)

const (
	defaultRelayURL = "https://relay.micasa.dev"
	pollInterval    = 2 * time.Second
	pollTimeout     = 5 * time.Minute
)

func newProCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pro",
		Short: "Manage micasa Pro sync",
		Long: `Encrypted multi-device sync for your household data.

Typical workflow:
  1. First device:  micasa pro init
  2. First device:  micasa pro invite    (prints a one-time code)
  3. Second device: micasa pro join <code>
  4. Either device: micasa pro sync      (push and pull changes)`,
		Example: `  micasa pro init
  micasa pro invite
  micasa pro join 01JQ7X2K.abc123
  micasa pro sync
  micasa pro status`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.AddCommand(
		newProInitCmd(),
		newProStatusCmd(),
		newProStorageCmd(),
		newProSyncCmd(),
		newProInviteCmd(),
		newProJoinCmd(),
		newProDevicesCmd(),
		newProConflictsCmd(),
	)
	return cmd
}

// proDeps holds resolved dependencies shared by most pro subcommands.
type proDeps struct {
	store     *data.Store
	device    data.SyncDevice
	secretDir string
	token     string
	key       crypto.HouseholdKey
	kp        crypto.DeviceKeyPair
}

func resolveProDeps(dbPath string) (*proDeps, error) {
	store, err := openAndMigrate(dbPath)
	if err != nil {
		return nil, err
	}

	// Close the store on any error path; disable on successful return.
	closeOnErr := true
	defer func() {
		if closeOnErr {
			_ = store.Close()
		}
	}()

	dev, err := store.GetSyncDevice()
	if err != nil {
		if errors.Is(err, data.ErrNoSyncDevice) {
			return nil, fmt.Errorf("sync not set up -- run `micasa pro init` first")
		}
		return nil, fmt.Errorf("read sync state: %w", err)
	}
	if dev.HouseholdID == "" {
		return nil, fmt.Errorf("sync not set up -- run `micasa pro init` first")
	}

	secretDir, err := crypto.SecretsDir()
	if err != nil {
		return nil, fmt.Errorf("resolve secrets directory: %w", err)
	}

	token, err := crypto.LoadDeviceToken(secretDir)
	if err != nil {
		return nil, fmt.Errorf("load device token: %w", err)
	}

	key, err := crypto.LoadHouseholdKey(secretDir)
	if err != nil {
		return nil, fmt.Errorf("load household key: %w", err)
	}

	kp, err := crypto.LoadDeviceKeyPair(secretDir)
	if err != nil {
		return nil, fmt.Errorf("load device keypair: %w", err)
	}

	closeOnErr = false
	return &proDeps{
		store:     store,
		device:    dev,
		secretDir: secretDir,
		token:     token,
		key:       key,
		kp:        kp,
	}, nil
}

func openAndMigrate(dbPath string) (*data.Store, error) {
	resolved, err := resolveDBPathArg(dbPath)
	if err != nil {
		return nil, err
	}
	store, err := data.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := store.AutoMigrate(); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return store, nil
}

// openExisting opens a database that must already exist. Returns a clear
// error if the file is missing instead of silently creating one.
func openExisting(dbPath string) (*data.Store, error) {
	resolved, err := resolveDBPathArg(dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(resolved); err != nil {
		return nil, fmt.Errorf("database not found: %s", resolved)
	}
	return openAndMigrate(resolved)
}

func resolveDBPathArg(dbPath string) (string, error) {
	if dbPath != "" {
		return data.ExpandHome(dbPath), nil
	}
	return data.DefaultDBPath()
}

// dbPathFromEnvOrArg returns the database path from a positional arg
// (if provided) or the MICASA_DB_PATH env var.
func dbPathFromEnvOrArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return os.Getenv("MICASA_DB_PATH")
}

// resolveRelayURL returns the relay URL respecting flag > env > default
// precedence. flagChanged is true when the user explicitly passed --relay-url.
func resolveRelayURL(flagValue string, flagChanged bool) string {
	if flagChanged {
		return flagValue
	}
	if envURL := os.Getenv("MICASA_RELAY_URL"); envURL != "" {
		return envURL
	}
	return flagValue // still holds the default
}

// --- pro init ---

func newProInitCmd() *cobra.Command {
	var relayURL string

	cmd := &cobra.Command{
		Use:           "init [database-path]",
		Short:         "Bootstrap: create household, generate keys, register device",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			url := resolveRelayURL(relayURL, cmd.Flags().Changed("relay-url"))
			return runProInit(dbPathFromEnvOrArg(args), url)
		},
	}
	cmd.Flags().StringVar(
		&relayURL, "relay-url", defaultRelayURL,
		"Relay server URL (honors MICASA_RELAY_URL)",
	)
	return cmd
}

func runProInit(dbPath, relayURL string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	store, err := openAndMigrate(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	// Guard: error if already initialized.
	dev, err := store.GetSyncDevice()
	if err == nil && dev.HouseholdID != "" {
		return fmt.Errorf(
			"already initialized (household %s) -- to reinitialize, "+
				"delete the secrets directory and reset the sync device",
			dev.HouseholdID,
		)
	}

	secretDir, err := crypto.SecretsDir()
	if err != nil {
		return fmt.Errorf("resolve secrets directory: %w", err)
	}

	// Generate device keypair + household key.
	kp, err := crypto.GenerateDeviceKeyPair()
	if err != nil {
		return fmt.Errorf("generate device keypair: %w", err)
	}
	key, err := crypto.GenerateHouseholdKey()
	if err != nil {
		return fmt.Errorf("generate household key: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	// Register with relay.
	client := sync.NewManagementClient(relayURL, "")
	resp, err := client.CreateHousehold(ctx, sync.CreateHouseholdRequest{
		DeviceName: hostname,
		PublicKey:  kp.PublicKey[:],
	})
	if err != nil {
		return fmt.Errorf("register with relay: %w", err)
	}

	// Save secrets.
	if err := crypto.SaveDeviceKeyPair(secretDir, kp); err != nil {
		return fmt.Errorf("save device keypair: %w", err)
	}
	if err := crypto.SaveHouseholdKey(secretDir, key); err != nil {
		return fmt.Errorf("save household key: %w", err)
	}
	if err := crypto.SaveDeviceToken(secretDir, resp.DeviceToken); err != nil {
		return fmt.Errorf("save device token: %w", err)
	}

	// Update oplog entries with old device ID before switching.
	oldDeviceID := store.DeviceID()
	if oldDeviceID != "" && oldDeviceID != resp.DeviceID {
		if err := store.UpdateOplogDeviceIDs(oldDeviceID, resp.DeviceID); err != nil {
			return fmt.Errorf("update oplog device IDs: %w", err)
		}
	}

	// Update cached device ID before DB write so any hooks during
	// UpdateSyncDevice see the new ID.
	store.SetDeviceID(resp.DeviceID)

	// Update SyncDevice record.
	if err := store.UpdateSyncDevice(map[string]any{
		"id":           resp.DeviceID,
		"household_id": resp.HouseholdID,
		"relay_url":    relayURL,
	}); err != nil {
		return fmt.Errorf("update sync device: %w", err)
	}

	fmt.Fprintf(os.Stderr, "household: %s\n", resp.HouseholdID)
	fmt.Fprintf(os.Stderr, "device:    %s\n", resp.DeviceID)
	fmt.Fprintf(os.Stderr, "secrets:   %s\n", secretDir)
	return nil
}

// --- pro status ---

func newProStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "status [database-path]",
		Short:         "Show sync status",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runProStatus(dbPathFromEnvOrArg(args))
		},
	}
}

func runProStatus(dbPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)
	status, err := client.Status(ctx)
	if err != nil {
		return fmt.Errorf("fetch status: %w", err)
	}

	fmt.Printf("household: %s\n", status.HouseholdID)
	fmt.Printf("devices:   %d\n", len(status.Devices))
	fmt.Printf("ops:       %d\n", status.OpsCount)
	fmt.Printf("last seq:  %d\n", deps.device.LastSeq)
	fmt.Printf(
		"storage:   %s\n",
		formatStorageUsage(status.BlobStorage.UsedBytes, status.BlobStorage.QuotaBytes),
	)
	if status.StripeStatus != nil && *status.StripeStatus != "" {
		fmt.Printf("plan:      %s\n", *status.StripeStatus)
	}

	// Show unsynced local ops count.
	unsynced, err := deps.store.UnsyncedOps()
	if err != nil {
		return fmt.Errorf("count unsynced ops: %w", err)
	}
	if len(unsynced) > 0 {
		fmt.Printf("unsynced:  %d local ops pending push\n", len(unsynced))
	}
	return nil
}

// --- pro storage ---

func newProStorageCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "storage [database-path]",
		Short:         "Show blob storage usage",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runProStorage(dbPathFromEnvOrArg(args))
		},
	}
}

func runProStorage(dbPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)
	status, err := client.Status(ctx)
	if err != nil {
		return fmt.Errorf("fetch status: %w", err)
	}

	fmt.Println(formatStorageUsage(
		status.BlobStorage.UsedBytes,
		status.BlobStorage.QuotaBytes,
	))
	return nil
}

// formatBytes formats a byte count as a human-readable string using
// binary units (KiB, MiB, GiB).
func formatBytes(b int64) string {
	if b < 0 {
		return "0 B"
	}
	return humanize.IBytes(uint64(b))
}

// formatStorageUsage formats used/quota bytes. When quota is 0
// (unlimited), returns just the used amount.
func formatStorageUsage(used, quota int64) string {
	if quota <= 0 {
		return formatBytes(used)
	}
	pct := float64(used) / float64(quota) * 100
	return fmt.Sprintf("%s / %s (%.1f%%)", formatBytes(used), formatBytes(quota), pct)
}

// --- pro sync ---

func newProSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "sync [database-path]",
		Short:         "Force immediate push+pull cycle",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runProSync(dbPathFromEnvOrArg(args))
		},
	}
}

func runProSync(dbPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewClient(deps.device.RelayURL, deps.token, deps.key)
	engine := sync.NewEngine(deps.store, client, deps.device.HouseholdID)

	result, err := engine.Sync(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "pulled %d ops, pushed %d ops\n", result.Pulled, result.Pushed)
	if result.BlobsUp > 0 {
		fmt.Fprintf(os.Stderr, "uploaded %d blobs\n", result.BlobsUp)
	}
	if result.BlobsDown > 0 {
		fmt.Fprintf(os.Stderr, "fetched %d blobs\n", result.BlobsDown)
	}
	if result.BlobErrs > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d blob operation(s) failed\n", result.BlobErrs)
	}
	return nil
}

// --- pro invite ---

func newProInviteCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "invite [database-path]",
		Short:         "Generate invite code, wait for joiner handshake",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runProInvite(dbPathFromEnvOrArg(args))
		},
	}
}

func runProInvite(dbPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)

	invite, err := client.Invite(ctx, deps.device.HouseholdID)
	if err != nil {
		return fmt.Errorf("create invite: %w", err)
	}

	// Compound code: HOUSEHOLD_ID.CODE
	compoundCode := deps.device.HouseholdID + "." + invite.Code
	fmt.Printf("%s\n", compoundCode)

	remaining := time.Until(invite.ExpiresAt).Truncate(time.Minute)
	h := int(remaining.Hours())
	m := int(remaining.Minutes()) % 60
	var dur string
	switch {
	case h > 0 && m > 0:
		dur = fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		dur = fmt.Sprintf("%dh", h)
	default:
		dur = fmt.Sprintf("%dm", m)
	}
	fmt.Fprintf(os.Stderr,
		"on the other device, run: micasa pro join %s\n"+
			"code expires in %s (at %s)\n"+
			"waiting for joiner...\n",
		compoundCode,
		dur,
		invite.ExpiresAt.Local().Format("3:04 PM"),
	)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	deadline := time.After(pollTimeout)
	var exchange sync.PendingKeyExchange
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("interrupted")
		case <-deadline:
			return fmt.Errorf("timed out waiting for joiner (5 minutes)")
		case <-ticker.C:
			exchanges, err := client.GetPendingExchanges(ctx, deps.device.HouseholdID)
			if err != nil {
				return fmt.Errorf("poll pending exchanges: %w", err)
			}
			if len(exchanges) > 0 {
				exchange = exchanges[0]
				goto found
			}
		}
	}
found:

	// Encrypt household key for the joiner.
	if len(exchange.JoinerPublicKey) != crypto.KeySize {
		return fmt.Errorf("joiner public key has wrong size: got %d, want %d", len(exchange.JoinerPublicKey), crypto.KeySize)
	}
	var joinerPubKey [crypto.KeySize]byte
	copy(joinerPubKey[:], exchange.JoinerPublicKey)

	sealed, err := crypto.BoxSeal(
		deps.kp.PrivateKey,
		joinerPubKey,
		deps.key[:],
	)
	if err != nil {
		return fmt.Errorf("encrypt household key for joiner: %w", err)
	}

	if err := client.CompleteKeyExchange(ctx, exchange.ID, sealed); err != nil {
		return fmt.Errorf("complete key exchange: %w", err)
	}

	fmt.Fprintf(os.Stderr, "device joined: %s\n", exchange.JoinerName)
	return nil
}

// --- pro join ---

func newProJoinCmd() *cobra.Command {
	var relayURL string

	cmd := &cobra.Command{
		Use:           "join <code> [database-path]",
		Short:         "Join household with invite code",
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			} else {
				dbPath = os.Getenv("MICASA_DB_PATH")
			}
			url := resolveRelayURL(relayURL, cmd.Flags().Changed("relay-url"))
			return runProJoin(args[0], dbPath, url)
		},
	}
	cmd.Flags().StringVar(
		&relayURL, "relay-url", defaultRelayURL,
		"Relay server URL (honors MICASA_RELAY_URL)",
	)
	return cmd
}

func runProJoin(code, dbPath, relayURL string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	store, err := openAndMigrate(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	// Guard: not already joined.
	dev, err := store.GetSyncDevice()
	if err == nil && dev.HouseholdID != "" {
		return fmt.Errorf(
			"already in household %s -- cannot join another",
			dev.HouseholdID,
		)
	}

	// Parse compound code: HOUSEHOLD_ID.CODE (split on first dot).
	householdID, inviteCode, ok := strings.Cut(code, ".")
	if !ok {
		return fmt.Errorf(
			"invalid invite code format -- expected HOUSEHOLD_ID.CODE (got %q)",
			code,
		)
	}
	if householdID == "" || inviteCode == "" {
		return fmt.Errorf(
			"invalid invite code format -- " +
				"both household ID and code must be non-empty",
		)
	}

	secretDir, err := crypto.SecretsDir()
	if err != nil {
		return fmt.Errorf("resolve secrets directory: %w", err)
	}

	// Generate device keypair.
	kp, err := crypto.GenerateDeviceKeyPair()
	if err != nil {
		return fmt.Errorf("generate device keypair: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	// Join household.
	client := sync.NewManagementClient(relayURL, "")
	joinResp, err := client.Join(ctx, householdID, sync.JoinRequest{
		InviteCode: inviteCode,
		DeviceName: hostname,
		PublicKey:  kp.PublicKey[:],
	})
	if err != nil {
		return fmt.Errorf("join household: %w", err)
	}

	fmt.Fprintf(os.Stderr, "waiting for inviter to approve...\n")

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	timeoutCh := time.After(pollTimeout)
	var result *sync.KeyExchangeResult
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("interrupted")
		case <-timeoutCh:
			return fmt.Errorf(
				"timed out waiting for inviter approval (5 minutes)",
			)
		case <-ticker.C:
			r, err := client.GetKeyExchangeResult(ctx, joinResp.ExchangeID)
			if err != nil {
				return fmt.Errorf("poll key exchange: %w", err)
			}
			if r.Ready {
				result = r
				goto exchangeDone
			}
		}
	}
exchangeDone:

	// Decrypt household key.
	if len(joinResp.InviterPublicKey) != crypto.KeySize {
		return fmt.Errorf("inviter public key has wrong size: got %d, want %d", len(joinResp.InviterPublicKey), crypto.KeySize)
	}
	var inviterPubKey [crypto.KeySize]byte
	copy(inviterPubKey[:], joinResp.InviterPublicKey)

	keyBytes, err := crypto.BoxOpen(
		kp.PrivateKey,
		inviterPubKey,
		result.EncryptedHouseholdKey,
	)
	if err != nil {
		return fmt.Errorf("decrypt household key: %w", err)
	}

	var key crypto.HouseholdKey
	if len(keyBytes) != crypto.KeySize {
		return fmt.Errorf(
			"decrypted key has wrong size: expected %d, got %d",
			crypto.KeySize,
			len(keyBytes),
		)
	}
	copy(key[:], keyBytes)

	// Save secrets.
	if err := crypto.SaveDeviceKeyPair(secretDir, kp); err != nil {
		return fmt.Errorf("save device keypair: %w", err)
	}
	if err := crypto.SaveHouseholdKey(secretDir, key); err != nil {
		return fmt.Errorf("save household key: %w", err)
	}
	if err := crypto.SaveDeviceToken(secretDir, result.DeviceToken); err != nil {
		return fmt.Errorf("save device token: %w", err)
	}

	// Update oplog entries with old device ID.
	oldDeviceID := store.DeviceID()
	if oldDeviceID != "" && oldDeviceID != result.DeviceID {
		if err := store.UpdateOplogDeviceIDs(
			oldDeviceID,
			result.DeviceID,
		); err != nil {
			return fmt.Errorf("update oplog device IDs: %w", err)
		}
	}

	// Update cached device ID before DB write so any hooks during
	// UpdateSyncDevice see the new ID.
	store.SetDeviceID(result.DeviceID)

	// Update SyncDevice record.
	if err := store.UpdateSyncDevice(map[string]any{
		"id":           result.DeviceID,
		"household_id": householdID,
		"relay_url":    relayURL,
	}); err != nil {
		return fmt.Errorf("update sync device: %w", err)
	}

	// Initial pull using the sync engine (pull-only: nothing to push on a fresh join).
	syncClient := sync.NewClient(relayURL, result.DeviceToken, key)
	engine := sync.NewEngine(store, syncClient, householdID)
	syncResult, err := engine.Sync(ctx)
	if err != nil {
		return fmt.Errorf("initial pull: %w", err)
	}

	fmt.Fprintf(os.Stderr, "joined household %s\n", householdID)
	fmt.Fprintf(os.Stderr, "device: %s\n", result.DeviceID)
	if syncResult.Pulled > 0 {
		fmt.Fprintf(os.Stderr, "pulled %d ops\n", syncResult.Pulled)
	}
	return nil
}

// --- pro devices ---

func newProDevicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "devices [database-path]",
		Short:         "List devices",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			return runProDevicesList(dbPathFromEnvOrArg(args))
		},
	}
	cmd.AddCommand(newProDevicesRevokeCmd())
	return cmd
}

func runProDevicesList(dbPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)
	devices, err := client.ListDevices(ctx, deps.device.HouseholdID)
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(devices); err != nil {
		return fmt.Errorf("encode devices: %w", err)
	}
	return nil
}

func newProDevicesRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "revoke <device-id> [database-path]",
		Short:         "Revoke a device",
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			} else {
				dbPath = os.Getenv("MICASA_DB_PATH")
			}
			return runProDevicesRevoke(args[0], dbPath)
		},
	}
}

func runProDevicesRevoke(deviceID, dbPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	if deviceID == deps.device.ID {
		return fmt.Errorf("cannot revoke your own device")
	}

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)
	if err := client.RevokeDevice(
		ctx,
		deps.device.HouseholdID,
		deviceID,
	); err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}

	fmt.Fprintf(os.Stderr, "revoked device %s\n", deviceID)
	return nil
}

// --- pro conflicts ---

func newProConflictsCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "conflicts [database-path]",
		Short:         "List sync ops that lost LWW conflict resolution",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProConflicts(cmd.OutOrStdout(), dbPathFromEnvOrArg(args))
		},
	}
}

func runProConflicts(w io.Writer, dbPath string) error {
	store, err := openExisting(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	losers, err := store.ConflictLosers()
	if err != nil {
		return fmt.Errorf("query conflict losers: %w", err)
	}
	if len(losers) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintf(tw, "ID\tTABLE\tROW ID\tOP\tDEVICE\tCREATED\n"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for _, op := range losers {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			op.ID,
			op.TableName,
			op.RowID,
			op.OpType,
			op.DeviceID,
			op.CreatedAt.Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("write conflict: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush output: %w", err)
	}
	return nil
}
