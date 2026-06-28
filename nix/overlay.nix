# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Overlay that wraps CI linting/security tools with project-specific
# configuration. Each wrapper shadows the upstream nixpkgs package so
# `pkgs.govulncheck`, `pkgs.deadcode`, etc. include our flags and
# exclusion logic.

_final: prev:
let
  # Scoped Go 1.26.4 override for micasa and its dev tools only.
  # NOT exported as go/go_1_26/buildGoModule — doing so rebuilds the
  # entire transitive closure from source (VHS → Chromium → PipeWire →
  # ffmpeg/gstreamer) because every Go derivation's input hash changes.
  #
  # 1.26.4 fixes stdlib vulnerabilities flagged by govulncheck:
  #   GO-2026-4918 (net/http HTTP/2 SETTINGS frame infinite loop)
  #   GO-2026-4971 (net Dial/LookupPort panic on NUL input on Windows)
  #   GO-2026-4977 (net/mail consumePhrase DoS)
  #   GO-2026-4980 (html/template empty <script type=> escape bug)
  #   GO-2026-4982 (html/template <meta> URL escape gap)
  #   GO-2026-4986 (net/mail parsing CPU/memory exhaustion)
  #   GO-2026-5037
  #   GO-2026-5039
  # Drop this override once nixpkgs picks up Go 1.26.4.
  patchedGo = prev.go_1_26.overrideAttrs (_: rec {
    version = "1.26.4";
    src = prev.fetchurl {
      url = "https://go.dev/dl/go${version}.src.tar.gz";
      hash = "sha256-T2aKMvv8ETLmqIH7lowvHa2mMUkqM5IRc1+7JVpCYC0=";
    };
  });
in
{
  # Expose scoped overrides for flake.nix to use explicitly.
  micasaGo = patchedGo;
  micasaBuildGoModule = prev.buildGo126Module.override { go = patchedGo; };

  deadcode =
    let
      unwrapped = prev.buildGoModule {
        pname = "deadcode";
        version = "0.43.0";
        src = prev.fetchFromGitHub {
          owner = "golang";
          repo = "tools";
          rev = "v0.43.0";
          hash = "sha256-A4c+/kWJQ6/3dIu8lR/NW9HUvsrIVs255lPfBYWK3tE=";
        };
        subPackages = [ "cmd/deadcode" ];
        vendorHash = "sha256-+tJs+0exGSauZr7PBuXf0htoiLST5GVMiP2lEFpd4A4=";
        doCheck = false;
      };
    in
    prev.writeShellApplication {
      name = "deadcode";
      runtimeInputs = [
        unwrapped
        patchedGo
      ];
      runtimeEnv.CGO_ENABLED = "0";
      text = builtins.readFile ./scripts/deadcode.bash;
    };

  nilaway =
    let
      unwrapped = prev.buildGoModule {
        pname = "nilaway";
        version = "0.0.0-20260318203545-ad240b12fb4c";
        src = prev.fetchFromGitHub {
          owner = "uber-go";
          repo = "nilaway";
          rev = "ad240b12fb4c370017eb413f0388c71f3be8722c";
          hash = "sha256-XCK3qpV73Rjib8FBM0GpNOGXpUjcscMMUuHU/IVAv7s=";
        };
        subPackages = [ "cmd/nilaway" ];
        vendorHash = "sha256-BztW64NfWbgPk237F8fHDKaAuDkCgNB9QEIKDrwk50g=";
        doCheck = false;
      };
    in
    prev.writeShellApplication {
      name = "nilaway";
      runtimeInputs = [
        unwrapped
        patchedGo
      ];
      runtimeEnv.CGO_ENABLED = "0";
      text = builtins.readFile ./scripts/nilaway.bash;
    };

  golangci-lint = prev.writeShellApplication {
    name = "golangci-lint";
    runtimeInputs = [
      prev.golangci-lint
      patchedGo
    ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/golangci-lint.bash;
  };

  govulncheck = prev.writeShellApplication {
    name = "govulncheck";
    runtimeInputs = [
      prev.govulncheck
      patchedGo
      prev.jq
      prev.ripgrep
    ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/govulncheck.bash;
  };

  osv-scanner = prev.writeShellApplication {
    name = "osv-scanner";
    runtimeInputs = [ prev.osv-scanner ];
    text = builtins.readFile ./scripts/osv-scanner.bash;
  };
}
