# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Overlay that wraps CI linting/security tools with project-specific
# configuration. Each wrapper shadows the upstream nixpkgs package so
# `pkgs.govulncheck`, `pkgs.deadcode`, etc. include our flags and
# exclusion logic.

final: prev: {
  # Go 1.27.4 fixes GO-2026-6859 (net/http MaxBytesReader header race).
  # nixpkgs still ships 1.27.3; drop this override once it catches up.
  go_1_27 = prev.go_1_27.overrideAttrs (_: rec {
    version = "1.27.4";
    src = prev.fetchurl {
      url = "https://go.dev/dl/go${version}.src.tar.gz";
      hash = "sha256-c8+4VxrsDAEXdofbhxY9Rqog6XeWcncokaoXZU2aYJk=";
    };
  });

  # Expose scoped overrides for flake.nix to use explicitly.
  micasaGo = final.go_1_27;
  micasaBuildGoModule = final.buildGo127Module;

  # deadcode and nilaway read code with the go/types of the Go that built
  # them, so they must be built with Go 1.27, and nixpkgs builds them with
  # an older Go. The scripts `go run` a pinned version, which Go compiles
  # into its build cache, so Nix never builds them. Renovate updates the
  # pins through the `# renovate:` comments in the scripts.
  deadcode = prev.writeShellApplication {
    name = "deadcode";
    runtimeInputs = [ final.go_1_27 ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/deadcode.bash;
  };

  nilaway = prev.writeShellApplication {
    name = "nilaway";
    runtimeInputs = [ final.go_1_27 ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/nilaway.bash;
  };

  golangci-lint = prev.writeShellApplication {
    name = "golangci-lint";
    runtimeInputs = [
      prev.golangci-lint
      final.go_1_27
    ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/golangci-lint.bash;
  };

  govulncheck = prev.writeShellApplication {
    name = "govulncheck";
    runtimeInputs = [
      prev.govulncheck
      final.go_1_27
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
