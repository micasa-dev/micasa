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
))' .releaserc.json | sponge .releaserc.json

git add .releaserc.json
git commit -m "test: semantic-release dry run" --no-verify --no-gpg-sign

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
