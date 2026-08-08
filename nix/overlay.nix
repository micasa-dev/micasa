# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Overlay that wraps CI linting/security tools with project-specific
# configuration. Each wrapper shadows the upstream nixpkgs package so
# `pkgs.govulncheck`, `pkgs.deadcode`, etc. include our flags and
# exclusion logic.

_final: prev: {
  # Expose scoped overrides for flake.nix to use explicitly.
  micasaGo = prev.go_1_26;
  micasaBuildGoModule = prev.buildGo126Module;

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
        prev.go_1_26
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
        prev.go_1_26
      ];
      runtimeEnv.CGO_ENABLED = "0";
      text = builtins.readFile ./scripts/nilaway.bash;
    };

  golangci-lint = prev.writeShellApplication {
    name = "golangci-lint";
    runtimeInputs = [
      prev.golangci-lint
      prev.go_1_26
    ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/golangci-lint.bash;
  };

  govulncheck = prev.writeShellApplication {
    name = "govulncheck";
    runtimeInputs = [
      prev.govulncheck
      prev.go_1_26
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
