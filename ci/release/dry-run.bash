#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0
#
# Simulate a semantic-release run in a disposable worktree.
# Strips @semantic-release/github to avoid any GitHub API calls.

set -euo pipefail

curdir="$PWD"
worktree="$(mktemp -d)"
branch="$(basename "$worktree")"

git worktree add "$worktree"

cleanup() {
  cd "$curdir"
  git worktree remove --force "$worktree"
  git worktree prune
  git branch -D "$branch"
}
trap cleanup EXIT

cd "$worktree"

# Strip @semantic-release/github so the dry-run makes no API calls
jq '.plugins |= map(select(
  if type == "array" then .[0] != "@semantic-release/github"
  else . != "@semantic-release/github"
  end
))' .releaserc.json > .releaserc.json.tmp
mv .releaserc.json.tmp .releaserc.json

git config user.email "ci@localhost"
git config user.name "CI"
git add .releaserc.json
PRE_COMMIT_ALLOW_NO_CONFIG=1 git commit -m "test: semantic-release dry run" --no-gpg-sign

# Unset so semantic-release exercises the full pipeline instead of
# short-circuiting on PR detection
unset GITHUB_ACTIONS

npx --yes \
  -p "semantic-release@25.0.3" \
  -p "@semantic-release/commit-analyzer" \
  -p "@semantic-release/release-notes-generator" \
  -p "@semantic-release/exec@7.1.0" \
  -p "@semantic-release/git@10.0.1" \
  semantic-release \
  --ci \
  --dry-run \
  --branches "$branch" \
  --repository-url "file://$PWD"
