# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

mkdir -p testdata
[[ -f testdata/sample-text.png ]] && exit 0
magick -size 1x1 xc:white null: 2>/dev/null || { echo "magick xc: delegate unavailable, skipping"; exit 0; }
# Nix sets MICASA_FIXTURE_FONT to a pinned font file. CI runs this script
# without Nix and relies on ImageMagick's default font.
magick -size 612x200 xc:white \
  ${MICASA_FIXTURE_FONT:+-font "$MICASA_FIXTURE_FONT"} \
  -pointsize 24 \
  -fill black \
  -annotate +72+72 "Sample text for OCR testing\nInvoice #5678\nDate: 2025-03-15\nVendor: Test Plumbing Co.\nTotal: \$2,500.00" \
  testdata/sample-text.png
