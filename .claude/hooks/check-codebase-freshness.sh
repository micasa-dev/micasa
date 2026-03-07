#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0
#
# Check .claude/codebase/*.md for stale <!-- verified: YYYY-MM-DD --> dates.
# If any file is >30 days stale or missing the tag, inject context telling
# the agent to audit and update them before starting other work.
set -euo pipefail

today=$(date +%s)
stale=()

for f in "$CLAUDE_PROJECT_DIR"/.claude/codebase/*.md; do
  [ -f "$f" ] || continue
  name=$(basename "$f")
  date_str=$(grep -oP '(?<=<!-- verified: )\d{4}-\d{2}-\d{2}' "$f" || true)

  if [ -z "$date_str" ]; then
    stale+=("$name: missing verified date")
    continue
  fi

  file_epoch=$(date -d "$date_str" +%s)
  age_days=$(( (today - file_epoch) / 86400 ))

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
