<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Self-Hosted Relay

## Goal

Make the relay self-hostable via Docker Compose so users can run their
own sync infrastructure. The cloud version becomes a managed instance of
the same binary. "End-to-end encrypted. Self-host or let us run it for
you. Same code, same binary."

## Relay Mode

The relay operates in one of two modes, resolved at startup from
environment variables:

- **Cloud mode** (default): Stripe webhook verification enabled,
  subscription gating enforced, blob quota = 1 GB per household.
- **Self-hosted mode** (`SELF_HOSTED=true`): Stripe disabled,
  subscription gating bypassed, blob quota configurable (default:
  unlimited).

### Env Var Resolution

| `SELF_HOSTED` | `STRIPE_WEBHOOK_SECRET` | Result |
|---|---|---|
| unset | unset | Cloud mode, webhooks return 503 |
| unset | set | Cloud mode, webhooks verified |
| `true` | unset | Self-hosted mode |
| `true` | set | **Hard error, refuse to start** |

The conflict case is a hard startup error because it's ambiguous whether
the operator intended cloud or self-hosted behavior. Forcing an explicit
choice prevents misconfiguration.

### Subscription Bypass

In self-hosted mode, `requireSubscription` is a no-op passthrough. All
households are treated as having active subscriptions regardless of
their `stripe_status` field.

### Stripe Webhooks in Self-Hosted Mode

The `POST /webhooks/stripe` route stays registered in both modes. When
`webhookSecret` is empty (always true in self-hosted mode, since the
hard error prevents both vars being set), the handler already returns
503. No code change needed -- the existing behavior is correct.

### Blob Quota

`BLOB_QUOTA_BYTES` env var controls per-household blob storage.
Resolution order:

1. If `BLOB_QUOTA_BYTES` is set and valid, use that value.
2. If unset, use the mode default:
   - Self-hosted: `0` (unlimited)
   - Cloud: `1073741824` (1 GB, the existing `defaultBlobQuota`)

Negative, non-integer, or otherwise unparseable values are a hard
startup error.

Quota is a configurable field on the Handler, set via
`WithBlobQuota(int64)`. The Handler passes the quota value to the
store via `PutBlob`'s new `quota int64` parameter. The store
enforces the quota: when quota is 0 the store skips the usage
check; when quota > 0 the store enforces it. This replaces the
current approach where each store implementation hardcodes
`defaultBlobQuota` internally.

The `handlePutBlob` 413 response reports the Handler's configured
quota (not the `defaultBlobQuota` constant). The handler does not
duplicate enforcement logic -- it simply passes `h.blobQuota`
through to the store and formats the error response.

The `handleStatus` endpoint reports `quota_bytes: 0` when unlimited.
Add a doc comment on `BlobStorage.QuotaBytes`: "0 means unlimited."
The client adapts its display (no division by quota):

- `quota_bytes > 0`: `storage: 52.4 MB / 1.0 GB`
- `quota_bytes == 0`: `storage: 52.4 MB`

## Health Endpoint

The relay already registers an unauthenticated `GET /health` endpoint
returning `200 {"status":"ok"}`. No new endpoint needed -- Docker
health checks and the Caddy compose override use this existing route.

## Docker Artifacts

All deployment files live under `deploy/`.

### `deploy/relay/Dockerfile`

Multi-stage build:

1. **Build stage**: `golang:<version>-alpine` (match `go.mod` go
   directive), copies module files, downloads deps, builds
   `cmd/relay/main.go` with `CGO_ENABLED=0` and `-ldflags='-s -w'`
   for a static stripped binary.
2. **Runtime stage**: `FROM alpine:3` (not `scratch` -- need `wget`
   for Docker health checks), copies the binary, exposes 8080,
   `ENTRYPOINT ["/relay"]`.

### `deploy/docker-compose.yml`

Base stack (no TLS):

- **postgres**: `postgres:17`, named volume for data,
  `POSTGRES_USER=micasa`, `POSTGRES_DB=micasa`,
  `POSTGRES_PASSWORD=${POSTGRES_PASSWORD}`.
  Health check: `pg_isready -U micasa -d micasa`.
- **relay**: built from `deploy/relay/Dockerfile`, env vars:
  `DATABASE_URL=postgres://micasa:${POSTGRES_PASSWORD}@postgres:5432/micasa?sslmode=disable`,
  `SELF_HOSTED=true`, `PORT=8080`, optional `BLOB_QUOTA_BYTES`.
  Depends on postgres with `condition: service_healthy`.
  Health check hits `GET /health`. Exposes port 8080.

### `deploy/docker-compose.caddy.yml`

TLS override (compose with base via `-f`):

