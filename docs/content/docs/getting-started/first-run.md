+++
title = "First Run"
weight = 2
description = "What to expect the first time you launch micasa."
linkTitle = "First Run"
+++

<video src="/videos/first-run.webm" class="demo-video" autoplay loop muted playsinline></video>

## Demo mode

The fastest way to explore micasa is demo mode:

```sh
micasa demo
```

This launches with sample data in an in-memory database -- projects,
maintenance items, appliances, service log entries, quotes, and a
pre-filled house profile. Nothing is saved to disk. When you quit, the data
is gone.

To persist demo data for later, pass a path:

```sh
micasa demo /tmp/demo.db
```

## Starting fresh

Just run:

```sh
micasa
```

On first launch, micasa creates its database in your platform's data directory
(e.g. `~/.local/share/micasa/micasa.db` on Linux) and presents you with the **house profile
form**. Fill in your home's details -- nickname is the only required field,
everything else is optional. You can always edit the profile later with <kbd>p</kbd> in
Edit mode.

Once the house profile is saved, you land on the <a href="/docs/guide/dashboard/" class="tab-pill">Dashboard</a>, which shows an
at-a-glance overview of your home (it'll be empty to start). Press <kbd>f</kbd> to
dismiss the dashboard and start adding data.

## First steps

A typical workflow to get started:

1. **Add a project**: press <kbd>f</kbd> to switch to the <a href="/docs/guide/projects/" class="tab-pill">Projects</a> tab, <kbd>i</kbd> to enter
   Edit mode, then <kbd>a</kbd> to add. Fill in a title, pick a type and status.
2. **Add a maintenance item**: <kbd>f</kbd> to the <a href="/docs/guide/maintenance/" class="tab-pill">Maintenance</a> tab, <kbd>a</kbd> to add. Name
   it, set a category, optionally link an appliance, and set an interval.
3. **Add an appliance**: <kbd>f</kbd> to <a href="/docs/guide/appliances/" class="tab-pill">Appliances</a>, <kbd>a</kbd> to add. Name, brand, model
   number, warranty expiry.
4. **Check the dashboard**: press <kbd>D</kbd> to see what needs attention.

Don't worry about entering everything at once. micasa is designed for
incremental data entry -- add things as you think of them, edit later.

## LLM chat

If you have a local LLM server running (like [Ollama](https://ollama.com)),
press <kbd>@</kbd> to open the chat overlay and ask questions about your data in plain
English -- "How much have I spent on plumbing?" or "When is the HVAC filter
due?"

See [LLM Chat]({{< ref "/docs/guide/llm-chat" >}}) for setup details and
[Configuration]({{< ref "/docs/reference/configuration" >}}) for backend options.
