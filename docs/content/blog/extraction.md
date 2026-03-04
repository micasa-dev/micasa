+++
title = "Your receipts are not going to enter themselves"
date = 2026-02-26
description = "How micasa reads your invoices with OCR and a local LLM, and why the LLM is the least important part."
+++

Last week I was staring at a plumbing invoice -- $287 for a garbage disposal
install, which is apparently what it costs to have a man lie under your sink
for forty minutes -- and I realized I was about to do the thing I built this
app to avoid: transcribing data by hand.

Open the vendor form, type "Ace Plumbing." Open the quote form, type "$287."
Open the maintenance form, type "garbage disposal." Three forms for one piece
of paper. The information is *right there*, printed in ink, and I'm
re-entering it like a medieval scribe copying a manuscript about drains.

So I tried to teach micasa to read.

## The pipeline

The [extraction pipeline](/docs/guide/documents/#extraction-pipeline)
([#475](https://github.com/cpcloud/micasa/pull/475)) runs when you save
a document with a file attached. It has three layers and each one is
independent. If a layer's tools aren't installed, it gets skipped and the
others still run.

**Text extraction.** `pdftotext` pulls selectable text out of PDFs.

**OCR.** If there's no selectable text (scanned PDF, photo of a receipt you
took at a bad angle in a parking lot), Tesseract does OCR on each page. micasa
tries three different PDF-to-image tools in order (`pdfimages`, then
`pdftohtml`, then `pdftoppm`) because no single one handles every PDF
correctly. The overlay shows which tool it picked and per-page progress.

**LLM.** A local model reads whatever text the first two layers produced and
proposes database operations: create a vendor called Ace Plumbing, create
a quote for $287 linked to that vendor, update this document's title to
"Garbage Disposal Invoice." The results appear in a tabbed preview using the
same table layout as the rest of the app. You review, you accept or reject.
The LLM never writes to your database directly.

<video src="/videos/extraction.webm" autoplay loop muted playsinline style="max-width:100%;border-radius:8px"></video>

The overlay is keyboard-driven: `j`/`k` to navigate steps, `enter` to
expand logs, `x` to explore the proposed operations table, `a` to accept,
`r` to rerun the LLM if the first attempt missed, `esc` to cancel.

## The LLM is the least important part

I know I just spent a paragraph on it. But the pipeline is designed so that
text extraction and OCR are the load-bearing layers. They're deterministic:
they either work or they don't, and when they work the text is *correct*. The
LLM reads that text and tries to propose structured fields (vendor name,
amount, title) so you don't have to type them yourself.

It's also a 7-billion-parameter model running on your laptop that will
propose that your plumber's name is "Invoice" and the amount is
"Date: 2026-01-15." That's why you review before accepting. If the model
isn't installed, the pipeline still extracts text and shows it to you -- you
fill in the fields yourself.

Same applies to the [LLM chat](/docs/guide/llm-chat/). Useful for quick
questions about your data, but sorting the table is usually faster and
doesn't hallucinate dollar amounts.

## What it's good at

One-page invoices with a company name, phone number, line items, and a total
at the bottom. The pipeline usually gets the vendor, the amount, and the
document title right. Scanned receipts work too, within reason.

Multi-page insurance documents with nested tables and legalese? The model
tries. You'll be correcting more than you're saving. Feed it a 30-page HOA
covenant and it'll pull out someone's phone number and call it a quote.

All the extraction tools are standard unix: `poppler-utils` for PDF handling,
`tesseract` for OCR, Ollama for the LLM. One `apt install` or `brew install`
away. Or `nix develop` in the repo.

## Other things since launch

Some other things that landed since the [launch post](/blog/launch/):

- **[Relative dates](/docs/using/date-input/#natural-language)** -- type `yesterday`,
  `2 weeks ago`, or `last friday` instead of `YYYY-MM-DD`. Works in any date
  field.
  ([#528](https://github.com/cpcloud/micasa/pull/528))
- **[`t` for today](/docs/using/date-input/#calendar-picker)** -- in date
  pickers. Typing `2026-02-26` every time was a small daily indignity.
  ([#517](https://github.com/cpcloud/micasa/pull/517))
- **[Maintenance](/docs/guide/maintenance/) due dates** -- items can now have
  a fixed date instead of only a recurring interval. "Clean the gutters every
  6 months" and "replace the roof by 2028" are different kinds of tasks.
  ([#525](https://github.com/cpcloud/micasa/pull/525))
- **`E` for [full edit](/docs/reference/keybindings/#data-operations)** -- opens the edit form
  from any column. Previously you had to navigate to the right column first.
  ([#540](https://github.com/cpcloud/micasa/pull/540))
- **Scrollable [dashboard](/docs/guide/dashboard/)** -- previously clipped
  sections that didn't fit. It scrolls correctly now.
  ([#531](https://github.com/cpcloud/micasa/pull/531))

## Try it

```sh
go run github.com/cpcloud/micasa/cmd/micasa@latest --demo
```

Switch to the Docs tab, press `i` to enter Edit mode, then `A` to drop
a PDF in and see what the pipeline makes of it. Binaries on the
[releases page](https://github.com/cpcloud/micasa/releases/latest).

If you find a bug or have an opinion about how extraction should work,
[open an issue](https://github.com/cpcloud/micasa/issues). If you find it
useful, a [star](https://github.com/cpcloud/micasa) helps other people
find the project.
