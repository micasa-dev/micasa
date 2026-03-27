#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Run whole-program dead code analysis. Exits non-zero if findings exist
# (deadcode itself always exits 0).

set -euo pipefail

_tmpdir=$(mktemp -d -t micasa-deadcode-XXXXXX)
trap 'chmod -R u+w "$_tmpdir" 2>/dev/null; rm -rf "$_tmpdir"' EXIT
export GOCACHE="${GOCACHE:-$_tmpdir/gocache}"
export GOMODCACHE="${GOMODCACHE:-$_tmpdir/gomodcache}"
output=$(command deadcode -generated -test "$@" ./...)
if [ -n "$output" ]; then
  echo "$output"
  exit 1
fi
