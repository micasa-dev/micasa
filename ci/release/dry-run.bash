#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0
#
# Simulate a semantic-release run in a disposable worktree.
# Strips @semantic-release/github to avoid any GitHub API calls.

set -euo pipefail

curdir="$PWD"
worktree="$(mktemp -d)"
branch="semantic-release-dry-run-$(basename "$worktree")"

git worktree add -b "$branch" "$worktree" HEAD

cleanup() {
  cd "$curdir"
  git worktree remove --force "$worktree"
  git worktree prune
  if git show-ref --verify --quiet "refs/heads/$branch"; then
    git branch -D "$branch"
  fi
}
trap cleanup EXIT

cd "$worktree"

# Strip @semantic-release/github so the dry-run makes no API calls
tmp=$(mktemp)
jq '.plugins |= map(select(
  if type == "array" then .[0] != "@semantic-release/github"
  else . != "@semantic-release/github"
  end
))' .releaserc.json > "$tmp"
mv "$tmp" .releaserc.json

git add .releaserc.json
git -c user.email=ci@localhost -c user.name=CI \
  commit -m "test: semantic-release dry run" --no-verify --no-gpg-sign

# Unset so semantic-release exercises the full pipeline instead of
# short-circuiting on PR detection
unset GITHUB_ACTIONS

npx --yes \
  -p "semantic-release@25.0.3" \
  -p "@semantic-release/exec@7.1.0" \
  -p "@semantic-release/git@10.0.1" \
  -p "conventional-changelog-conventionalcommits" \
  semantic-release \
  --ci \
  --dry-run \
  --branches "$branch" \
  --repository-url "file://$PWD"
