#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Capture an ad-hoc VHS tape as a PNG screenshot or animated WebP video.
# Reads the output path from the tape's Output directive.
#
# Usage:
#   capture-adhoc <tape-file>           # PNG screenshot (last frame)
#   capture-adhoc --video <tape-file>   # Animated WebP (full recording)

set -euo pipefail

video=false
if [[ "${1:-}" == "--video" ]]; then
  video=true
  shift
fi

if [[ $# -ne 1 ]]; then
  echo "usage: capture-adhoc [--video] <tape-file>" >&2
  exit 1
fi

tape="$1"

webm_path=$(grep -m1 '^Output ' "$tape" | awk '{print $2}')
if [[ -z "$webm_path" || "$webm_path" != *.webm ]]; then
  echo "error: tape must contain an Output directive ending in .webm" >&2
  exit 1
fi

mkdir -p "$(dirname "$webm_path")"
vhs "$tape"

if [[ "$video" == true ]]; then
  webp_path="${webm_path%.webm}.webp"
  ffmpeg -y -i "$webm_path" -c:v libwebp_anim -compression_level 6 -loop 0 "$webp_path"
  rm -f "$webm_path"
  echo "$webp_path"
else
  png_path="${webm_path%.webm}.png"
  ffmpeg -y -sseof -0.04 -i "$webm_path" -frames:v 1 -update 1 "$png_path"
  rm -f "$webm_path"
  echo "$png_path"
fi
