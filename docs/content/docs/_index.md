+++
title = "micasa docs"
description = "Documentation for micasa, a terminal UI for tracking everything about your home."
+++

Your house is quietly plotting to break while you sleep -- and you're dreaming
about redoing the kitchen. micasa tracks both from your terminal.

micasa is a keyboard-driven terminal UI for managing everything about your home:
maintenance schedules, projects, incidents, vendor quotes, appliances,
warranties, service history, and file attachments. It stores all data in a
single SQLite file on your machine.
No cloud. No account. No subscriptions.

## What it does

- **[Maintenance tracking]({{< ref "/docs/guide/maintenance" >}})** with auto-computed due dates, service log history,
  and vendor records
- **[Project management]({{< ref "/docs/guide/projects" >}})** from ideating through completion (or graceful
  abandonment), with budget tracking
- **[Quote comparison]({{< ref "/docs/guide/quotes" >}})** across vendors, with cost breakdowns
- **[Incident logging]({{< ref "/docs/guide/incidents" >}})** with severity, location, and optional links to
  appliances and vendors
- **[Appliance inventory]({{< ref "/docs/guide/appliances" >}})** with warranty windows, purchase dates, and
  maintenance history tied to each one
- **[Dashboard]({{< ref "/docs/guide/dashboard" >}})** showing open incidents, overdue maintenance, active projects,
  and expiring warranties at a glance
- **[Vim-style modal navigation]({{< ref "/docs/using/navigation" >}})** with Nav and Edit modes, multi-column
  sorting, column hiding, and cross-tab FK links
- **[Document attachments]({{< ref "/docs/guide/documents" >}})** -- attach files (manuals, invoices,
  photos) to any record, stored as BLOBs in the same SQLite file. When an LLM
  is configured, an [extraction pipeline]({{< ref "/docs/guide/documents#extraction-pipeline" >}})
  (OCR, text extraction, LLM) pre-fills form fields with structured data
- **[LLM chat]({{< ref "/docs/guide/llm-chat" >}})** -- ask questions about your home data,
  powered by a local LLM ([Ollama](https://ollama.com) or any OpenAI-compatible API)

## What it doesn't do

micasa is not a smart home controller, a home automation platform, or a
property management SaaS. It's a personal tool for one house (yours), designed
to answer questions like "when did I last change the furnace filter?" and "is
the dishwasher still under warranty?"

## Quick start

```sh
go install github.com/cpcloud/micasa/cmd/micasa@latest
micasa demo   # poke around with sample data
micasa          # start fresh with your own house
```

![micasa dashboard](/images/dashboard.webp)

Read the full [Installation]({{< ref "/docs/getting-started/installation" >}}) guide for
other options (binaries, Nix, container).
