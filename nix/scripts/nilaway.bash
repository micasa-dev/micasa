#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Run nilaway nil-safety analysis scoped to first-party packages.

set -euo pipefail

exec nilaway \
  -include-pkgs "github.com/micasa-dev/micasa" \
  -exclude-test-files \
  ./...
