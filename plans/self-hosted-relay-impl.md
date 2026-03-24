<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Self-Hosted Relay Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development
> (if subagents available) or superpowers:executing-plans to implement this plan.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the relay self-hostable via Docker Compose with configurable
blob quota and subscription bypass, so the cloud version is a managed instance
of the same binary.

**Architecture:** Add `selfHosted` and `blobQuota` fields to the Handler,
plumb quota through the `PutBlob` interface, wire env var parsing in
`cmd/relay/main.go`, and create Docker deployment artifacts. Existing behavior
(cloud mode) is preserved as the default.

**Tech Stack:** Go 1.26, Docker, Docker Compose, Caddy, PostgreSQL 17

**Spec:** `plans/self-hosted-relay.md`

---

## File Structure

### Create

- `deploy/relay/Dockerfile` -- multi-stage Go build for relay binary
- `deploy/docker-compose.yml` -- base stack (postgres + relay)
- `deploy/docker-compose.caddy.yml` -- TLS override with Caddy
- `deploy/Caddyfile` -- reverse proxy template
- `deploy/.env.example` -- example environment file

### Modify

- `internal/relay/store.go` -- add `quota int64` param to `PutBlob`
- `internal/relay/blob.go` -- export `DefaultBlobQuota`
- `internal/relay/memstore.go` -- update `PutBlob` signature, remove
  `SetBlobQuota`/`blobQuotaBytes`
- `internal/relay/pgstore.go` -- update `PutBlob` signature, use quota param
- `internal/relay/handler.go` -- add `selfHosted`, `blobQuota` fields +
  options, update `requireSubscription`, `handlePutBlob`, `handleStatus`
- `internal/relay/handler_test.go` -- update quota tests, add self-hosted tests
- `internal/relay/concurrent_test.go` -- update quota test
- `internal/sync/types.go` -- doc comment on `BlobStorage.QuotaBytes`
- `cmd/relay/main.go` -- env var parsing for `SELF_HOSTED`, `BLOB_QUOTA_BYTES`
- `cmd/micasa/pro.go` -- adaptive `formatStorageUsage`

---

## Codebase conventions for relay handler tests

Tests in `internal/relay/handler_test.go` are in `package relay` (not
`relay_test`) and follow these patterns:

- Use `httptest.NewRecorder()` + `h.ServeHTTP(rec, req)` (NOT
  `httptest.NewServer`)
- `createTestHousehold(t, h)` returns `sync.CreateHouseholdResponse`
  with fields `.HouseholdID`, `.DeviceID`, `.DeviceToken`
- `authRequest(method, path, body, token)` returns `*http.Request` with
  Bearer auth header set
- `blobURL(householdID, hash)` returns the blob endpoint path
- `testHash` is a pre-defined valid SHA-256 constant
- `StatusResponse` must be qualified as `sync.StatusResponse`

---

## Task 1: Self-hosted mode with configurable quota

Plumb quota through `PutBlob` interface, add handler options for
self-hosted mode and blob quota, update `requireSubscription`,
`handlePutBlob`, and `handleStatus`. This is one atomic task because the
Store interface change, handler options, and test updates are
interdependent.

**Files:**
- Modify: `internal/relay/store.go:122-125`
- Modify: `internal/relay/blob.go:18-19`
- Modify: `internal/relay/memstore.go:42,87-99,545-568`
- Modify: `internal/relay/pgstore.go:666-704`
- Modify: `internal/relay/handler.go:24-89,129-147,430-443,545-554`
- Modify: `internal/relay/handler_test.go`
- Modify: `internal/relay/concurrent_test.go:97-102`

- [ ] **Step 1: Write failing tests for self-hosted mode**

Add to `internal/relay/handler_test.go`. Also add `"context"` to the
import block if not present.

