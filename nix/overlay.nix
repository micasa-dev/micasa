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

  deadcode =
    let
      unwrapped = prev.buildGo127Module {
        pname = "deadcode";
        version = "0.52.0";
        src = prev.fetchFromGitHub {
          owner = "golang";
          repo = "tools";
          rev = "v0.52.0";
          hash = "sha256-invRApL81q9uIUCbBUhWGyS+14OKK6HZYdONzhHGKP4=";
        };
        subPackages = [ "cmd/deadcode" ];
        vendorHash = "sha256-+1/eKOwN4yUvLCgGRJbW6MTleIEodfcIoEHCBRHd2MQ=";
        doCheck = false;
      };
    in
    prev.writeShellApplication {
      name = "deadcode";
      runtimeInputs = [
        unwrapped
        prev.go_1_27
      ];
      runtimeEnv.CGO_ENABLED = "0";
      text = builtins.readFile ./scripts/deadcode.bash;
    };

  nilaway =
    let
      unwrapped = prev.buildGo127Module {
        pname = "nilaway";
        version = "0.0.0-20260918162853-acb8859b9031";
        src = prev.fetchFromGitHub {
          owner = "uber-go";
          repo = "nilaway";
          rev = "acb8859b9031bb9496be97e027df5573f9fb5340";
          hash = "sha256-GvDZ5tlvOrTI93tYcIcLd45ZHdqwFopVtoBffD/kbuM=";
        };
        subPackages = [ "cmd/nilaway" ];
        vendorHash = "sha256-qVmvDneq6V/q5UHZ/Cjjqd5/XPPNfvVGoxwg9nz4/Ds=";
        doCheck = false;
      };
    in
    prev.writeShellApplication {
      name = "nilaway";
      runtimeInputs = [
        unwrapped
        prev.go_1_27
      ];
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
