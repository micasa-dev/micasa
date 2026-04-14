# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Overlay that wraps CI linting/security tools with project-specific
# configuration. Each wrapper shadows the upstream nixpkgs package so
# `pkgs.govulncheck`, `pkgs.deadcode`, etc. include our flags and
# exclusion logic.

_final: prev:
let
  # Scoped Go 1.26.2 override for micasa and its dev tools only.
  # NOT exported as go/go_1_26/buildGoModule — doing so rebuilds the
  # entire transitive closure from source (VHS → Chromium → PipeWire →
  # ffmpeg/gstreamer) because every Go derivation's input hash changes.
  #
  # 1.26.2 fixes five stdlib vulnerabilities flagged by govulncheck:
  #   GO-2026-4865 (html/template JsBraceDepth XSS)
  #   GO-2026-4866 (crypto/x509 excludedSubtrees auth bypass)
  #   GO-2026-4870 (crypto/tls KeyUpdate DoS)
  #   GO-2026-4946 (crypto/x509 inefficient policy validation)
  #   GO-2026-4947 (crypto/x509 unexpected work during chain building)
  # Drop this override once nixpkgs picks up Go 1.26.2.
  patchedGo = prev.go_1_26.overrideAttrs (_: rec {
    version = "1.26.2";
    src = prev.fetchurl {
      url = "https://go.dev/dl/go${version}.src.tar.gz";
      hash = "sha256-LpHrtpR6lulDb7KzkmqIAu/mOm03Xf/sT4Kqnb1v1Ds=";
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
