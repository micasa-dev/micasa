#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Run nilaway nil-safety analysis scoped to first-party packages.

set -euo pipefail

# renovate: datasource=go depName=go.uber.org/nilaway
nilaway_version=v0.0.0-20260918162853-acb8859b9031

exec go run "go.uber.org/nilaway/cmd/nilaway@${nilaway_version}" \
  -include-pkgs "github.com/micasa-dev/micasa" \
  -exclude-test-files \
  ./...
