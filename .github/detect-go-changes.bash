#!/usr/bin/env bash
# Detect whether a PR or push includes changes that need CI.
# Outputs two lines to stdout:
#   go=true|false   — whether Go-related files changed (tests, build, lint)
#   ci=true|false   — whether ANY file that warrants CI changed
#
# ci=false only when every changed file is metadata that needs no CI at all
# (root .md files, .claude/, LICENSE). ci=true includes docs/ and images/
# changes that still need the docs build and pre-commit checks.
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
    echo "go=true"
    echo "ci=true"
    exit 0
  fi
  changed=$(gh api "repos/${GITHUB_REPOSITORY}/compare/${before_sha}...${head_sha}" \
    --jq '.files[].filename')
fi

if [ -z "$changed" ]; then
  echo "go=false"
  echo "ci=false"
  exit 0
fi

# For push events, the compare API caps at 300 files.
if [ "$event_name" != "pull_request" ]; then
  file_count=$(echo "$changed" | wc -l)
  if [ "$file_count" -ge 300 ]; then
    echo "::warning::Compare API file cap hit ($file_count files), assuming Go changes"
    echo "go=true"
    echo "ci=true"
    exit 0
  fi
fi

# Anything outside these paths is considered Go-related.
# Intentionally conservative: unknown paths trigger Go CI.
non_docs=$(echo "$changed" | grep -vE '^docs/|^images/|\.md$|^LICENSE|^\.github/workflows/pages\.yml$|^\.claude/' || true)
if [ -n "$non_docs" ]; then
  echo "go=true"
else
  echo "go=false"
fi

# Files that need zero CI: root markdown, .claude/, LICENSE.
# Everything else (including docs/, images/, workflows) needs some CI.
needs_ci=$(echo "$changed" | grep -vE '\.md$|^LICENSE$|^\.claude/' || true)
if [ -n "$needs_ci" ]; then
  echo "ci=true"
else
  echo "ci=false"
fi