```go
func TestSelfHostedSubscriptionBypass(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithSelfHosted())
	hh := createTestHousehold(t, h)

	// Set subscription to canceled — cloud mode would return 402.
	require.NoError(t, store.UpdateSubscription(
		context.Background(),
		hh.HouseholdID,
		"sub_test",
		"canceled",
	))

	// In self-hosted mode, push should succeed despite canceled subscription.
	// Empty ops returns 400, not 402 — proves subscription check was bypassed.
	rec := httptest.NewRecorder()
	req := authRequest("POST", "/sync/push", sync.PushRequest{}, hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSelfHostedUnlimitedQuota(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithSelfHosted())
	hh := createTestHousehold(t, h)

	// Status should report quota=0 (unlimited) in self-hosted mode.
	rec := httptest.NewRecorder()
	req := authRequest("GET", "/status", nil, hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var status sync.StatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	assert.Equal(t, int64(0), status.BlobStorage.QuotaBytes)
}

func TestCloudModeDefaultQuota(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default())
	hh := createTestHousehold(t, h)

	rec := httptest.NewRecorder()
	req := authRequest("GET", "/status", nil, hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var status sync.StatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	assert.Equal(t, DefaultBlobQuota, status.BlobStorage.QuotaBytes)
}

func TestCustomBlobQuota(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithBlobQuota(500))
	hh := createTestHousehold(t, h)

	// Upload a 200-byte blob — should succeed (200 < 500).
	payload := make([]byte, 200)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, testHash), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// Upload another 400-byte blob — would exceed 500 quota.
	// Use a different hash (sha256 of 400 zero bytes).
	payload2 := make([]byte, 400)
	hash2 := fmt.Sprintf("%x", sha256.Sum256(payload2))
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, hash2), bytes.NewReader(payload2))
	req2.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec2.Code)

	// Verify the 413 response reports the custom quota, not DefaultBlobQuota.
	var body map[string]any
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&body))
	assert.Equal(t, float64(500), body["quota_bytes"])
}
```

