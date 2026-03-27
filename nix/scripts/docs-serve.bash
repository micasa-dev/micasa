#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Start the Hugo dev server with pagefind pre-built.

set -euo pipefail

mkdir -p docs/static/images docs/static/videos
cp images/favicon.svg docs/static/images/favicon.svg
cp videos/demo.webm docs/static/videos/demo.webm

# Build once to generate the pagefind index, then copy it
# into docs/static/ so hugo server serves it as a static asset.
_tmpsite=$(mktemp -d)
hugo --source docs --destination "$_tmpsite" --buildDrafts --buildFuture --minify --noBuildLock --quiet
pagefind --site "$_tmpsite" --quiet
rm -rf docs/static/pagefind
cp -r "$_tmpsite/pagefind" docs/static/pagefind
rm -rf "$_tmpsite"

_port=$((RANDOM % 10000 + 30000))
printf 'http://localhost:%s\n' "$_port"
exec hugo server --source docs --buildDrafts --buildFuture --disableFastRender --noHTTPCache --port "$_port" --bind 0.0.0.0 &>/dev/null
