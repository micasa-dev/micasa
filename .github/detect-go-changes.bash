#!/usr/bin/env bash
# Detect whether a PR or push includes Go-related changes.
# Prints "true" or "false" to stdout.
#
# Requires: GH_TOKEN, GITHUB_REPOSITORY
# Arguments: <event_name> <pr_number|""> <before_sha|""> <head_sha|"">
set -euo pipefail

event_name=$1
pr_number=${2:-}
before_sha=${3:-}
head_sha=${4:-}

if [ "$event_name" = "pull_request" ]; then
  changed=$(gh api "repos/${GITHUB_REPOSITORY}/pulls/${pr_number}/files" \
    --paginate --jq '.[].filename')
else
  if [ "$before_sha" = "0000000000000000000000000000000000000000" ]; then
    echo true
    exit 0
  fi
  changed=$(gh api "repos/${GITHUB_REPOSITORY}/compare/${before_sha}...${head_sha}" \
    --jq '.files[].filename')
fi

if [ -z "$changed" ]; then
  echo false
  exit 0
fi

# For push events, the compare API caps at 300 files.
if [ "$event_name" != "pull_request" ]; then
  file_count=$(echo "$changed" | wc -l)
  if [ "$file_count" -ge 300 ]; then
    echo "::warning::Compare API file cap hit ($file_count files), assuming Go changes" >&2
    echo true
    exit 0
  fi
fi

# Anything outside these paths is considered Go-related.
# Intentionally conservative: unknown paths trigger Go CI.
non_docs=$(echo "$changed" | grep -vE '^docs/|^images/|\.md$|^LICENSE|^\.github/workflows/pages\.yml$|^\.claude/' || true)
if [ -n "$non_docs" ]; then
  echo true
else
  echo false
fi
