#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Build the Hugo site with pagefind search index.

set -euo pipefail

mkdir -p docs/static/images docs/static/videos
cp images/favicon.svg docs/static/images/favicon.svg
cp videos/demo.webm docs/static/videos/demo.webm
rm -rf website
hugo --source docs --destination ../website \
  --minify \
  --gc \
  --noBuildLock \
  --noChmod \
  --noTimes \
  --printPathWarnings \
  --panicOnWarning
pagefind --site website \
  --quiet \
  --force-language en
