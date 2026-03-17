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

	"github.com/cpcloud/micasa/internal/crypto"
	"github.com/cpcloud/micasa/internal/data"
	"github.com/cpcloud/micasa/internal/sync"
	"github.com/dustin/go-humanize"
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
	resp, err := client.CreateHousehold(sync.CreateHouseholdRequest{
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
	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)
	status, err := client.Status()
	if err != nil {
		return fmt.Errorf("fetch status: %w", err)
	}

	fmt.Printf("household: %s\n", status.HouseholdID)
	fmt.Printf("devices:   %d\n", len(status.Devices))
	fmt.Printf("ops:       %d\n", status.OpsCount)
	fmt.Printf("last seq:  %d\n", deps.device.LastSeq)
	fmt.Printf("storage:   %s / %s\n",
		humanize.IBytes(uint64(status.BlobStorage.UsedBytes)),
		humanize.IBytes(uint64(status.BlobStorage.QuotaBytes)),
	)
	if status.StripeStatus != "" {
		fmt.Printf("plan:      %s\n", status.StripeStatus)
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
	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)
	status, err := client.Status()
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

// formatStorageUsage formats used/quota bytes as "X / Y (Z%)".
func formatStorageUsage(used, quota int64) string {
	var pct float64
	if quota > 0 {
		pct = float64(used) / float64(quota) * 100
	}
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
	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewClient(deps.device.RelayURL, deps.token, deps.key)

	pulled, err := pullAll(deps.store, client, deps.device.LastSeq)
	if err != nil {
		return err
	}

	pushed, pushedOps, err := pushAll(deps.store, client)
	if err != nil {
		return err
	}

	// Upload blobs for pushed document ops.
	uploaded, uploadErrs := uploadPendingBlobs(
		deps.store,
		client,
		deps.device.HouseholdID,
		pushedOps,
	)

	// Fetch blobs for documents that arrived without data.
	fetched, fetchErrs := fetchPendingBlobs(deps.store, client, deps.device.HouseholdID)

	fmt.Fprintf(os.Stderr, "pulled %d ops, pushed %d ops\n", pulled, pushed)
	if uploaded > 0 {
		fmt.Fprintf(os.Stderr, "uploaded %d blobs\n", uploaded)
	}
	if fetched > 0 {
		fmt.Fprintf(os.Stderr, "fetched %d blobs\n", fetched)
	}
	if blobErrs := uploadErrs + fetchErrs; blobErrs > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d blob operation(s) failed\n", blobErrs)
	}
	return nil
}

func pullAll(
	store *data.Store,
	client *sync.Client,
	lastSeq int64,
) (int, error) {
	total := 0
	seq := lastSeq
	for {
		result, err := client.Pull(seq, 100)
		if err != nil {
			return total, fmt.Errorf("pull: %w", err)
		}
		if len(result.Ops) == 0 {
			break
		}

		ar := sync.ApplyOps(store.GormDB(), result.Ops)
		if len(ar.Errors) > 0 {
			return total, fmt.Errorf("apply ops: %w", ar.Errors[0])
		}
		total += ar.Applied + ar.Conflicts

		// Update last seq from the highest envelope seq.
		// The relay returns ops ordered by seq (ascending), so the max
		// is always the last element, but we scan defensively.
		for _, dop := range result.Ops {
			if dop.Envelope.Seq > seq {
				seq = dop.Envelope.Seq
			}
		}
		if err := store.UpdateSyncDevice(map[string]any{
			"last_seq": seq,
		}); err != nil {
			return total, fmt.Errorf("update last seq: %w", err)
		}

		if !result.HasMore {
			break
		}
	}
	return total, nil
}

func pushAll(store *data.Store, client *sync.Client) (int, []sync.OpPayload, error) {
	unsynced, err := store.UnsyncedOps()
	if err != nil {
		return 0, nil, fmt.Errorf("load unsynced ops: %w", err)
	}
	if len(unsynced) == 0 {
		return 0, nil, nil
	}

	ops := make([]sync.OpPayload, 0, len(unsynced))
	for _, entry := range unsynced {
		ops = append(ops, sync.OpPayload{
			ID:        entry.ID,
			TableName: entry.TableName,
			RowID:     entry.RowID,
			OpType:    entry.OpType,
			Payload:   entry.Payload,
			DeviceID:  entry.DeviceID,
			CreatedAt: entry.CreatedAt,
		})
	}

	pushResp, err := client.Push(ops)
	if err != nil {
		return 0, nil, fmt.Errorf("push: %w", err)
	}

	ids := make([]string, 0, len(pushResp.Confirmed))
	for _, c := range pushResp.Confirmed {
		ids = append(ids, c.ID)
	}
	if err := store.MarkSynced(ids); err != nil {
		return 0, nil, fmt.Errorf("mark synced: %w", err)
	}

	return len(pushResp.Confirmed), ops, nil
}

