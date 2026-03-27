#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Pre-commit hook: ensure every file has the correct license header.
# Adds missing headers and bumps stale years.

set -euo pipefail

year=$(date +%Y)
owner="Phillip Cloud"
spdx="Licensed under the Apache License, Version 2.0"

comment_prefix() {
  case "$1" in
    *.go|go.mod|*.js) echo "//" ;;
    *.nix|*.yml|*.yaml|*.sh|*.bash|.envrc|.gitignore) echo "#" ;;
    *.md)         echo "md" ;;
    *)            echo "#" ;;
  esac
}

status=0
for f in "$@"; do
  name=$(basename "$f")
  pfx=$(comment_prefix "$name")

  if [ "$pfx" = "md" ]; then
    line1="<!-- Copyright $year $owner -->"
    line2="<!-- $spdx -->"
    year_pat="<!-- Copyright [0-9]\{4\} $owner -->"
  else
    line1="$pfx Copyright $year $owner"
    line2="$pfx $spdx"
    year_pat="$pfx Copyright [0-9]\{4\} $owner"
  fi

  first=$(head -n1 "$f")

  # Shebang-aware: if first line is #!, check lines 2-3 instead
  if echo "$first" | grep -q '^#!'; then
    check1=$(sed -n '2p' "$f")
    check2=$(sed -n '3p' "$f")
    insert_line=1  # insert after line 1 (the shebang)
  else
    check1="$first"
    check2=$(sed -n '2p' "$f")
    insert_line=0  # insert before line 1
  fi

  # Already correct
  if [ "$check1" = "$line1" ] && [ "$check2" = "$line2" ]; then
    continue
  fi

  # Header present with stale year -- bump it
  if echo "$check1" | grep -q "^$year_pat$" \
     && [ "$check2" = "$line2" ]; then
    sed -i "s|$year_pat|$line1|" "$f"
    echo "bumped year in $f"
    continue
  fi

  # No header -- insert it
  if [ "$insert_line" -eq 0 ]; then
    sed -i "1i\\$line1\n$line2\n" "$f"
  else
    sed -i "1a\\$line1\n$line2" "$f"
  fi
  echo "added license header to $f"
  status=1
done
exit $status
