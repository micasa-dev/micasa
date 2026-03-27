#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Run golangci-lint with isolated caches.

set -euo pipefail

_tmpdir=$(mktemp -d -t micasa-golangci-lint-XXXXXX)
trap 'chmod -R u+w "$_tmpdir" 2>/dev/null; rm -rf "$_tmpdir"' EXIT
export GOCACHE="${GOCACHE:-$_tmpdir/gocache}"
export GOMODCACHE="${GOMODCACHE:-$_tmpdir/gomodcache}"
command golangci-lint run ./...
