# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

mkdir -p testdata
[[ -f testdata/mixed-inspection.pdf ]] && exit 0
command -v pdfunite &>/dev/null || { echo "pdfunite not found, skipping"; exit 0; }
pdfunite \
  testdata/sample.pdf \
  testdata/scanned-invoice.pdf \
  testdata/mixed-inspection.pdf