Add `"context"`, `"crypto/sha256"`, and `"fmt"` to the import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -shuffle=on -run 'TestSelfHosted|TestCloudModeDefaultQuota|TestCustomBlobQuota' ./internal/relay/`
Expected: FAIL (undefined symbols: `WithSelfHosted`, `WithBlobQuota`, `DefaultBlobQuota`)

- [ ] **Step 3: Export DefaultBlobQuota**

In `internal/relay/blob.go`, rename the constant:

```go
// DefaultBlobQuota is the included blob storage per household (1 GB).
// Used as the cloud-mode default when no WithBlobQuota option is set.
const DefaultBlobQuota int64 = 1 << 30
```

Update all references from `defaultBlobQuota` to `DefaultBlobQuota` in
`handler.go` and `pgstore.go` (memstore references removed in step 5).

- [ ] **Step 4: Update Store interface**

In `internal/relay/store.go`, change the `PutBlob` signature:

```go
// PutBlob stores an encrypted blob for a household, keyed by SHA-256
// hash. Returns errBlobExists if the hash already exists (dedup),
// errQuotaExceeded if the household's blob storage would exceed quota.
// A quota of 0 disables quota enforcement.
PutBlob(ctx context.Context, householdID, hash string, data []byte, quota int64) error
```

- [ ] **Step 5: Update MemStore.PutBlob**

In `internal/relay/memstore.go`:

1. Remove `blobQuota int64` field from `MemStore` struct (line 42) and
   its comment.
2. Remove `SetBlobQuota` method (lines 87-92).
3. Remove `blobQuotaBytes` method (lines 94-99).
4. Update `PutBlob` signature:

```go
func (m *MemStore) PutBlob(_ context.Context, householdID, hash string, data []byte, quota int64) error {
```

5. Replace the quota check:

```go
	// When quota is 0, skip enforcement (unlimited).
	if quota > 0 && used > quota-int64(len(data)) {
		return errQuotaExceeded
	}
```

- [ ] **Step 6: Update PgStore.PutBlob**

In `internal/relay/pgstore.go`, update `PutBlob` signature:

```go
func (s *PgStore) PutBlob(ctx context.Context, householdID, hash string, data []byte, quota int64) error {
```

Replace the hardcoded quota check (line 689):

```go
		if quota > 0 && used > quota-int64(len(data)) {
			return errQuotaExceeded
		}
```

- [ ] **Step 7: Add Handler fields and options**

In `internal/relay/handler.go`, update the Handler struct:

```go
type Handler struct {
	store        Store
	mux          *http.ServeMux
	log          *slog.Logger
	webhookSecret string
	selfHosted   bool
	blobQuota    int64
	blobQuotaSet bool
}
```

The `blobQuotaSet` field distinguishes "not configured" from an explicit
`WithBlobQuota(0)`. Add handler options after `WithWebhookSecret`:

```go
// WithSelfHosted enables self-hosted mode: subscription checks are
// bypassed and the default blob quota is unlimited (0).
func WithSelfHosted() HandlerOption {
	return func(h *Handler) { h.selfHosted = true }
}

// WithBlobQuota sets the per-household blob storage quota in bytes.
// A value of 0 disables quota enforcement (unlimited).
func WithBlobQuota(n int64) HandlerOption {
	return func(h *Handler) {
		h.blobQuota = n
		h.blobQuotaSet = true
	}
}
```

In `NewHandler`, after applying options, set the default quota if not
explicitly configured:

```go
func NewHandler(store Store, log *slog.Logger, opts ...HandlerOption) *Handler {
	h := &Handler{store: store, log: log}
	for _, opt := range opts {
		opt(h)
	}
	// Default blob quota when not explicitly set:
	// unlimited (0) in self-hosted mode, 1 GB in cloud mode.
	if !h.blobQuotaSet && !h.selfHosted {
		h.blobQuota = DefaultBlobQuota
	}
	if h.webhookSecret == "" {
		log.Warn("Stripe webhook secret is empty -- webhook signature verification disabled")
	}
	// ... rest of mux setup unchanged
```

- [ ] **Step 8: Update requireSubscription**

In `internal/relay/handler.go`, modify `requireSubscription`:

```go
func (h *Handler) requireSubscription(next authenticatedHandler) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, dev sync.Device) {
		if h.selfHosted {
			next(w, r, dev)
			return
		}
		hh, err := h.store.GetHousehold(r.Context(), dev.HouseholdID)
		if err != nil {
			h.log.Error("get household for subscription check", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if hh.StripeStatus != "" && hh.StripeStatus != sync.SubscriptionActive {
			writeError(w, http.StatusPaymentRequired, "subscription inactive")
			return
		}
		next(w, r, dev)
	}
}
```

- [ ] **Step 9: Update handlePutBlob**

Replace `DefaultBlobQuota` in the call and error response:

```go
	if err := h.store.PutBlob(r.Context(), hhID, hash, data, h.blobQuota); err != nil {
		switch {
		case errors.Is(err, errBlobExists):
			writeJSON(w, http.StatusConflict, map[string]string{"status": "exists"})
		case errors.Is(err, errQuotaExceeded):
			usage, usageErr := h.store.BlobUsage(r.Context(), hhID)
			if usageErr != nil {
				h.log.Error("blob usage query in quota error path", "error", usageErr)
			}
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
				"error":       "blob storage quota exceeded",
				"used_bytes":  usage,
				"quota_bytes": h.blobQuota,
			})
```

- [ ] **Step 10: Update handleStatus**

Replace the hardcoded `DefaultBlobQuota` in `handleStatus`:

```go
	writeJSON(w, http.StatusOK, sync.StatusResponse{
		HouseholdID:  dev.HouseholdID,
		Devices:      devices,
		OpsCount:     opsCount,
		StripeStatus: hh.StripeStatus,
		BlobStorage: sync.BlobStorage{
			UsedBytes:  blobUsed,
			QuotaBytes: h.blobQuota,
		},
	})
```

- [ ] **Step 11: Update existing TestBlobQuotaExceeded**

Replace the test to use `WithBlobQuota(100)` on the handler instead of
`store.SetBlobQuota(100)`:

```go
func TestBlobQuotaExceeded(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithBlobQuota(100))
	hh := createTestHousehold(t, h)

	// Upload a blob larger than quota.
	bigPayload := make([]byte, 200)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		"PUT",
		blobURL(hh.HouseholdID, testHash),
		bytes.NewReader(bigPayload),
	)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), "quota")
}
```

- [ ] **Step 12: Update concurrent_test.go**

In `internal/relay/concurrent_test.go`, replace
`store.SetBlobQuota(quota)` with creating the handler using
`WithBlobQuota(quota)`. The test creates its own handler — change the
handler creation line to:

```go
h := NewHandler(store, slog.Default(), WithBlobQuota(quota))
```

Remove the `store.SetBlobQuota(quota)` line.

- [ ] **Step 13: Run all tests**

Run: `go build ./...`
Run: `go test -shuffle=on ./internal/relay/`
Expected: PASS

- [ ] **Step 14: Commit**

```
feat(relay): add self-hosted mode with subscription bypass and configurable quota

