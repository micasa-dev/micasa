# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

{
  description = "micasa Go development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable-small";
    flake-utils.url = "github:numtide/flake-utils";
    git-hooks.url = "github:cachix/git-hooks.nix";
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
        pkgs = import nixpkgs { inherit system; };
        version = builtins.replaceStrings [ "\n" "\r" ] [ "" "" ] (builtins.readFile ./VERSION);

        micasa = pkgs.buildGoModule {
          pname = "micasa";
          inherit version;
          src = ./.;
          subPackages = [ "cmd/micasa" ];
          vendorHash = "sha256-5gJRhu+oLLlaK4lSCrAgzJbVHGOidx+BpGZG+psjcDM=";
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
            second=$($sed -n '2p' "$f")

            # Already correct
            if [ "$first" = "$line1" ] && [ "$second" = "$line2" ]; then
              continue
            fi

            # Header present with stale year -- bump it
            if echo "$first" | $grep -q "^$year_pat$" \
               && [ "$second" = "$line2" ]; then
              $sed -i "1s|$year_pat|$line1|" "$f"
              echo "bumped year in $f"
              continue
            fi

            # No header -- insert it
            $sed -i "1i\\$line1\n$line2\n" "$f"
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
            golangci-lint.enable = true;
            actionlint.enable = true;
            statix.enable = true;
            deadnix.enable = true;
            biome.enable = true;
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
          };
        };

        # Fontconfig for VHS recordings using Hack Nerd Font.
        # JetBrains Mono's variable font files cause xterm.js in Chromium to
        # miscalculate cell width, producing visible letter-spacing gaps.
        # Hack Nerd Font renders correctly and includes icon glyphs.
        vhsFontsConf = pkgs.makeFontsConf {
          fontDirectories = [ "${pkgs.nerd-fonts.hack}/share/fonts/truetype" ];
        };

        deadcode = pkgs.buildGoModule {
          pname = "deadcode";
          version = "0.42.0";
          src = pkgs.fetchFromGitHub {
            owner = "golang";
            repo = "tools";
            rev = "v0.42.0";
            hash = "sha256-0RiinnIocPaj8Z5jtYGkbFiRf1BXyap4Z8e/sw2FBgg=";
          };
          subPackages = [ "cmd/deadcode" ];
          vendorHash = "sha256-oYmM+5lNmlP2i78NsG3v4WRhAUbiwS+EFkiicI6MKXA=";
          doCheck = false;
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
            packages = [
              pkgs.go
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
              pkgs.gopls
              pkgs.goreleaser
              pkgs.syft
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
              mkdir -p docs/static/images
              cp images/favicon.svg docs/static/images/favicon.svg
              cp images/demo.webp docs/static/images/demo.webp
              rm -rf website
              hugo --source docs --destination ../website --minify
              pagefind --site website --quiet
            '';
          };
          site = pkgs.writeShellApplication {
            name = "micasa-website";
            runtimeInputs = [
              pkgs.hugo
              pkgs.pagefind
            ];
            text = ''
              mkdir -p docs/static/images
              cp images/favicon.svg docs/static/images/favicon.svg
              cp images/demo.webp docs/static/images/demo.webp

              # Build once to generate the pagefind index, then copy it
              # into docs/static/ so hugo server serves it as a static asset.
              _tmpsite=$(mktemp -d)
              hugo --source docs --destination "$_tmpsite" --minify --quiet
              pagefind --site "$_tmpsite" --quiet
              rm -rf docs/static/pagefind
              cp -r "$_tmpsite/pagefind" docs/static/pagefind
              rm -rf "$_tmpsite"

              _port=$((RANDOM % 10000 + 30000))
              printf 'http://localhost:%s\n' "$_port"
              exec hugo server --source docs --buildDrafts --disableFastRender --noHTTPCache --port "$_port" --bind 0.0.0.0 &>/dev/null
            '';
          };
          # Records any VHS tape and converts the GIF output to WebP
          record-tape = pkgs.writeShellApplication {
            name = "record-tape";
            runtimeInputs = [
              micasa
              pkgs.vhs
              pkgs.nerd-fonts.hack
              pkgs.libwebp
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

              gif_path=$(grep -m1 '^Output ' "$tape" | awk '{print $2}')
              if [[ -z "$gif_path" || "$gif_path" != *.gif ]]; then
                echo "error: tape must contain an Output directive ending in .gif" >&2
                exit 1
              fi

              webp_path="''${gif_path%.gif}.webp"

              mkdir -p "$(dirname "$gif_path")"
              vhs "$tape"
              gif2webp -m 6 "$gif_path" -o "$webp_path"
              rm "$gif_path"
            '';
          };
          record-demo = pkgs.writeShellApplication {
            name = "record-demo";
            runtimeInputs = [ self.packages.${system}.record-tape ];
            text = ''
              record-tape docs/tapes/demo.tape
            '';
          };
          # Captures a single VHS tape to a WebP screenshot: capture-one <tape-file>
          capture-one = pkgs.writeShellApplication {
            name = "capture-one";
            runtimeInputs = [
              micasa
              pkgs.vhs
              pkgs.nerd-fonts.hack
              pkgs.imagemagick
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

              tmpdir=$(mktemp -d)
              trap 'rm -rf "$tmpdir"' EXIT

              vhs "$tape"

              # Extract last frame from GIF as lossless WebP
              magick "$OUT/$name.gif" -coalesce "$tmpdir/frame-%04d.png"
              last=$(printf '%s\n' "$tmpdir/frame"-*.png | sort -t- -k2 -n | tail -1)
              magick "$last" -quality 100 -define webp:lossless=true "$OUT/$name.webp"
              rm -f "$OUT/$name.gif"

              echo "$name -> $OUT/$name.webp"
            '';
          };

          # Captures VHS tapes in parallel: capture-screenshots [name ...]
          capture-screenshots = pkgs.writeShellApplication {
            name = "capture-screenshots";
            runtimeInputs = [
              self.packages.${system}.capture-one
              pkgs.fd
              pkgs.parallel
            ];
            text = ''
              TAPES="docs/tapes"

              if [[ $# -gt 0 ]]; then
                # Named tapes in parallel
                printf '%s\n' "$@" | parallel --bar capture-one "$TAPES/{}.tape"
                exit
              fi

              # All tapes in parallel (skip demo and using-* animated tapes)
              ntapes=$(fd -e tape --exclude demo.tape --exclude 'using-*.tape' . "$TAPES" | wc -l)
              nprocs=$(nproc)
              jobs=$(( ntapes < nprocs ? ntapes : nprocs ))
              fd -e tape --exclude demo.tape --exclude 'using-*.tape' -0 . "$TAPES" \
                | parallel -0 -j"$jobs" --bar capture-one {}
            '';
          };
          # Records all animated demo tapes (using-*) in parallel
          record-animated = pkgs.writeShellApplication {
            name = "record-animated";
            runtimeInputs = [
              self.packages.${system}.record-tape
              pkgs.fd
              pkgs.parallel
            ];
            text = ''
              TAPES="docs/tapes"
              ntapes=$(fd -g 'using-*.tape' . "$TAPES" | wc -l)
              if [[ "$ntapes" -eq 0 ]]; then
                echo "no using-*.tape files found in $TAPES" >&2
                exit 1
              fi
              nprocs=$(nproc)
              jobs=$(( ntapes < nprocs ? ntapes : nprocs ))
              fd -g 'using-*.tape' -0 . "$TAPES" \
                | parallel -0 -j"$jobs" --bar record-tape {}
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
          run-deadcode = pkgs.writeShellApplication {
            name = "run-deadcode";
            runtimeInputs = [
              deadcode
              pkgs.go
            ];
            runtimeEnv.CGO_ENABLED = "0";
            text = ''
              export GOCACHE="''${GOCACHE:-$(mktemp -d)}"
              export GOMODCACHE="''${GOMODCACHE:-$(mktemp -d)}"
              deadcode -test ./...
            '';
          };
          run-osv-scanner = pkgs.writeShellApplication {
            name = "run-osv-scanner";
            runtimeInputs = [ pkgs.osv-scanner ];
            text = ''
              osv-scanner scan --config osv-scanner.toml --no-ignore --recursive .
            '';
          };
          run-pre-commit = pkgs.writeShellApplication {
            name = "run-pre-commit";
            runtimeInputs = [
              pkgs.go
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
              pre-commit run --all-files
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
            record-tape = app (pkg "record-tape") "Record a VHS tape to WebP";
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
            osv-scanner = app (pkg "run-osv-scanner") "Scan for known vulnerabilities";
            pre-commit = app (pkg "run-pre-commit") "Run all pre-commit hooks";
          };

        formatter = pkgs.nixpkgs-fmt;
      }
    );
}
