+++
title = "Installation"
weight = 1
description = "How to install micasa on your system."
linkTitle = "Installation"
+++

micasa is a single static binary with no runtime dependencies. Pick whichever
method suits you.

## Homebrew

Available on macOS and Linux via [Homebrew](https://brew.sh):

```sh
brew install micasa
```

## Go install

Requires Go 1.25+:

```sh
go install github.com/cpcloud/micasa/cmd/micasa@latest
```

This installs the binary into your `$GOBIN` (usually `~/go/bin`).

## Pre-built binaries

Download from the
[latest release](https://github.com/cpcloud/micasa/releases/latest). Binaries
are available for:

| OS      | Architectures    |
|---------|------------------|
| Linux   | amd64, arm64     |
| macOS   | amd64, arm64     |
| Windows | amd64, arm64     |

Each release includes a `checksums.txt` file for verification.

## Nix

If you use [Nix](https://nixos.org) with flakes:

```sh
# Run directly
nix run github:cpcloud/micasa

# Or add to a flake
{
  inputs.micasa.url = "github:cpcloud/micasa";
}
```

## Container

A container image is published to GitHub Container Registry on each release:

```sh
docker pull ghcr.io/cpcloud/micasa:latest
docker run -it --rm ghcr.io/cpcloud/micasa:latest demo
```

> **Note:** micasa is a terminal UI, so you need `-it` (interactive + TTY) for
> the container to work properly.

## Verify it works

```sh
micasa --help
```

You should see the usage output with available flags.
