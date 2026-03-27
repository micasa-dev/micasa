#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Record a VHS tape to WebM.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: record-tape <tape-file>" >&2
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
