# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

{
  description = "micasa Go development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable-small";
    flake-utils.url = "github:numtide/flake-utils";
    git-hooks.url = "github:cachix/git-hooks.nix";
    git-hooks.inputs.nixpkgs.follows = "nixpkgs";
    gitignore.url = "github:hercules-ci/gitignore.nix";
    gitignore.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      git-hooks,
      gitignore,
      ...
    }:
    {
      nixosModules.default = import ./nix/module.nix;
    }
    // flake-utils.lib.eachDefaultSystem (
      system:
      let
        inherit (nixpkgs) lib;
        pkgs = import nixpkgs {
          inherit system;
          overlays = [
            (import ./nix/overlay.nix)
          ];
        };
        micasa = pkgs.callPackage ./nix/package.nix {
          buildGoModule = pkgs.micasaBuildGoModule;
          inherit (gitignore.lib) gitignoreSource;
        };

        licenseCheck = pkgs.writeShellApplication {
          name = "license-check";
          runtimeInputs = [
            pkgs.coreutils
            pkgs.gnused
            pkgs.gnugrep
          ];
          text = builtins.readFile ./nix/scripts/license-check.bash;
        };

        preCommit = git-hooks.lib.${system}.run {
          src = ./.;
          hooks = {
            golines = {
              enable = true;
              settings.flags = "--base-formatter=${lib.getExe pkgs.gofumpt} " + "--max-len=100";
            };
            nixfmt.enable = true;
            golangci-lint = {
              enable = false; # CI-only job
              stages = [ "pre-push" ];
            };
            actionlint.enable = true;
            statix.enable = true;
            deadnix.enable = true;
            biome = {
              enable = true;
              excludes = [ "\\.claude/settings\\.json" ];
            };
            taplo.enable = true;
            license-header = {
              enable = true;
              name = "license-header";
              entry = "${lib.getExe licenseCheck}";
              files = "\\.(go|nix|ya?ml|sh|md|js)$|^\\.envrc$|\\.gitignore$|^go\\.mod$";
              excludes = [
                "LICENSE"
                "flake\\.lock"
                "go\\.sum"
                "\\.json$"
                "^docs/content/"
              ];
              language = "system";
              pass_filenames = true;
            };
            go-mod-tidy = {
              enable = true;
              name = "go-mod-tidy";
              entry = "${lib.getExe goModTidyCheck}";
              files = "\\.go$|^go\\.(mod|sum)$";
              language = "system";
              pass_filenames = false;
            };
            deadcode-check = {
              enable = false; # CI-only job
              name = "deadcode";
              entry = "${lib.getExe pkgs.deadcode}";
              files = "\\.go$";
              language = "system";
              pass_filenames = false;
              stages = [ "pre-push" ];
            };
            govulncheck = {
              enable = false; # CI-only job
              name = "govulncheck";
              entry = "${lib.getExe pkgs.govulncheck}";
              files = "^go\\.(mod|sum)$";
              language = "system";
              pass_filenames = false;
              stages = [ "pre-push" ];
            };
            osv-scanner = {
              enable = false; # CI-only job
              name = "osv-scanner";
              entry = "${lib.getExe pkgs.osv-scanner}";
              files = "^go\\.(mod|sum)$";
              language = "system";
              pass_filenames = false;
              stages = [ "pre-push" ];
            };
            go-generate-check = {
              enable = true;
              name = "go-generate-check";
              entry = "${lib.getExe goGenerateCheck}";
              files = "^(cmd/micasa/.*\\.go|internal/(data/(models|cmd/genmeta/main)|app/(coldefs|cmd/gencolumns/main)|config/.*)\\.go|docs/data/deprecations\\.json|docs/content/docs/reference/cli\\.md)$";
              language = "system";
              pass_filenames = false;
              stages = [ "pre-push" ];
            };
            vendor-hash-check = {
              enable = true;
              name = "vendor-hash-check";
              entry = "${lib.getExe vendorHashCheck}";
              files = "^go\\.(mod|sum)$";
              language = "system";
              pass_filenames = false;
            };
          };
        };

        # Fontconfig for VHS recordings using Hack Nerd Font.
        # JetBrains Mono's variable font files cause xterm.js in Chromium to
        # miscalculate cell width, producing visible letter-spacing gaps.
        # Hack Nerd Font renders correctly and includes icon glyphs.
        vhsFontsConf = pkgs.makeFontsConf {
          fontDirectories = [ "${pkgs.nerd-fonts.hack}/share/fonts/truetype" ];
        };

        goModTidyCheck = pkgs.writeShellApplication {
          name = "go-mod-tidy-check";
          runtimeInputs = [
            pkgs.micasaGo
            pkgs.git
          ];
          text = ''
            go mod tidy
            git diff --exit-code go.mod go.sum || {
              echo "go mod tidy modified go.mod/go.sum -- please re-stage" >&2
              exit 1
            }
          '';
        };

        goGenerateCheck = pkgs.writeShellApplication {
          name = "go-generate-check";
          runtimeInputs = [
            pkgs.micasaGo
            pkgs.git
          ];
          runtimeEnv.CGO_ENABLED = "0";
          text = ''
            _tmpdir=$(mktemp -d -t micasa-gogenerate-XXXXXX)
            trap 'chmod -R u+w "$_tmpdir" 2>/dev/null; rm -rf "$_tmpdir"' EXIT
            export GOCACHE="''${GOCACHE:-$_tmpdir/gocache}"
            export GOMODCACHE="''${GOMODCACHE:-$_tmpdir/gomodcache}"
            go generate ./internal/data/
            go generate ./internal/app/
            go generate ./internal/config/
            go generate ./cmd/micasa/
            git diff --exit-code \
              internal/data/meta_generated.go \
              internal/app/columns_generated.go \
              docs/data/deprecations.json \
              docs/content/docs/reference/cli.md || {
              echo "go generate produced changes -- please re-stage the generated files" >&2
              exit 1
            }
          '';
        };

        vendorHashCheck = pkgs.writeShellApplication {
          name = "vendor-hash-check";
          runtimeInputs = [ pkgs.git ];
          text = ''
            if git diff --cached --quiet -- go.sum; then
              exit 0
            fi
            if ! git diff --cached --quiet -- nix/package.nix; then
              exit 0
            fi
            echo "go.sum changed but nix/package.nix is unchanged -- run /update-vendor-hash to update vendorHash" >&2
            exit 1
          '';
        };

        relnotes = pkgs.writeShellApplication {
          name = "relnotes";
          runtimeInputs = [
            pkgs.nodejs
          ];
          text = ''
            npx -y -p conventional-changelog-cli -- conventional-changelog --config ./.conventionalcommits.js --tag-prefix v
          '';
        };

      in
      {
        checks = {
          pre-commit = preCommit;
        };

        devShells.default =
          let
            inherit (preCommit) enabledPackages;
          in
          pkgs.mkShell {
            shellHook = ''
              ${preCommit.shellHook}

              # lipgloss v2 does not detect tmux-256color as truecolor-capable (#789)
              export TERM=xterm-256color

              # Generate test document fixtures if missing.
              if [[ ! -f internal/extract/testdata/mixed-inspection.pdf ]]; then
                bash internal/extract/gen-sample-pdf.bash
                bash internal/extract/gen-invoice-png.bash
                bash internal/extract/gen-sample-text-png.bash
                bash internal/extract/gen-scanned-pdf.bash
                bash internal/extract/gen-mixed-pdf.bash
              fi
            '';
            CGO_ENABLED = "0";
            GOFLAGS = "-trimpath";
            packages = [
              pkgs.micasaGo
              pkgs.osv-scanner
              pkgs.git
              pkgs.hugo
              pkgs.vhs
              pkgs.ripgrep
              pkgs.fd
              pkgs.sd
              pkgs.sqlite-interactive
              pkgs.tesseract
              pkgs.poppler-utils
              pkgs.imagemagick
              pkgs.golangci-lint
              pkgs.gopls
              pkgs.goreleaser
              pkgs.govulncheck
              pkgs.deadcode
              pkgs.nodejs
              pkgs.jq
              pkgs.glow
              relnotes
            ]
            ++ enabledPackages;
          };

        packages = {
          inherit micasa;
          default = micasa;
          docs = pkgs.writeShellApplication {
            name = "micasa-docs";
            runtimeInputs = [
              pkgs.hugo
              pkgs.pagefind
            ];
            text = builtins.readFile ./nix/scripts/docs-build.bash;
          };
          site = pkgs.writeShellApplication {
            name = "micasa-website";
            runtimeInputs = [
              pkgs.hugo
              pkgs.pagefind
            ];
            text = builtins.readFile ./nix/scripts/docs-serve.bash;
          };
          # Records any VHS tape to WebM
          record-tape = pkgs.writeShellApplication {
            name = "record-tape";
            runtimeInputs = [
              micasa
              pkgs.vhs
              pkgs.nerd-fonts.hack
            ];
            runtimeEnv = {
              FONTCONFIG_FILE = "${vhsFontsConf}";
            };
            text = builtins.readFile ./nix/scripts/record-tape.bash;
          };
          record-demo = pkgs.writeShellApplication {
            name = "record-demo";
            runtimeInputs = [
              self.packages.${system}.record-tape
              pkgs.ffmpeg-headless
            ];
            text = ''
              record-tape docs/tapes/demo.tape
              ffmpeg -y -i videos/demo.webm -c:v libwebp_anim -compression_level 6 -loop 0 images/demo.webp
            '';
          };
          # Captures a single VHS tape to a WebP screenshot: capture-one <tape-file>
          capture-one = pkgs.writeShellApplication {
            name = "capture-one";
            runtimeInputs = [
              micasa
              pkgs.vhs
              pkgs.nerd-fonts.hack
              pkgs.ffmpeg-headless
            ];
            runtimeEnv = {
              FONTCONFIG_FILE = "${vhsFontsConf}";
            };
            text = builtins.readFile ./nix/scripts/capture-one.bash;
          };

          # Captures an ad-hoc VHS tape as PNG screenshot or animated WebP video
          capture-adhoc = pkgs.writeShellApplication {
            name = "capture-adhoc";
            runtimeInputs = [
              micasa
              pkgs.vhs
              pkgs.nerd-fonts.hack
              pkgs.ffmpeg-headless
            ];
            runtimeEnv = {
              FONTCONFIG_FILE = "${vhsFontsConf}";
            };
            text = builtins.readFile ./nix/scripts/capture-adhoc.bash;
          };

          # Captures VHS tapes in parallel: capture-screenshots [name ...]
          capture-screenshots = pkgs.writeShellApplication {
            name = "capture-screenshots";
            runtimeInputs = [
              self.packages.${system}.capture-one
              pkgs.fd
            ];
            text = builtins.readFile ./nix/scripts/capture-screenshots.bash;
          };
          # Records all animated demo tapes (using-*, extraction) in parallel
          record-animated = pkgs.writeShellApplication {
            name = "record-animated";
            runtimeInputs = [
              self.packages.${system}.record-tape
              pkgs.fd
            ];
            text = ''
              TAPES="docs/tapes"
              fd -g '{using-*,extraction}.tape' . "$TAPES" \
                -x record-tape {}
            '';
          };
          gen-sample-pdf = pkgs.writeShellApplication {
            name = "gen-sample-pdf";
            text = ''
              bash internal/extract/gen-sample-pdf.bash
            '';
          };
          gen-invoice-png = pkgs.writeShellApplication {
            name = "gen-invoice-png";
            runtimeInputs = [ pkgs.imagemagick ];
            text = ''
              bash internal/extract/gen-invoice-png.bash
            '';
          };
          gen-sample-text-png = pkgs.writeShellApplication {
            name = "gen-sample-text-png";
            runtimeInputs = [ pkgs.imagemagick ];
            text = ''
              bash internal/extract/gen-sample-text-png.bash
            '';
          };
          gen-scanned-pdf = pkgs.writeShellApplication {
            name = "gen-scanned-pdf";
            runtimeInputs = [ pkgs.imagemagick ];
            text = ''
              bash internal/extract/gen-scanned-pdf.bash
            '';
          };
          gen-mixed-pdf = pkgs.writeShellApplication {
            name = "gen-mixed-pdf";
            runtimeInputs = [ pkgs.poppler-utils ];
            text = ''
              bash internal/extract/gen-mixed-pdf.bash
            '';
          };
          gen-testdata = pkgs.writeShellApplication {
            name = "gen-testdata";
            runtimeInputs = [
              self.packages.${system}.gen-sample-pdf
              self.packages.${system}.gen-invoice-png
              self.packages.${system}.gen-sample-text-png
              self.packages.${system}.gen-scanned-pdf
              self.packages.${system}.gen-mixed-pdf
            ];
            text = ''
              gen-sample-pdf
              gen-invoice-png
              gen-sample-text-png
              gen-scanned-pdf
              gen-mixed-pdf
            '';
          };
          inherit (pkgs)
            deadcode
            golangci-lint
            govulncheck
            osv-scanner
            ;
          coverage = pkgs.writeShellApplication {
            name = "coverage";
            runtimeInputs = [
              pkgs.micasaGo
              pkgs.sd
            ];
            runtimeEnv.CGO_ENABLED = "1";
            text = ''
              go test -coverprofile cover.out ./...
              go tool cover -func cover.out \
                | sd '^github.com/micasa-dev/micasa/' "" \
                | column -t
            '';
          };
          run-pre-commit = pkgs.writeShellApplication {
            name = "run-pre-commit";
            runtimeInputs = [
              pkgs.micasaGo
              pkgs.git
            ]
            ++ preCommit.enabledPackages;
            excludeShellChecks = [
              # shellHook from git-hooks.lib contains patterns that
              # trigger these warnings; the code is upstream-generated.
              "SC2006"
              "SC2043"
              "SC2086"
              "SC2157"
              "SC2221"
              "SC2222"
              "SC2295"
            ];
            text = ''
              ${preCommit.shellHook}
              if [ $# -eq 0 ]; then
                set -- --all-files
              fi
              pre-commit run "$@"
              pre-commit run "$@" --hook-stage pre-push
            '';
          };

        };

        apps =
          let
            app = drv: desc: flake-utils.lib.mkApp { inherit drv; } // { meta.description = desc; };
            p = self.packages.${system};
          in
          {
            default = app micasa "Terminal UI for home maintenance";
            site = app p.site "Start local Hugo dev server";
            record-tape = app p.record-tape "Record a VHS tape to WebM";
            record-demo = app p.record-demo "Record the main demo tape";
            capture-adhoc = app p.capture-adhoc "Capture an ad-hoc VHS tape screenshot or video";
            capture-one = app p.capture-one "Capture a VHS tape screenshot";
            capture-screenshots = app p.capture-screenshots "Capture all VHS screenshots in parallel";
            record-animated = app p.record-animated "Record all animated demo tapes";
            gen-sample-pdf = app p.gen-sample-pdf "Generate sample.pdf test fixture";
            gen-invoice-png = app p.gen-invoice-png "Generate invoice.png test fixture";
            gen-sample-text-png = app p.gen-sample-text-png "Generate sample-text.png test fixture";
            gen-scanned-pdf = app p.gen-scanned-pdf "Generate scanned-invoice.pdf test fixture";
            gen-mixed-pdf = app p.gen-mixed-pdf "Generate mixed-inspection.pdf test fixture";
            gen-testdata = app p.gen-testdata "Generate all test document fixtures";
            deadcode = app p.deadcode "Run whole-program dead code analysis";
            govulncheck = app p.govulncheck "Check for known Go vulnerabilities with call-graph analysis";
            osv-scanner = app p.osv-scanner "Scan for known vulnerabilities";
            golangci-lint = app p.golangci-lint "Run golangci-lint";
            pre-commit = app p.run-pre-commit "Run all pre-commit hooks";
          };

        formatter = pkgs.nixpkgs-fmt;
      }
    );
}
