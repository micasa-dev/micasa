+++
title = "Self-Hosting"
weight = 3
description = "Run your own sync relay with Docker Compose."
linkTitle = "Self-Hosting"
+++

micasa syncs your household data across machines through an encrypted
relay. Run your own with [Docker Compose](https://docs.docker.com/compose/).

## What you need

- A machine with [Docker](https://docs.docker.com/get-docker/) and
  Docker Compose
- A domain name (optional, for TLS)

The relay runs two containers: PostgreSQL for storage and the relay
binary for sync traffic. PostgreSQL holds encrypted sync operations,
encrypted document blobs, device registrations, and invite state.
All household data is end-to-end encrypted — the relay never sees
plaintext.

## Quick start

Clone the repo and copy the example env file:

```sh
git clone https://github.com/micasa-dev/micasa.git
cd micasa/deploy
cp .env.example .env
```

Edit `.env` and set a real password and encryption key:

```sh
# Generate an encryption key first:
openssl rand -hex 32
# Copy the output, then edit .env:
```

```sh
POSTGRES_PASSWORD=something-strong-here
RELAY_ENCRYPTION_KEY=paste-the-64-char-hex-key-here
```

Start the stack:

```sh
docker compose up -d
```

The relay is now running on port 8080. Verify:

```sh
curl http://localhost:8080/health
# {"status":"ok"}
```

## Connect micasa to your relay

On each machine, initialize with your relay URL:

```sh
micasa pro init --relay-url http://your-server:8080
```

On the first machine this creates a new household. On additional
machines, generate an invite on the first machine and join from the
second:

```sh
# Machine 1:
micasa pro invite

# Machine 2:
micasa pro join <code> --relay-url http://your-server:8080
```

Sync happens automatically after that.

## Adding TLS

For production use, add TLS via the included Caddy overlay. Set your
domain in `.env`:

```sh
DOMAIN=sync.example.com
```

Point your DNS A record to the server, then start with both compose
files:

```sh
docker compose \
  -f docker-compose.yml \
  -f docker-compose.caddy.yml \
  up -d
```

Caddy automatically obtains and renews a Let's Encrypt certificate.
The relay is no longer exposed on port 8080 — all traffic goes through
Caddy on 443.

Connect micasa using the HTTPS URL:

```sh
micasa pro init --relay-url https://sync.example.com
```

## Configuration

All configuration is via environment variables in `.env`.

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_PASSWORD` | *(required)* | PostgreSQL password. |
| `RELAY_ENCRYPTION_KEY` | *(required)* | 32-byte hex key for encrypting device tokens at rest. Generate with `openssl rand -hex 32`. |
| `BLOB_QUOTA` | `0` | Per-household blob storage limit. Accepts human-readable sizes (`5GB`, `500MB`). `0` = unlimited. |
| `DOMAIN` | *(unset)* | Domain for TLS via Caddy. Only needed with the Caddy overlay. |

## Blob storage

Documents attached in micasa are synced as encrypted blobs. By default,
self-hosted relays have no storage limit per household. To set a limit:

```sh
# 5 GB per household
BLOB_QUOTA=5GB
```

Individual blobs are capped at 50 MB regardless of quota.

## Backups

Everything the relay stores lives in PostgreSQL. Back up with:

```sh
docker compose exec postgres pg_dump -U micasa micasa > backup.sql
```

The backup contains encrypted data only — sync operations, blobs, and
device registrations. Encryption keys live on each device, not on the
relay, so a database dump is not useful without the devices that
generated the keys.

## Updating

Pull the latest code and rebuild:

```sh
git pull
docker compose up -d --build
```

The relay runs database migrations automatically on startup.