Plumb quota through PutBlob interface (0 = unlimited). Add
WithSelfHosted() to bypass requireSubscription. Add WithBlobQuota(n) to
configure per-household blob storage. DefaultBlobQuota (1 GB) is the
cloud default. handlePutBlob and handleStatus use the configured quota.
```

---

## Task 2: Add BlobStorage.QuotaBytes doc comment

Document that 0 means unlimited on the API type.

**Files:**
- Modify: `internal/sync/types.go:58-61`

- [ ] **Step 1: Add doc comment**

```go
// BlobStorage reports blob storage usage for a household.
type BlobStorage struct {
	UsedBytes  int64 `json:"used_bytes"`
	// QuotaBytes is the per-household blob quota in bytes. 0 means unlimited.
	QuotaBytes int64 `json:"quota_bytes"`
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`

- [ ] **Step 3: Commit**

```
docs(sync): document BlobStorage.QuotaBytes zero-value semantics
```

---

## Task 3: Adaptive storage display in CLI

Update `formatStorageUsage` to handle quota=0 and update `runProStatus`.

**Files:**
- Modify: `cmd/micasa/pro.go:326-329,389-396`
- Modify: `cmd/micasa/pro_test.go` (update ZeroQuota test case)

- [ ] **Step 1: Write failing test for unlimited quota**

Add to `cmd/micasa/pro_test.go`:

```go
func TestFormatStorageUsageUnlimited(t *testing.T) {
	t.Parallel()
	result := formatStorageUsage(52428800, 0) // 50 MiB, unlimited
	assert.NotContains(t, result, "/")
	assert.NotContains(t, result, "%")
	assert.Contains(t, result, "50 MiB")
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test -shuffle=on -run TestFormatStorageUsage ./cmd/micasa/`
Expected: `TestFormatStorageUsageUnlimited` FAIL (currently produces
`50 MiB / 0 B (0.0%)`), and existing `ZeroQuota` still passes (but
that test case will need updating after the fix)

- [ ] **Step 3: Update formatStorageUsage**

```go
// formatStorageUsage formats used/quota bytes. When quota is 0
// (unlimited), returns just the used amount.
func formatStorageUsage(used, quota int64) string {
	if quota <= 0 {
		return formatBytes(used)
	}
	pct := float64(used) / float64(quota) * 100
	return fmt.Sprintf("%s / %s (%.1f%%)", formatBytes(used), formatBytes(quota), pct)
}
```

- [ ] **Step 4: Update existing ZeroQuota test case**

The existing `TestFormatStorageUsage` table test has a `ZeroQuota` case
expecting `"0 B / 0 B (0.0%)"`. Update it to match the new behavior:

```go
{
	name:  "ZeroQuota",
	used:  0,
	quota: 0,
	want:  "0 B",
},
```

- [ ] **Step 5: Update runProStatus**

Replace the hardcoded storage line (lines 326-329):

```go
	if status.BlobStorage.QuotaBytes > 0 {
		fmt.Printf("storage:   %s / %s\n",
			humanize.IBytes(uint64(status.BlobStorage.UsedBytes)),
			humanize.IBytes(uint64(status.BlobStorage.QuotaBytes)),
		)
	} else {
		fmt.Printf("storage:   %s\n",
			humanize.IBytes(uint64(status.BlobStorage.UsedBytes)),
		)
	}
```

- [ ] **Step 6: Run tests**

Run: `go test -shuffle=on ./cmd/micasa/`
Expected: PASS

- [ ] **Step 7: Commit**

```
feat(cli): adaptive storage display for unlimited quota

formatStorageUsage returns just the used amount when quota is 0
(self-hosted unlimited mode) instead of producing broken output.
runProStatus adapts accordingly.
```

---

## Task 4: Wire env vars in cmd/relay/main.go

Parse `SELF_HOSTED`, `STRIPE_WEBHOOK_SECRET`, and `BLOB_QUOTA_BYTES`
with the conflict detection and validation from the spec.

**Files:**
- Modify: `cmd/relay/main.go`
- Create: `cmd/relay/main_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/relay/main_test.go`:

```go
package main

import (
	"testing"

	"github.com/micasa-dev/micasa/internal/relay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRelayMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		selfHosted     string
		webhookSecret  string
		wantSelfHosted bool
		wantErr        bool
	}{
		{
			name:           "cloud mode, no webhook",
			wantSelfHosted: false,
		},
		{
			name:           "cloud mode, webhook set",
			webhookSecret:  "whsec_test",
			wantSelfHosted: false,
		},
		{
			name:           "self-hosted mode",
			selfHosted:     "true",
			wantSelfHosted: true,
		},
		{
			name:          "conflict: both set",
			selfHosted:    "true",
			webhookSecret: "whsec_test",
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			selfHosted, err := resolveRelayMode(tt.selfHosted, tt.webhookSecret)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSelfHosted, selfHosted)
		})
	}
}