// --- blob sync helpers ---

// uploadPendingBlobs uploads blobs for document ops that were just pushed.
// Returns the number of successful uploads and the number of errors.
func uploadPendingBlobs(
	store *data.Store,
	client *sync.Client,
	householdID string,
	ops []sync.OpPayload,
) (int, int) {
	uploaded, errCount := 0, 0
	for _, op := range ops {
		if op.TableName != data.TableDocuments {
			continue
		}
		// Extract blob_ref from the payload JSON.
		var payload map[string]any
		if err := json.Unmarshal([]byte(op.Payload), &payload); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unmarshal payload for %s: %v\n", op.RowID, err)
			errCount++
			continue
		}
		blobRef, _ := payload["blob_ref"].(string)
		if blobRef == "" {
			continue
		}

		// Skip if already on relay.
		exists, err := client.HasBlob(householdID, blobRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: check blob %s: %v\n", blobRef, err)
			errCount++
			continue
		}
		if exists {
			continue
		}

		// Load full document to get Data.
		doc, err := store.GetDocument(op.RowID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: load document %s: %v\n", op.RowID, err)
			errCount++
			continue
		}
		if doc.Data == nil {
			continue // metadata-only or pending
		}

		if err := client.UploadBlob(householdID, blobRef, doc.Data); err != nil {
			fmt.Fprintf(os.Stderr, "warning: upload blob %s: %v\n", blobRef, err)
			errCount++
			continue
		}
		uploaded++
	}
	return uploaded, errCount
}

// fetchPendingBlobs downloads blobs for documents that have a checksum
// but no local data (arrived via sync without the blob).
// Returns the number of successful fetches and the number of errors.
func fetchPendingBlobs(
	store *data.Store,
	client *sync.Client,
	householdID string,
) (int, int) {
	pending, err := store.PendingBlobDocuments()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: query pending blobs: %v\n", err)
		return 0, 1
	}

	fetched, errCount := 0, 0
	for _, doc := range pending {
		plaintext, err := client.DownloadBlob(householdID, doc.ChecksumSHA256)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: download blob %s: %v\n", doc.ChecksumSHA256, err)
			errCount++
			continue
		}
		if err := store.UpdateDocumentData(doc.ID, plaintext); err != nil {
			fmt.Fprintf(os.Stderr, "warning: save blob %s: %v\n", doc.ID, err)
			errCount++
			continue
		}
		fetched++
	}
	return fetched, errCount
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
	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)

	invite, err := client.Invite(deps.device.HouseholdID)
	if err != nil {
		return fmt.Errorf("create invite: %w", err)
	}

	// Compound code: HOUSEHOLD_ID.CODE
	compoundCode := deps.device.HouseholdID + "." + invite.Code
	fmt.Printf("%s\n", compoundCode)
	fmt.Fprintf(
		os.Stderr,
		"expires: %s\n",
		invite.ExpiresAt.Format(time.RFC3339),
	)
	fmt.Fprintf(os.Stderr, "waiting for joiner...\n")

	// Poll for pending key exchanges, cancellable via Ctrl+C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

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
			exchanges, err := client.GetPendingExchanges(deps.device.HouseholdID)
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

	if err := client.CompleteKeyExchange(exchange.ID, sealed); err != nil {
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
	dotIdx := strings.IndexByte(code, '.')
	if dotIdx < 0 {
		return fmt.Errorf(
			"invalid invite code format -- expected HOUSEHOLD_ID.CODE (got %q)",
			code,
		)
	}
	householdID := code[:dotIdx]
	inviteCode := code[dotIdx+1:]
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
	joinResp, err := client.Join(householdID, sync.JoinRequest{
		InviteCode: inviteCode,
		DeviceName: hostname,
		PublicKey:  kp.PublicKey[:],
	})
	if err != nil {
		return fmt.Errorf("join household: %w", err)
	}

	fmt.Fprintf(os.Stderr, "waiting for inviter to approve...\n")

	// Poll for key exchange result, cancellable via Ctrl+C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

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
			r, err := client.GetKeyExchangeResult(joinResp.ExchangeID)
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

	// Initial pull.
	syncClient := sync.NewClient(relayURL, result.DeviceToken, key)
	pulled, err := pullAll(store, syncClient, 0)
	if err != nil {
		return fmt.Errorf("initial pull: %w", err)
	}

	fmt.Fprintf(os.Stderr, "joined household %s\n", householdID)
	fmt.Fprintf(os.Stderr, "device: %s\n", result.DeviceID)
	if pulled > 0 {
		fmt.Fprintf(os.Stderr, "pulled %d ops\n", pulled)
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
	deps, err := resolveProDeps(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = deps.store.Close() }()

	client := sync.NewManagementClient(deps.device.RelayURL, deps.token)
	devices, err := client.ListDevices(deps.device.HouseholdID)
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(devices)
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
	return tw.Flush()
}
