#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Wrapper around govulncheck that filters out excluded vulnerabilities.
# Exclusions are listed in .govulncheck-exclude (one GO-YYYY-NNNN per line).

set -euo pipefail

_tmpdir=$(mktemp -d -t micasa-govulncheck-XXXXXX)
trap 'chmod -R u+w "$_tmpdir" 2>/dev/null; rm -rf "$_tmpdir"' EXIT
export GOCACHE="${GOCACHE:-$_tmpdir/gocache}"
export GOMODCACHE="${GOMODCACHE:-$_tmpdir/gomodcache}"

exclude_file=".govulncheck-exclude"
raw=$(command govulncheck -format json ./... 2>&1) || true
found=$(echo "$raw" | jq -r 'select(.finding) | select(.finding.trace[0].function) | .finding.osv' | sort -u)

if [ -z "$found" ]; then
  exit 0
fi

excluded=""
if [ -f "$exclude_file" ]; then
  excluded=$(rg -oN 'GO-[0-9]+-[0-9]+' "$exclude_file" | sort -u)
fi

new=$(comm -23 <(echo "$found") <(echo "$excluded"))

if [ -z "$new" ]; then
  exit 0
fi

echo "govulncheck: unexcluded vulnerabilities found:"
echo "$new"
exit 1
