<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Create a GitHub issue for the current user request.

Every user request that doesn't already have a GitHub issue gets one. This
includes small one-liner asks and micro UI tweaks. Do this immediately when
the request is made, not later in a batch.

Conventions:

- Repo: `micasa-dev/micasa`
- Title: conventional-commit style (e.g. `feat(ui): add dark mode toggle`,
  `fix(data): quotes not linking to vendor`)
- Body: write to a temp file and use `--body-file` (not `--body`)
- Keep the body concise: what the user asked for, any relevant context

Exceptions:

- Do not create an issue solely for AGENTS.md rule updates
- Do not create duplicate issues -- search first with
  `gh issue list --repo micasa-dev/micasa --search "relevant keywords"`
