#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0
#
# Check .claude/codebase/*.md for stale <!-- verified: YYYY-MM-DD --> dates.
# If any file is >30 days stale or missing the tag, inject context telling
# the agent to audit and update them before starting other work.
set -euo pipefail

stale=()

for f in "$CLAUDE_PROJECT_DIR"/.claude/codebase/*.md; do
  [ -f "$f" ] || continue
  name=$(basename "$f")
  # sed and jq instead of grep -P and date -d, which BSD (macOS) lacks.
  date_str=$(sed -n 's/^<!-- verified: \([0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}\) -->$/\1/p' "$f")

  if [ -z "$date_str" ]; then
    stale+=("$name: missing verified date")
    continue
  fi

  age_days=$(jq -n --arg d "$date_str" \
    '(now - ($d | strptime("%Y-%m-%d") | mktime)) / 86400 | floor')

  if [ "$age_days" -gt 30 ]; then
    stale+=("$name: last verified $date_str ($age_days days ago)")
  fi
done

[ ${#stale[@]} -eq 0 ] && exit 0

msg="CODEBASE MAP STALE — audit and update before starting other work:"
for s in "${stale[@]}"; do
  msg+=$'\n'"  - $s"
done

jq -n --arg msg "$msg" '{
  hookSpecificOutput: {
    hookEventName: "SessionStart",
    additionalContext: $msg
  }
}'
