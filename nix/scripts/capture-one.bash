#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Capture a single VHS tape to a WebP screenshot (last frame).

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: capture-one <tape-file>" >&2
  exit 1
fi

tape="$1"
name="$(basename "$tape" .tape)"
OUT="docs/static/images"
mkdir -p "$OUT"

vhs "$tape"

# Extract last frame from WebM as lossless WebP
ffmpeg -y -sseof -0.04 -i "$OUT/$name.webm" -frames:v 1 -c:v libwebp -lossless 1 "$OUT/$name.webp"
rm -f "$OUT/$name.webm"

echo "$name -> $OUT/$name.webp"
