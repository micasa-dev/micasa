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

  # The header goes after a shebang or YAML front matter, which must stay
  # on line 1. insert_line is the last line before the header (0 = none).
  if echo "$first" | grep -q '^#!'; then
    insert_line=1
  elif [ "$pfx" = "md" ] && [ "$first" = "---" ]; then
    insert_line=$(sed -n '2,${/^---$/{=;q}}' "$f")
    if [ -z "$insert_line" ]; then
      echo "unterminated front matter in $f: add a closing --- line" >&2
      status=1
      continue
    fi
  else
    insert_line=0
  fi
  check1=$(sed -n "$((insert_line + 1))p" "$f")
  check2=$(sed -n "$((insert_line + 2))p" "$f")

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
    sed -i "${insert_line}a\\$line1\n$line2" "$f"
  fi
  echo "added license header to $f"
  status=1
done
exit $status