func TestParseBlobQuota(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		envVal     string
		selfHosted bool
		want       int64
		wantErr    bool
	}{
		{
			name:       "unset, cloud mode",
			selfHosted: false,
			want:       relay.DefaultBlobQuota,
		},
		{
			name:       "unset, self-hosted",
			selfHosted: true,
			want:       0,
		},
		{
			name:       "explicit value",
			envVal:     "5368709120",
			selfHosted: false,
			want:       5368709120,
		},
		{
			name:       "explicit zero",
			envVal:     "0",
			selfHosted: false,
			want:       0,
		},
		{
			name:    "negative",
			envVal:  "-1",
			wantErr: true,
		},
		{
			name:    "non-integer",
			envVal:  "abc",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseBlobQuota(tt.envVal, tt.selfHosted)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -shuffle=on -run 'TestResolveRelayMode|TestParseBlobQuota' ./cmd/relay/`
Expected: FAIL (undefined functions)

- [ ] **Step 3: Implement helper functions**

Add to `cmd/relay/main.go`:

```go
// resolveRelayMode determines whether the relay runs in self-hosted mode.
// Returns an error if SELF_HOSTED and STRIPE_WEBHOOK_SECRET are both set.
func resolveRelayMode(selfHostedEnv, webhookSecret string) (bool, error) {
	selfHosted := selfHostedEnv == "true"
	if selfHosted && webhookSecret != "" {
		return false, fmt.Errorf(
			"SELF_HOSTED=true and STRIPE_WEBHOOK_SECRET are mutually exclusive -- " +
				"set one or the other, not both",
		)
	}
	return selfHosted, nil
}

// parseBlobQuota parses the BLOB_QUOTA_BYTES env var. Returns the mode
// default when envVal is empty. Returns an error for negative or
// non-integer values.
func parseBlobQuota(envVal string, selfHosted bool) (int64, error) {
	if envVal == "" {
		if selfHosted {
			return 0, nil
		}
		return relay.DefaultBlobQuota, nil
	}
	n, err := strconv.ParseInt(envVal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid BLOB_QUOTA_BYTES %q: %w", envVal, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("BLOB_QUOTA_BYTES must be non-negative, got %d", n)
	}
	return n, nil
}
```

Add `"fmt"` and `"strconv"` to imports (if not already present).

- [ ] **Step 4: Wire into main()**

Update `cmd/relay/main.go` main function. Replace the existing webhook
secret block (lines 48-55) with the new startup logic. Insert after the
`store` setup:

```go
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	selfHosted, err := resolveRelayMode(os.Getenv("SELF_HOSTED"), webhookSecret)
	if err != nil {
		log.Error("startup configuration error", "error", err)
		os.Exit(1)
	}

	blobQuota, err := parseBlobQuota(os.Getenv("BLOB_QUOTA_BYTES"), selfHosted)
	if err != nil {
		log.Error("startup configuration error", "error", err)
		os.Exit(1)
	}

	var handlerOpts []relay.HandlerOption
	if selfHosted {
		handlerOpts = append(handlerOpts, relay.WithSelfHosted())
		log.Info("running in self-hosted mode")
	}
	handlerOpts = append(handlerOpts, relay.WithBlobQuota(blobQuota))

	if webhookSecret != "" {
		handlerOpts = append(handlerOpts, relay.WithWebhookSecret(webhookSecret))
		log.Info("Stripe webhook verification enabled")
	} else {
		log.Warn("STRIPE_WEBHOOK_SECRET not set -- Stripe webhooks will be rejected")
	}

	handler := relay.NewHandler(store, log, handlerOpts...)
```

- [ ] **Step 5: Run tests**

Run: `go build ./...`
Run: `go test -shuffle=on ./cmd/relay/`
Expected: PASS

- [ ] **Step 6: Commit**

```
feat(relay): parse SELF_HOSTED and BLOB_QUOTA_BYTES env vars at startup

Hard error on SELF_HOSTED + STRIPE_WEBHOOK_SECRET conflict.
Hard error on negative or non-integer BLOB_QUOTA_BYTES.
Self-hosted default: quota=0 (unlimited). Cloud default: 1 GB.
```

---

## Task 5: Self-hosted integration test

End-to-end test: self-hosted handler with canceled subscription, blob
upload, download, and status verification.

**Files:**
- Modify: `internal/relay/handler_test.go`

- [ ] **Step 1: Write integration test**

```go
func TestSelfHostedIntegration(t *testing.T) {
	t.Parallel()
	store := NewMemStore()
	h := NewHandler(store, slog.Default(), WithSelfHosted())
	hh := createTestHousehold(t, h)

	// Cancel subscription — self-hosted should still work.
	require.NoError(t, store.UpdateSubscription(
		context.Background(),
		hh.HouseholdID,
		"sub_test",
		"canceled",
	))

	// Upload blob despite canceled subscription.
	payload := []byte("self-hosted-integration-test-blob")
	hash := fmt.Sprintf("%x", sha256.Sum256(payload))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", blobURL(hh.HouseholdID, hash), bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// Download it back.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", blobURL(hh.HouseholdID, hash), nil)
	req.Header.Set("Authorization", "Bearer "+hh.DeviceToken)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, payload, rec.Body.Bytes())

	// Status reports quota=0.
	rec = httptest.NewRecorder()
	req = authRequest("GET", "/status", nil, hh.DeviceToken)
	h.ServeHTTP(rec, req)
	var status sync.StatusResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	assert.Equal(t, int64(0), status.BlobStorage.QuotaBytes)
	assert.Equal(t, int64(len(payload)), status.BlobStorage.UsedBytes)
}
```

- [ ] **Step 2: Run test**

Run: `go test -shuffle=on -run TestSelfHostedIntegration ./internal/relay/`
Expected: PASS (implementation done in Task 1)

- [ ] **Step 3: Commit**

```
test(relay): add self-hosted mode integration test

