<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Evaluate numtide/go2nix for the Nix Go builds

## Question

Should micasa's Nix Go builds move from nixpkgs' `buildGoModule` to
[numtide/go2nix](https://github.com/numtide/go2nix)? go2nix compiles each Go
package in its own derivation, so a one-file edit rebuilds only that package,
its dependents, and the link. Modules are pinned per module from a
`go2nix.toml` lockfile, or from `go.sum` at evaluation time, in place of a
single `vendorHash`.

Three builds are candidates:

- `nix/package.nix`: micasa itself (`buildGo127Module`, `vendorHash`,
  `CGO_ENABLED=0`, `subPackages = [ "cmd/micasa" ]`, `-X main.version`,
  tests of `cmd/micasa`).
- `nix/overlay.nix`: `deadcode` (golang/tools v0.52.0) and `nilaway`
  (pinned commit), each with its own `vendorHash` and `doCheck = false`.

A build moves only if go2nix gives a concrete benefit, keeps the binary and
its checks equivalent, builds with Go 1.27.3 or newer, and works on every
system the flake supports with the Nix that CI and contributors run.

## Decision

Keep all three builds on `buildGoModule`. Do not add go2nix as a flake input.
Nothing in `vendorHash`, the `vendor-hash-check` hook, the
`update-vendor-hash` workflow, or `/update-vendor-hash` changes.

go2nix does make rebuilds faster (see [Measurements](#measurements)), but
adopting it for the package the flake exports would break evaluation for
everyone who builds micasa without a custom Nix setup, it cannot run the
same tests, and its zero-maintenance lockfile-free mode has a cache
poisoning bug. The reasons, in order of weight:

1. **Neither mode works with stock Nix.** The default builder calls
   `builtins.resolveGoPackages`, a primop from a Nix plugin that has to be
   loaded with `plugin-files` and built against the exact Nix release that
   loads it (it links Nix 2.34's unstable C++ API). Without it, evaluation
   fails with `attribute 'resolveGoPackages' missing`. The experimental
   builder needs no plugin but needs Nix >= 2.34 with `recursive-nix`,
   `ca-derivations` and `dynamic-derivations`, a builder offering the
   `recursive-nix` system feature, and it runs no tests and ignores
   `version`. `packages.micasa` is what CI, `nix profile install
   github:micasa-dev/micasa`, the NixOS module, and the capture and record
   apps all build, so either requirement would land on all of them.
2. **The tests cannot match.** `buildGoModule` with `subPackages` runs
   `go test ./cmd/micasa` only. go2nix's `doCheck` runs the tests of every
   local package `cmd/micasa` reaches (22 packages here) and has no option to
   narrow that. Under its test runner `internal/address`'s live API test
   fails, and with `-test.short` `internal/app` still fails
   (`TestGlamourStyleIsNoTTYInTests`, `TestCancellationDuringAnswerStreaming`,
   `TestRenderChatMessagesShowsAllRoles`). The two choices are a different,
   failing test set or no tests.
3. **Lockfile-free mode is unsafe; lockfile mode keeps the maintenance cost.**
   Without `goLock`, the plugin hashes module trees in `GOMODCACHE` and
   caches the result under `$XDG_CACHE_HOME/go2nix/nar/`, keyed only by the
   `go.sum` `h1:` string, without checking the tree against that `h1:`
   (`packages/go2nix-nix-plugin/rust/src/module_hashes.rs` and
   `nar_cache.rs` at f736989). The module fetch runs with `GOSUMDB=off`
   (`nix/dag/fetch-go-module.nix`). Evaluating a hostile repository, a
   malicious pull request included, can plant a cache entry that a later
   micasa build trusts. With a committed `go2nix.toml` the cache is not
   used, but then every `go.mod` change needs `go2nix generate`, which is
   the same chore `vendorHash` is today, with a 128-entry file in place of
   one hash.
4. **x86_64-darwin is unsupported.** go2nix supports x86_64-linux,
   aarch64-linux and aarch64-darwin. `flake-utils.lib.eachDefaultSystem`
   also exports x86_64-darwin. CI runs Nix only on x86_64-linux and the
   release workflow builds with goreleaser, so nothing in the repository
   depends on x86_64-darwin, but external users on Intel Macs would lose
   `packages.micasa`. With `buildGoModule` kept, the system stays.
5. **The benefit lands where Nix rebuilds are rare.** Day-to-day iteration
   uses `go build` and `go test` with Go's own cache. CI builds
   `.#micasa` from a fresh store, where per-package reuse only helps if a
   binary cache is shared between runs, and micasa's CI has none. The
   capture and record apps rebuild micasa after every source edit, and that
   is the one place a contributor would notice the speed-up.

`deadcode` and `nilaway` stay on `buildGoModule` for a simpler reason: they
are third-party tools pinned to one upstream revision, built once and then
served from the store. Per-package rebuilds only pay off when the source
changes, and theirs changes only on a version bump, when every package
changes anyway.

## Measurements

Measured on aarch64-darwin, Nix 2.34.9, with go2nix f736989 in lockfile mode
(`goLock`, 128 modules), `CGO_ENABLED=0`, Go 1.27.3, the plugin loaded, and
remote builders disabled (`--builders ''`). Each edit appends one constant to
one file and builds `micasa`.

| Edit | `buildGoModule` (tests on) | `buildGoModule` (tests off) | go2nix (tests off) |
| ---- | -------------------------- | --------------------------- | ------------------ |
| `internal/app/model.go` | 68 s | 30 s | 7 s (3 derivations) |
| `internal/locale/currency.go` | 68 s | 30 s | 9 s (12 derivations) |

Of the 68 s, about 35 s is `go test ./cmd/micasa`. go2nix with tests on did
not get a time because its test set fails (see above). A cold go2nix build
(standard library, 128 module fetches, every package) took 279 s.
Evaluating `.#micasa.drvPath` with `--no-eval-cache` took about 0.6 s with
`buildGoModule` and about 0.85 s with go2nix, which runs `go list` during
every evaluation.

The go2nix binary matched on `micasa --version`, Go version, `-trimpath`,
`CGO_ENABLED=0` and target. `go version -m` differed in two ways: go2nix
records each dependency's `h1:` sum, which `buildGoModule`'s vendor mode
leaves out, and it reports `-buildmode=pie` where `buildGoModule` reports
`-buildmode=exe`. Both binaries are PIE Mach-O files.

## What adopting it would take

If the trade-offs above change, an opt-in output such as
`packages.micasa-go2nix`, with `packages.micasa` left on `buildGoModule`,
avoids breaking consumers. It would need:

- `go2nix` as a flake input with `inputs.nixpkgs.follows = "nixpkgs"`, a
  committed `go2nix.toml`, regenerated from the repository root on every
  `go.mod` change, and a pre-commit hook and Renovate workflow for it next to
  the `vendorHash` ones.
- `doCheck = false`, since the test sets differ, so the output would not be a
  substitute for `packages.micasa` in CI.
- The plugin loaded per command (`--option plugin-files
  <plugin>/lib/nix/plugins/libgo2nix_plugin.so`), never globally in
  `nix.conf`: a globally loaded plugin lets any evaluated expression run
  `go list` with network access and write to the go2nix hash cache.
- The output gated so `nix flake show` and other evaluations without the
  plugin still succeed.

For CI to build it, the Nix that `cachix/install-nix-action` installs would
have to be the same 2.34 release the plugin is built against, the plugin
would have to be built first (Rust and C++ against Nix's libraries, so the
Harden Runner allowlists would need crates.io or a binary cache that holds
the plugin),
and the build step would pass `--option plugin-files`. Evaluation also
reaches `proxy.golang.org`, which the Nix jobs already allow.

## Revisit when

- go2nix can build without a plugin or experimental features, or Nix ships
  an equivalent primop;
- go2nix can restrict `doCheck` to chosen packages, or micasa's whole test
  suite passes under its runner;
- the plugin's NAR cache verifies trees against `go.sum` and keys entries by
  `path@version` as well as `h1:`;
- micasa gains a shared Nix binary cache, which would make per-package reuse
  pay off in CI.
