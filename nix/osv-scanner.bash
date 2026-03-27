#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Run osv-scanner with project configuration.

set -euo pipefail

command osv-scanner scan --config osv-scanner.toml --no-ignore --no-call-analysis=go --recursive .
