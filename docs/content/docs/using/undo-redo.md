+++
title = "Undo and Redo"
weight = 4
description = "Multi-level undo and redo for data edits."
linkTitle = "Undo & Redo"
+++

micasa supports multi-level undo and redo for data edits.

<video src="/videos/using-undo-redo.webm" class="demo-video" autoplay loop muted playsinline></video>

## How it works

Every time you save a form (add, edit, or inline edit), micasa snapshots the
previous state of the entity before applying your changes. These snapshots are
stored in a LIFO stack (up to 50 entries).

## Undo

In Edit mode, press <kbd>u</kbd> to undo the last edit. This restores the entity to its
state before the edit was made. You can undo multiple times to walk back
through your edit history.

The undo operation:

1. Pops the most recent snapshot from the undo stack
2. Snapshots the *current* state and pushes it onto the redo stack
3. Restores the entity to the saved state
4. Refreshes all tabs and the dashboard

## Redo

In Edit mode, press <kbd>r</kbd> to redo an undone edit. This re-applies the change
that was undone.

Redo works symmetrically to undo: it pops from the redo stack, snapshots the
current state onto the undo stack, and restores.

## Important notes

- **New edits clear the redo stack.** If you undo a change and then make a
  new edit, the redo history is lost. This is standard undo/redo behavior.
- **Undo works across tabs.** The undo stack is global, not per-tab. If you
  edit a project, then edit a maintenance item, pressing <kbd>u</kbd> twice will undo
  both, in reverse order.
- **Session-only.** Undo history is not persisted to disk. Quitting micasa
  clears the stacks.
- **Stack size.** The stack holds up to 50 entries. Older entries are silently
  dropped.
