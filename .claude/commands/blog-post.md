<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Create or edit a blog post in `docs/content/blog/`.

The user's topic or direction: $ARGUMENTS

Draft the full post unless they ask for an outline only.

## Conventions

- **File**: `docs/content/blog/<slug>.md` -- kebab-case, short, descriptive.
- **Front matter**: TOML (`+++` delimiters), three fields only:

```toml
+++
title = "Title here"
date = YYYY-MM-DD
description = "One sentence, shown in the blog index."
+++
```

- `date` controls visibility. Hugo excludes posts with a future `date` from
  builds. The site rebuilds every Thursday at ~9 AM ET.
- **Default to the next Thursday** for `date` unless the user explicitly asks
  to publish immediately (in which case use today's date).
- No `draft`, `publishDate`, or other front matter fields -- keep it minimal.

## Voice and style

Read **every** existing post in `docs/content/blog/` before writing. Study the
actual sentences, not just the structure. Match the voice precisely:
conversational, technically honest, opinionated, dry humor. First person
singular. Short paragraphs.

**Do not write AI slop.** No puns in headings. No quippy one-liners. No
"buckle up", "let's dive in", "spoiler alert", "here's the kicker", or any
other filler phrases that scream "a language model wrote this." No breathless
enthusiasm. No manufactured excitement. Write like a tired engineer who finds
the work genuinely interesting but would never use an exclamation mark to
prove it. If a sentence sounds like a LinkedIn post, delete it.

- Lead with a concrete anecdote or problem, not a feature announcement.
- Link to PRs/issues with `[#NNN](https://github.com/cpcloud/micasa/pull/NNN)`.
- Use `<kbd>key</kbd>` for keyboard shortcuts, backticks for code/config.
- End with a "Try it" section showing `go run` and releases link.
- Keep it under ~150 lines. These are weekly dev logs, not essays.

## Scheduled publishing

To publish immediately: set `date` to today or a past date and push to `main`.

To schedule for a future Thursday: set `date` to that Thursday. Push to `main`
now -- Hugo skips it. The weekly scheduled build picks it up on that Thursday.

## Checklist

1. Read existing posts for voice calibration.
2. Write the post.
3. Verify the slug doesn't collide with existing posts.
4. If scheduling, confirm the `date` is a Thursday.
