<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Phase 3: Encryption Layer

## Goal

Client-side encryption for sync operations using NaCl secretbox
(XSalsa20-Poly1305). All data is encrypted before leaving the device.
The relay server stores opaque ciphertext it cannot decrypt.

## Key Hierarchy

- **Household Key (HK):** 256-bit random symmetric key. Generated once
  during `micasa pro init`. Encrypts all sync operations.
- **Device Key (DK):** Curve25519 keypair. Generated per device. Used for
  key exchange when joining a household (Phase 6).

## Key Storage

Keys stored in `$XDG_DATA_HOME/micasa/keys/` (via `xdg.DataFile`):

```
household.key   # 32-byte household symmetric key (0600)
device.pub      # 32-byte Curve25519 public key
device.key      # 32-byte Curve25519 private key (0600)
```

Keys are NOT in SQLite (the DB is the data being synced; keys are the
mechanism for syncing).

## Implementation Plan

### Step 1: Package structure

New package `internal/crypto` with:
- `keys.go` -- key generation, loading, saving
- `encrypt.go` -- secretbox encrypt/decrypt
- `crypto_test.go` -- tests

### Step 2: Key types and generation

```go
type HouseholdKey [32]byte
type DeviceKeyPair struct {
    PublicKey  [32]byte
    PrivateKey [32]byte
}
```

- `GenerateHouseholdKey() (HouseholdKey, error)` -- crypto/rand
- `GenerateDeviceKeyPair() (DeviceKeyPair, error)` -- Curve25519

### Step 3: Key persistence

- `SecretsDir() string` -- `xdg.DataFile("micasa/secrets/")`
- `SaveHouseholdKey(key HouseholdKey) error`
- `LoadHouseholdKey() (HouseholdKey, error)`
- `SaveDeviceKeyPair(kp DeviceKeyPair) error`
- `LoadDeviceKeyPair() (DeviceKeyPair, error)`
- File permissions: 0600 for private keys

### Step 4: Encrypt/Decrypt

- `Encrypt(key HouseholdKey, plaintext []byte) ([]byte, error)`
  - Generate random 24-byte nonce
  - NaCl secretbox Seal
  - Return nonce || ciphertext
- `Decrypt(key HouseholdKey, sealed []byte) ([]byte, error)`
  - Split nonce (first 24 bytes) from ciphertext
  - NaCl secretbox Open
  - Return plaintext or error

### Step 5: Tests

- Round-trip encrypt/decrypt
- Decrypt with wrong key fails
- Decrypt tampered ciphertext fails
- Key generation produces unique keys
- Key save/load round-trip
- File permissions are correct (0600)
- Empty plaintext encrypt/decrypt

## Dependencies

- `golang.org/x/crypto/nacl/secretbox` (already indirect dep)
- `golang.org/x/crypto/curve25519` (already indirect dep)
- `github.com/adrg/xdg` (existing dep)

## File Changes

| File | Change |
|------|--------|
| `internal/crypto/keys.go` | New: key types, generation, persistence |
| `internal/crypto/encrypt.go` | New: secretbox encrypt/decrypt |
| `internal/crypto/crypto_test.go` | New: comprehensive tests |
