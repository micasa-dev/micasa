#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Capture VHS tapes to WebP screenshots in parallel.
# Usage: capture-screenshots [name ...]
# With no arguments, captures all tapes (excluding demo and animated ones).

set -euo pipefail

TAPES="docs/tapes"

if [[ $# -gt 0 ]]; then
  for name in "$@"; do
    capture-one "$TAPES/$name.tape" &
  done
  wait
  exit
fi

# All tapes in parallel (skip demo, using-*, and extraction animated tapes)
fd -e tape --exclude demo.tape --exclude 'using-*.tape' --exclude extraction.tape . "$TAPES" \
  -x capture-one {}
