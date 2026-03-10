<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Do NOT run pre-commit hooks manually. The git hooks run automatically on
`git commit` (pre-commit stage) and `git push` (pre-push stage).

Before committing, run these checks:

1. `nix run '.#pre-commit' -- --from-ref origin/main --to-ref HEAD --verbose` -- run all pre-commit hooks against changed files
2. `go build ./...` -- verify it compiles
3. `go test -shuffle=on ./...` -- all packages, shuffled, no `-v`

If the commit hook fails, fix the issue and commit again. Never use
`--no-verify`.

If pre-commit reformats files, re-stage them and commit again.

If pre-commit fails in a worktree with environment or cache errors, recover
with: `direnv allow`, then `git clean -fdx`, then `direnv reload`, then
retry.
