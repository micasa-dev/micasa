# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Overlay that wraps CI linting/security tools with project-specific
# configuration. Each wrapper shadows the upstream nixpkgs package so
# `pkgs.govulncheck`, `pkgs.deadcode`, etc. include our flags and
# exclusion logic.

final: prev:
let
  go = final.go_1_26;
in
{
  deadcode =
    let
      unwrapped = (prev.buildGoModule.override { inherit go; }) {
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
        go
      ];
      runtimeEnv.CGO_ENABLED = "0";
      text = builtins.readFile ./deadcode.bash;
    };

  golangci-lint = prev.writeShellApplication {
    name = "golangci-lint";
    runtimeInputs = [
      prev.golangci-lint
      go
    ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./golangci-lint.bash;
  };

  govulncheck = prev.writeShellApplication {
    name = "govulncheck";
    runtimeInputs = [
      prev.govulncheck
      go
      prev.jq
      prev.ripgrep
    ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./govulncheck.bash;
  };

  osv-scanner = prev.writeShellApplication {
    name = "osv-scanner";
    runtimeInputs = [ prev.osv-scanner ];
    text = builtins.readFile ./osv-scanner.bash;
  };
}
