# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Overlay that wraps CI linting/security tools with project-specific
# configuration. Each wrapper shadows the upstream nixpkgs package so
# `pkgs.govulncheck`, `pkgs.deadcode`, etc. include our flags and
# exclusion logic.

final: prev: {
  # Override Go to 1.26.2 (nixpkgs nixos-unstable-small currently ships
  # 1.26.1). 1.26.2 fixes five stdlib vulnerabilities flagged by
  # govulncheck:
  #   GO-2026-4865 (html/template JsBraceDepth XSS)
  #   GO-2026-4866 (crypto/x509 excludedSubtrees auth bypass)
  #   GO-2026-4870 (crypto/tls KeyUpdate DoS)
  #   GO-2026-4946 (crypto/x509 inefficient policy validation)
  #   GO-2026-4947 (crypto/x509 unexpected work during chain building)
  # Drop this override once nixpkgs picks up Go 1.26.2. Both `go_1_26`
  # and `go` must be overridden because `buildGoModule` resolves through
  # `go_1_26`, not the unversioned `go` alias.
  go_1_26 = prev.go_1_26.overrideAttrs (_: rec {
    version = "1.26.2";
    src = prev.fetchurl {
      url = "https://go.dev/dl/go${version}.src.tar.gz";
      hash = "sha256-LpHrtpR6lulDb7KzkmqIAu/mOm03Xf/sT4Kqnb1v1Ds=";
    };
  });
  go = final.go_1_26;
  buildGo126Module = prev.buildGo126Module.override { go = final.go_1_26; };
  buildGoModule = final.buildGo126Module;

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
        final.go
      ];
      runtimeEnv.CGO_ENABLED = "0";
      text = builtins.readFile ./scripts/deadcode.bash;
    };

  golangci-lint = prev.writeShellApplication {
    name = "golangci-lint";
    runtimeInputs = [
      prev.golangci-lint
      final.go
    ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/golangci-lint.bash;
  };

  govulncheck = prev.writeShellApplication {
    name = "govulncheck";
    runtimeInputs = [
      prev.govulncheck
      final.go
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