- Adds **caddy** service (`caddy:2-alpine`) reading `DOMAIN` env var.
- Reverse proxies `relay:8080`.
- Relay no longer exposes ports directly.
- Uses `deploy/Caddyfile` template.

Usage:
```bash
# Without TLS (local/dev):
docker compose -f deploy/docker-compose.yml up

# With TLS:
docker compose -f deploy/docker-compose.yml \
               -f deploy/docker-compose.caddy.yml up
```

### `deploy/Caddyfile`

```
{$DOMAIN} {
    reverse_proxy relay:8080
}
```

### `deploy/.env.example`

```
SELF_HOSTED=true
POSTGRES_PASSWORD=changeme
# Per-household blob quota in bytes (default: unlimited in self-hosted mode):
# BLOB_QUOTA_BYTES=0
# Uncomment for TLS via Caddy:
# DOMAIN=sync.example.com
```

## Code Changes

### `cmd/relay/main.go`

- Parse `SELF_HOSTED` and `STRIPE_WEBHOOK_SECRET` env vars.
- Hard error if both are set.
- Pass `relay.WithSelfHosted()` option to handler in self-hosted mode.
- Parse `BLOB_QUOTA_BYTES` env var: hard error on negative values.
  If unset, default is 0 in self-hosted mode, `defaultBlobQuota`
  (1 GB) in cloud mode. Pass via `relay.WithBlobQuota()`.

### `internal/relay/handler.go`

- Add `selfHosted bool` and `blobQuota int64` fields to Handler.
- Add `WithSelfHosted() HandlerOption`.
- Add `WithBlobQuota(int64) HandlerOption`.
- `requireSubscription`: if `selfHosted`, pass through immediately.
- `handlePutBlob`: pass `h.blobQuota` to `store.PutBlob`. The
  413 response uses `h.blobQuota` (not the constant).
- `handleStatus`: report `h.blobQuota` (0 = unlimited).

### `internal/relay/store.go`

- Update the `Store` interface: change `PutBlob` signature to
  `PutBlob(ctx context.Context, householdID, hash string, data []byte, quota int64) error`.
  A quota of 0 means skip enforcement.

### `internal/relay/blob.go`

- Keep `defaultBlobQuota` as the cloud-mode default (used by
  `cmd/relay/main.go` for the cloud-mode fallback).

### `internal/relay/memstore.go` + `internal/relay/pgstore.go`

- `PutBlob` accepts quota parameter. When quota is 0, skip the
  usage check. When quota > 0, enforce it.
- Remove `MemStore.blobQuotaBytes()` helper and `SetBlobQuota`
  method (quota is now caller-controlled). Update tests to pass
  quota via the handler option instead.
- **Semantic inversion**: previously `blobQuota == 0` meant "use
  1 GB default." Now `quota == 0` means "unlimited." Any existing
  test relying on `SetBlobQuota(0)` to mean the default must be
  updated to pass `defaultBlobQuota` explicitly.

### `cmd/micasa/pro.go`

- `runProStatus`: adaptive storage display. If `QuotaBytes == 0`,
  print `storage: X` without quota. If `QuotaBytes > 0`, print
  `storage: X / Y`. No division by quota -- purely display logic.
- `formatStorageUsage`: update to handle `quota == 0` (return
  usage-only string). Currently always renders `X / Y (Z%)` which
  would produce NaN/Inf when quota is 0.
- `runProStorage`: inherits the fix via `formatStorageUsage`.

### `internal/sync/types.go`

- Add doc comment on `BlobStorage.QuotaBytes`: `// 0 means unlimited.`

### Database Migrations

The relay binary runs `store.AutoMigrate()` on startup (existing
behavior). Self-hosted users get schema creation automatically on
first run. No separate migration step needed.

## Testing

- **Unit**: startup mode resolution (all 4 env var combinations),
  hard error case, negative `BLOB_QUOTA_BYTES` error,
  subscription bypass in self-hosted mode, quota=0 disables
  enforcement, `/health` returns 200 (already tested).
- **Integration**: self-hosted mode end-to-end -- create handler
  with `WithSelfHosted()` + `WithBlobQuota(0)`, verify
  subscription bypass, verify blob upload with no quota limit,
  verify status reports `quota_bytes: 0`.
- **Existing tests**: must continue passing -- cloud mode is the
  default when no options are set.
- **Docker**: `docker compose -f deploy/docker-compose.yml config`
  validates compose syntax (no runtime test needed in CI).

## Out of Scope

- S3-compatible blob storage backend (future enhancement).
- Automatic backup/restore tooling for the Postgres volume.
- Kubernetes manifests or Helm charts.
- Documentation website page (separate task).
