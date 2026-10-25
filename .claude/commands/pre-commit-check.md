---
description: "Use before committing to confirm the code builds and the tests pass, or when a git hook fails."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

The git hooks run on their own: `git commit` runs the pre-commit stage and
`git push` runs the pre-push stage (`go-generate-check`). Don't run them by
hand before committing.

Before committing, run:

1. `go build ./...` -- verify it compiles
2. `go test -shuffle=on ./...` -- all packages, shuffled, no `-v`

To reproduce CI's pre-commit job over the whole branch, for example after a
rebase:

```
nix run '.#pre-commit' -- --from-ref origin/main --to-ref HEAD
```

If a hook fails, fix the cause and commit again. If a hook reformats files,
stage them and commit again. Never use `--no-verify`.

If the hooks fail in a worktree with environment or cache errors, delete the
generated files and let direnv rebuild them:

1. `rm -r .direnv .pre-commit-config.yaml`
2. `direnv allow`
3. `direnv reload`
