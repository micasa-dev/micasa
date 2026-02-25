+++
title = "Building from Source"
weight = 1
description = "How to build micasa from source."
linkTitle = "Building"
+++

## Prerequisites

- **Go 1.25+** (the only hard requirement)
- **Nix** (optional, but provides the full dev environment)

## Quick build

```sh
git clone https://github.com/cpcloud/micasa.git
cd micasa
CGO_ENABLED=0 go build ./cmd/micasa
./micasa --demo
```

micasa uses a pure-Go SQLite driver, so `CGO_ENABLED=0` works and produces a
fully static binary.

## Nix dev shell

The recommended development environment uses Nix flakes:

```sh
nix develop
```

This gives you:

- `go` compiler
- `golangci-lint` (static analysis)
- `golines` + `gofumpt` (formatting)
- `osv-scanner` (vulnerability scanning)
- `hugo` (docs site)
- `vhs` (terminal recording)
- `git`
- Pre-commit hooks (auto-installed on first shell entry)

Everything is pinned to a consistent version. No system dependency surprises.

## Build commands

From within the dev shell (or with Go installed):

```sh
# Build the binary
go build ./cmd/micasa

# Run directly
go run ./cmd/micasa -- --demo

# Run tests
go test -shuffle=on -v ./...
```

## Nix build

To build the binary via Nix (reproducible, hermetic):

```sh
nix build
./result/bin/micasa --demo
```

## Nix flake apps

The flake exposes several convenience apps:

| Command | Description |
|---------|-------------|
| `nix run` | Run micasa directly |
| `nix run '.#website'` | Serve the website locally with live reload |
| `nix run '.#docs'` | Build the Hugo site into `website/` |
| `nix run '.#record-demo'` | Record the main demo GIF |
| `nix run '.#record-tape'` | Record a single VHS tape to WebP |
| `nix run '.#record-animated'` | Record all `using-*.tape` animated demos in parallel |
| `nix run '.#capture-one'` | Capture a single VHS tape as a WebP screenshot |
| `nix run '.#capture-screenshots'` | Capture all screenshot tapes in parallel |
| `nix run '.#pre-commit'` | Run pre-commit hooks on all files |
| `nix run '.#deadcode'` | Run whole-program dead code analysis |
| `nix run '.#osv-scanner'` | Scan dependencies for known vulnerabilities |

## Container image

Multi-arch container images (`linux/amd64` and `linux/arm64`) are published to
GHCR on every release:

```sh
docker run -it --rm ghcr.io/cpcloud/micasa:latest --demo
```
