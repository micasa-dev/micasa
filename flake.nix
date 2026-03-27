# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

{
  description = "micasa Go development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/2d6af1fa6349e4c3accef90d7e356d4300ce221c"; # nixos-unstable-small 2026-03-26
    flake-utils.url = "github:numtide/flake-utils";
    git-hooks.url = "github:cachix/git-hooks.nix";
    git-hooks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      git-hooks,
      ...
    }:
    {
      nixosModules.default = import ./nix/module.nix;
    }
    // flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [
            (final: prev: {
              govulncheck = prev.writeShellApplication {
                name = "govulncheck";
                runtimeInputs = [
                  prev.govulncheck
                  final.go_1_26
                  prev.jq
                  prev.ripgrep
                ];
                runtimeEnv.CGO_ENABLED = "0";
                text = ''
                  _tmpdir=$(mktemp -d -t micasa-govulncheck-XXXXXX)
                  trap 'chmod -R u+w "$_tmpdir" 2>/dev/null; rm -rf "$_tmpdir"' EXIT
                  export GOCACHE="''${GOCACHE:-$_tmpdir/gocache}"
                  export GOMODCACHE="''${GOMODCACHE:-$_tmpdir/gomodcache}"

                  exclude_file=".govulncheck-exclude"
                  raw=$(command govulncheck -format json ./... 2>&1) || true
                  found=$(echo "$raw" | jq -r 'select(.finding) | select(.finding.trace[0].function) | .finding.osv' | sort -u)

                  if [ -z "$found" ]; then
                    exit 0
                  fi

                  excluded=""
                  if [ -f "$exclude_file" ]; then
                    excluded=$(rg -oN 'GO-[0-9]+-[0-9]+' "$exclude_file" | sort -u)
                  fi

                  new=$(comm -23 <(echo "$found") <(echo "$excluded"))

                  if [ -z "$new" ]; then
                    exit 0
                  fi

                  echo "govulncheck: unexcluded vulnerabilities found:"
                  echo "$new"
                  exit 1
                '';
              };
            })
          ];
        };
        go = pkgs.go_1_26;
        version = builtins.replaceStrings [ "\n" "\r" ] [ "" "" ] (builtins.readFile ./VERSION);

        buildGoModule = pkgs.buildGoModule.override { inherit go; };

        micasa = buildGoModule {
          pname = "micasa";
          inherit version;
          src = ./.;
          subPackages = [ "cmd/micasa" ];
          vendorHash = "sha256-I4EUIFGbJ9Q3IUFKlF+YiJF84fDiIfXbUQMiDntENZU=";
          env.CGO_ENABLED = 0;
          preCheck = ''
            export HOME="$(mktemp -d)"
          '';
          ldflags = [
            "-X main.version=${version}"
          ];
        };

        licenseCheck = pkgs.writeShellScript "license-check" ''
          head=${pkgs.coreutils}/bin/head
          sed=${pkgs.gnused}/bin/sed
          grep=${pkgs.gnugrep}/bin/grep
          basename=${pkgs.coreutils}/bin/basename
          date=${pkgs.coreutils}/bin/date

          year=$($date +%Y)
          owner="Phillip Cloud"
          spdx="Licensed under the Apache License, Version 2.0"

          comment_prefix() {
            case "$1" in
              *.go|go.mod|*.js) echo "//" ;;
              *.nix|*.yml|*.yaml|*.sh|.envrc|.gitignore) echo "#" ;;
              *.md)         echo "md" ;;
              *)            echo "#" ;;
            esac
          }

          status=0
          for f in "$@"; do
            name=$($basename "$f")
            pfx=$(comment_prefix "$name")

            if [ "$pfx" = "md" ]; then
              line1="<!-- Copyright $year $owner -->"
              line2="<!-- $spdx -->"
              year_pat="<!-- Copyright [0-9]\{4\} $owner -->"
            else
              line1="$pfx Copyright $year $owner"
              line2="$pfx $spdx"
              year_pat="$pfx Copyright [0-9]\{4\} $owner"
            fi

            first=$($head -n1 "$f")

            # Shebang-aware: if first line is #!, check lines 2-3 instead
            if echo "$first" | $grep -q '^#!'; then
              check1=$($sed -n '2p' "$f")
              check2=$($sed -n '3p' "$f")
              insert_line=1  # insert after line 1 (the shebang)
            else
              check1="$first"
              check2=$($sed -n '2p' "$f")
              insert_line=0  # insert before line 1
            fi

            # Already correct
            if [ "$check1" = "$line1" ] && [ "$check2" = "$line2" ]; then
              continue
            fi

            # Header present with stale year -- bump it
            if echo "$check1" | $grep -q "^$year_pat$" \
               && [ "$check2" = "$line2" ]; then
              $sed -i "s|$year_pat|$line1|" "$f"
              echo "bumped year in $f"
              continue
            fi

            # No header -- insert it
            if [ "$insert_line" -eq 0 ]; then
              $sed -i "1i\\$line1\n$line2\n" "$f"
            else
              $sed -i "1a\\$line1\n$line2" "$f"
            fi
            echo "added license header to $f"
            status=1
          done
          exit $status
        '';

        preCommit = git-hooks.lib.${system}.run {
          src = ./.;
          hooks = {
            golines = {
              enable = true;
              settings.flags = "--base-formatter=${pkgs.gofumpt}/bin/gofumpt " + "--max-len=100";
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
              entry = "${licenseCheck}";
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
              entry = "${goModTidyCheck}/bin/go-mod-tidy-check";
              files = "\\.go$|^go\\.(mod|sum)$";
              language = "system";
              pass_filenames = false;
            };
            deadcode-check = {
              enable = false; # CI-only job
              name = "deadcode";
              entry = "${run-deadcode}/bin/run-deadcode";
              files = "\\.go$";
              language = "system";
              pass_filenames = false;
              stages = [ "pre-push" ];
            };
            govulncheck = {
              enable = false; # CI-only job
              name = "govulncheck";
              entry = "${pkgs.govulncheck}/bin/govulncheck";
              files = "^go\\.(mod|sum)$";
              language = "system";
              pass_filenames = false;
              stages = [ "pre-push" ];
            };
            osv-scanner = {
              enable = false; # CI-only job
              name = "osv-scanner";
              entry = "${run-osv-scanner}/bin/run-osv-scanner";
              files = "^go\\.(mod|sum)$";
              language = "system";
              pass_filenames = false;
              stages = [ "pre-push" ];
            };
            go-generate-check = {
              enable = true;
              name = "go-generate-check";
              entry = "${goGenerateCheck}/bin/go-generate-check";
              files = "^internal/(data/(models|cmd/genmeta/main)|app/(coldefs|cmd/gencolumns/main))\\.go$";
              language = "system";
              pass_filenames = false;
              stages = [ "pre-push" ];
            };
            vendor-hash-check = {
              enable = true;
              name = "vendor-hash-check";
              entry = "${vendorHashCheck}/bin/vendor-hash-check";
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

        deadcode = buildGoModule {
          pname = "deadcode";
          version = "0.43.0";
          src = pkgs.fetchFromGitHub {
            owner = "golang";
            repo = "tools";
            rev = "v0.43.0";
            hash = "sha256-A4c+/kWJQ6/3dIu8lR/NW9HUvsrIVs255lPfBYWK3tE=";
          };
          subPackages = [ "cmd/deadcode" ];
          vendorHash = "sha256-+tJs+0exGSauZr7PBuXf0htoiLST5GVMiP2lEFpd4A4=";
          doCheck = false;
        };

        run-deadcode = pkgs.writeShellApplication {
          name = "run-deadcode";
          runtimeInputs = [
            deadcode
            go
          ];
          runtimeEnv.CGO_ENABLED = "0";
          text = ''
            _tmpdir=$(mktemp -d -t micasa-deadcode-XXXXXX)
            trap 'chmod -R u+w "$_tmpdir" 2>/dev/null; rm -rf "$_tmpdir"' EXIT
            export GOCACHE="''${GOCACHE:-$_tmpdir/gocache}"
            export GOMODCACHE="''${GOMODCACHE:-$_tmpdir/gomodcache}"
            output=$(deadcode -generated -test "$@" ./...)
            if [ -n "$output" ]; then
              echo "$output"
              exit 1
            fi
          '';
        };

        run-osv-scanner = pkgs.writeShellApplication {
          name = "run-osv-scanner";
          runtimeInputs = [ pkgs.osv-scanner ];
          text = ''
            osv-scanner scan --config osv-scanner.toml --no-ignore --no-call-analysis=go --recursive .
          '';
        };

        run-golangci-lint = pkgs.writeShellApplication {
          name = "run-golangci-lint";
          runtimeInputs = [
            pkgs.golangci-lint
            go
          ];
          runtimeEnv.CGO_ENABLED = "0";
          text = ''
            _tmpdir=$(mktemp -d -t micasa-golangci-lint-XXXXXX)
            trap 'chmod -R u+w "$_tmpdir" 2>/dev/null; rm -rf "$_tmpdir"' EXIT
            export GOCACHE="''${GOCACHE:-$_tmpdir/gocache}"
            export GOMODCACHE="''${GOMODCACHE:-$_tmpdir/gomodcache}"
            golangci-lint run ./...
          '';
        };

        goModTidyCheck = pkgs.writeShellApplication {
          name = "go-mod-tidy-check";
          runtimeInputs = [
            go
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
            go
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
            git diff --exit-code internal/data/meta_generated.go internal/app/columns_generated.go || {
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
            if ! git diff --cached --quiet -- flake.nix; then
              exit 0
            fi
            echo "go.sum changed but flake.nix is unchanged -- run /update-vendor-hash to update vendorHash" >&2
            exit 1
          '';
        };

        relnotes = pkgs.writeShellApplication {
          name = "relnotes";
          runtimeInputs = [
            pkgs.nodejs
            pkgs.glow
            pkgs.ncurses
            pkgs.less
          ];
          text = ''
            notes=$(npx -y -p conventional-changelog-cli -- conventional-changelog --config ./.conventionalcommits.js --tag-prefix v)
            if [[ -n "$notes" ]] && [[ -t 1 ]]; then
              echo "$notes" | glow --width "$(tput cols)" - | less -FRX
            else
              echo "$notes"
            fi
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
              go
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
              deadcode
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
            text = ''
              mkdir -p docs/static/images docs/static/videos
              cp images/favicon.svg docs/static/images/favicon.svg
              cp videos/demo.webm docs/static/videos/demo.webm
              rm -rf website
              hugo --source docs --destination ../website \
                --minify \
                --gc \
                --noBuildLock \
                --noChmod \
                --noTimes \
                --printPathWarnings \
                --panicOnWarning
              pagefind --site website \
                --quiet \
                --force-language en
            '';
          };
          site = pkgs.writeShellApplication {
            name = "micasa-website";
            runtimeInputs = [
              pkgs.hugo
              pkgs.pagefind
            ];
            text = ''
              mkdir -p docs/static/images docs/static/videos
              cp images/favicon.svg docs/static/images/favicon.svg
              cp videos/demo.webm docs/static/videos/demo.webm

              # Build once to generate the pagefind index, then copy it
              # into docs/static/ so hugo server serves it as a static asset.
              _tmpsite=$(mktemp -d)
              hugo --source docs --destination "$_tmpsite" --buildDrafts --buildFuture --minify --noBuildLock --quiet
              pagefind --site "$_tmpsite" --quiet
              rm -rf docs/static/pagefind
              cp -r "$_tmpsite/pagefind" docs/static/pagefind
              rm -rf "$_tmpsite"

              _port=$((RANDOM % 10000 + 30000))
              printf 'http://localhost:%s\n' "$_port"
              exec hugo server --source docs --buildDrafts --buildFuture --disableFastRender --noHTTPCache --port "$_port" --bind 0.0.0.0 &>/dev/null
            '';
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
            text = ''
              if [[ $# -ne 1 ]]; then
                echo "usage: record-tape <tape-file>" >&2
                exit 1
              fi

              tape="$1"

              webm_path=$(grep -m1 '^Output ' "$tape" | awk '{print $2}')
              if [[ -z "$webm_path" || "$webm_path" != *.webm ]]; then
                echo "error: tape must contain an Output directive ending in .webm" >&2
                exit 1
              fi

              mkdir -p "$(dirname "$webm_path")"
              vhs "$tape"
            '';
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
            text = ''
              if [[ $# -ne 1 ]]; then
                echo "usage: capture-one <tape-file>" >&2
                exit 1
              fi

              tape="$1"
              name="$(basename "$tape" .tape)"
              OUT="docs/static/images"
              mkdir -p "$OUT"

              vhs "$tape"

              # Extract last frame from WebM as lossless WebP
              ffmpeg -y -sseof -0.04 -i "$OUT/$name.webm" -frames:v 1 -c:v libwebp -lossless 1 "$OUT/$name.webp"
              rm -f "$OUT/$name.webm"

              echo "$name -> $OUT/$name.webp"
            '';
          };

          # Captures VHS tapes in parallel: capture-screenshots [name ...]
          capture-screenshots = pkgs.writeShellApplication {
            name = "capture-screenshots";
            runtimeInputs = [
              self.packages.${system}.capture-one
              pkgs.fd
            ];
            text = ''
              TAPES="docs/tapes"

              if [[ $# -gt 0 ]]; then
                for name in "$@"; do
                  capture-one "$TAPES/$name.tape" &
                done
                wait
                exit
              fi

              # All tapes in parallel (skip demo, using-*, and extraction animated tapes)
              fd -e tape --exclude demo.tape --exclude 'using-*.tape' --exclude extraction.tape . "$TAPES" \
                -x capture-one {}
            '';
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
          inherit
            run-deadcode
            run-osv-scanner
            run-golangci-lint
            ;
          coverage = pkgs.writeShellApplication {
            name = "coverage";
            runtimeInputs = [
              go
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
              go
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
            pkg = name: self.packages.${system}.${name};
          in
          {
            default = app micasa "Terminal UI for home maintenance";
            site = app (pkg "site") "Start local Hugo dev server";
            record-tape = app (pkg "record-tape") "Record a VHS tape to WebM";
            record-demo = app (pkg "record-demo") "Record the main demo tape";
            capture-one = app (pkg "capture-one") "Capture a VHS tape screenshot";
            capture-screenshots = app (pkg "capture-screenshots") "Capture all VHS screenshots in parallel";
            record-animated = app (pkg "record-animated") "Record all animated demo tapes";
            gen-sample-pdf = app (pkg "gen-sample-pdf") "Generate sample.pdf test fixture";
            gen-invoice-png = app (pkg "gen-invoice-png") "Generate invoice.png test fixture";
            gen-sample-text-png = app (pkg "gen-sample-text-png") "Generate sample-text.png test fixture";
            gen-scanned-pdf = app (pkg "gen-scanned-pdf") "Generate scanned-invoice.pdf test fixture";
            gen-mixed-pdf = app (pkg "gen-mixed-pdf") "Generate mixed-inspection.pdf test fixture";
            gen-testdata = app (pkg "gen-testdata") "Generate all test document fixtures";
            deadcode = app (pkg "run-deadcode") "Run whole-program dead code analysis";
            govulncheck = {
              type = "app";
              program = "${pkgs.govulncheck}/bin/govulncheck";
              meta.description = "Check for known Go vulnerabilities with call-graph analysis";
            };
            osv-scanner = app (pkg "run-osv-scanner") "Scan for known vulnerabilities";
            golangci-lint = app (pkg "run-golangci-lint") "Run golangci-lint";
            pre-commit = app (pkg "run-pre-commit") "Run all pre-commit hooks";
          };

        formatter = pkgs.nixpkgs-fmt;
      }
    );
}