Verifies subscription bypass, blob upload with unlimited quota,
blob download, and status reporting quota_bytes=0.
```

---

## Task 6: Docker deployment artifacts

Create all Docker files for the self-hosted deployment.

**Files:**
- Create: `deploy/relay/Dockerfile`
- Create: `deploy/docker-compose.yml`
- Create: `deploy/docker-compose.caddy.yml`
- Create: `deploy/Caddyfile`
- Create: `deploy/.env.example`

- [ ] **Step 1: Create Dockerfile**

Create `deploy/relay/Dockerfile`:

```dockerfile
# Build stage
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /relay ./cmd/relay

# Runtime stage — alpine (not scratch) for wget health checks
FROM alpine:3
COPY --from=build /relay /relay
EXPOSE 8080
ENTRYPOINT ["/relay"]
```

- [ ] **Step 2: Create docker-compose.yml**

Create `deploy/docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:17
    environment:
      POSTGRES_USER: micasa
      POSTGRES_DB: micasa
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U micasa -d micasa"]
      interval: 5s
      timeout: 5s
      retries: 5

  relay:
    build:
      context: ../
      dockerfile: deploy/relay/Dockerfile
    environment:
      DATABASE_URL: postgres://micasa:${POSTGRES_PASSWORD}@postgres:5432/micasa?sslmode=disable
      SELF_HOSTED: "true"
      PORT: "8080"
      BLOB_QUOTA_BYTES: ${BLOB_QUOTA_BYTES:-}
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3

