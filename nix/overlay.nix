# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

# Overlay that wraps CI linting/security tools with project-specific
# configuration. Each wrapper shadows the upstream nixpkgs package so
# `pkgs.govulncheck`, `pkgs.deadcode`, etc. include our flags and
# exclusion logic.

_final: prev: {
  # Expose scoped overrides for flake.nix to use explicitly.
  micasaGo = prev.go_1_27;
  micasaBuildGoModule = prev.buildGo127Module;

  # deadcode and nilaway read code with the go/types of the Go that built
  # them, so they must be built with Go 1.27, and nixpkgs builds them with
  # an older Go. The scripts `go run` a pinned version, which Go compiles
  # into its build cache, so Nix never builds them. Renovate updates the
  # pins through the `# renovate:` comments in the scripts.
  deadcode = prev.writeShellApplication {
    name = "deadcode";
    runtimeInputs = [ prev.go_1_27 ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/deadcode.bash;
  };

  nilaway = prev.writeShellApplication {
    name = "nilaway";
    runtimeInputs = [ prev.go_1_27 ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/nilaway.bash;
  };

  golangci-lint = prev.writeShellApplication {
    name = "golangci-lint";
    runtimeInputs = [
      prev.golangci-lint
      prev.go_1_27
    ];
    runtimeEnv.CGO_ENABLED = "0";
    text = builtins.readFile ./scripts/golangci-lint.bash;
  };

  govulncheck = prev.writeShellApplication {
    name = "govulncheck";
    runtimeInputs = [
      prev.govulncheck
      prev.go_1_27
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
