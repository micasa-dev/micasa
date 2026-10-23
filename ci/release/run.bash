#!/usr/bin/env bash
# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0
#
# Run semantic-release to publish a new release.

set -euo pipefail

npx --yes \
  -p "semantic-release@25.0.9" \
  -p "@semantic-release/exec@7.1.0" \
  -p "@semantic-release/git@11.0.1" \
  -p "conventional-changelog-conventionalcommits@9.3.1" \
  -p "lodash-es@4.18.1" \
  semantic-release --ci
