---
description: "Use when TUI output renders wrong in VHS or live (stray ANSI codes, garbled or missing text) and reading the code doesn't show why."
---
<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Debug rendering issues (especially in VHS recordings) by dumping raw output
to `/tmp` for inspection.

## When to use

When rendered output looks wrong in VHS or live -- stray ANSI codes, garbled
text, missing content -- and reading the code alone doesn't reveal the cause.

## Procedure

1. **Add temporary dump calls** at the rendering site. Write the raw string
   to a file in `/tmp` so it can be inspected outside the TUI:

   ```go
   os.WriteFile("/tmp/micasa-debug-raw.txt", []byte(rawContent), 0o644)
   ```

   Place dumps at each transformation stage (pre-render, post-render,
   final composite) to isolate where corruption enters.

2. **Reproduce** the issue -- run VHS or interact with the live app.

3. **Inspect** the dump files with `cat -v` (makes control characters
   visible) or `xxd` for hex:

   ```sh
   cat -v /tmp/micasa-debug-raw.txt
   xxd /tmp/micasa-debug-raw.txt | head -40
   ```

4. **Identify** the offending bytes/sequences and trace them back to the
   code path that produced them.

5. **Remove** all `os.WriteFile` debug calls before committing. These are
   strictly temporary instrumentation.

## Tips

- Name dump files descriptively: `/tmp/micasa-chat-viewport.txt`,
  `/tmp/micasa-overlay-composite.txt`, etc.
- For streaming content, append a counter: `/tmp/micasa-chunk-001.txt`.
- If the issue only appears in VHS, the cause is often a terminal query
  (like OSC 11 for background color) whose response leaks into stdin.
  `cat -v` will show the response as literal escape sequences.