volumes:
  pgdata:
```

- [ ] **Step 3: Create docker-compose.caddy.yml**

Create `deploy/docker-compose.caddy.yml`:

```yaml
services:
  caddy:
    image: caddy:2-alpine
    environment:
      DOMAIN: ${DOMAIN}
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
      - caddy_config:/config
    depends_on:
      relay:
        condition: service_healthy

  relay:
    ports: []

volumes:
  caddy_data:
  caddy_config:
```

- [ ] **Step 4: Create Caddyfile**

Create `deploy/Caddyfile`:

```
{$DOMAIN} {
    reverse_proxy relay:8080
}
```

- [ ] **Step 5: Create .env.example**

Create `deploy/.env.example`:

```
SELF_HOSTED=true
POSTGRES_PASSWORD=changeme
# Per-household blob quota in bytes (default: unlimited in self-hosted mode):
# BLOB_QUOTA_BYTES=0
# Uncomment for TLS via Caddy:
# DOMAIN=sync.example.com
```

- [ ] **Step 6: Validate compose syntax**

Run: `docker compose -f deploy/docker-compose.yml config`
Expected: valid YAML output (may warn about missing POSTGRES_PASSWORD)

- [ ] **Step 7: Commit**

```
feat(deploy): add Docker Compose stack for self-hosted relay

Multi-stage Dockerfile builds a static relay binary on alpine.
Base compose runs postgres + relay with health checks.
Caddy override adds automatic TLS via DOMAIN env var.
```

---

## Verification

After all tasks are complete:

- [ ] `go build ./...` -- compiles clean
- [ ] `go test -shuffle=on ./internal/relay/` -- all relay tests pass
- [ ] `go test -shuffle=on ./cmd/relay/` -- startup config tests pass
- [ ] `go test -shuffle=on ./cmd/micasa/` -- CLI tests pass
- [ ] `go test -shuffle=on ./...` -- full suite green
- [ ] `docker compose -f deploy/docker-compose.yml config` -- valid syntax
