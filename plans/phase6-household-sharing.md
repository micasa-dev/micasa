<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Phase 6: Household Sharing

## Goal

Enable multi-device households through invite codes with NaCl box key
exchange. The relay facilitates key exchange without seeing the household
key.

## Implementation Plan

### Step 1: NaCl box crypto (`internal/crypto/box.go`)

Asymmetric encryption for household key transfer:
- `BoxSeal()` -- encrypt with sender private key + recipient public key
- `BoxOpen()` -- decrypt with recipient private key + sender public key
- Uses `nacl/box` (Curve25519 + XSalsa20-Poly1305)

### Step 2: Invite/join types (`internal/sync/types.go`)

New types for the sharing protocol:
- `InviteCode` -- code, household, expiry
- `JoinRequest/Response` -- joiner submits pubkey, gets exchange ID
- `PendingKeyExchange` -- tracks pending key exchanges
- `CompleteKeyExchangeRequest` -- inviter submits encrypted household key
- `KeyExchangeResult` -- joiner polls for encrypted key + device credentials

### Step 3: Store interface + MemStore

Extend `Store` with invite/join/device management:
- `CreateInvite` -- generate 8-char base32 code (24h expiry, max 5 attempts)
- `StartJoin` -- validate invite, store pending exchange
- `GetPendingExchanges` -- inviter discovers pending joins
- `CompleteKeyExchange` -- inviter submits encrypted key, registers joiner
- `GetKeyExchangeResult` -- joiner polls for result
- `ListDevices` -- list household devices
- `RevokeDevice` -- remove device auth

### Step 4: HTTP handlers

New relay routes:
- `POST /households/{id}/invite` -- create invite (auth required)
- `POST /invite/{code}/join` -- start join (no auth, invite is credential)
- `GET /households/{id}/pending-exchanges` -- inviter polls (auth required)
- `POST /key-exchange/{id}/complete` -- inviter submits encrypted key (auth)
- `GET /key-exchange/{id}` -- joiner polls for result (no auth, ID is capability)
- `GET /households/{id}/devices` -- list devices (auth required)
- `DELETE /households/{id}/devices/{device_id}` -- revoke (auth required)

### Step 5: Tests

- NaCl box round-trip, wrong keys, tampered data
- Invite creation and expiry
- Full key exchange flow (invite -> join -> complete -> poll)
- Max invite attempts enforcement
- Device list and revocation
- Auth enforcement on all protected endpoints

## File Changes

| File | Change |
|------|--------|
| `internal/crypto/box.go` | New: NaCl box seal/open |
| `internal/crypto/box_test.go` | New: box crypto tests |
| `internal/sync/types.go` | Extended: invite/join/exchange types |
| `internal/relay/store.go` | Extended: invite/join/device methods |
| `internal/relay/memstore.go` | Extended: MemStore implementation |
| `internal/relay/handler.go` | Extended: new HTTP routes |
| `internal/relay/handler_test.go` | Extended: new handler tests |
