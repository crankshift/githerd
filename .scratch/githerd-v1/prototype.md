# Prototype: githerd UI in Herdr

Status: resolved

Primary source: branch `prototype/herdr-ui`, files `prototypes/herdr-ui/index.html` (round 1) and `prototypes/herdr-ui/navigator.html` (round 2).

## Round 1: what is the main surface?

Question: popup table (A), triage board (B), picker with preview (C), or a docked rail that stays open (D)?

Verdict: **D**. A pane that stays open beside the work, refreshes itself and can drill into repos beats a modal that must be reopened. Wanted on top: browse git visually (diffs, branches), and stay in sync with Herdr workspaces.

## Round 2: how does the rail open a repo?

Question: Miller columns (D1), fixed list with tabbed detail (D2), or a narrow rail plus a zoomed repo page (D3)?

Verdict: **D3**. The rail stays narrow; Enter zooms the githerd pane and shows the repo page with a wide diff, without manual zooming.

Also settled with the prototype:

- `o` focuses the workspace mapped to the repo, and creates one only when none exists.
- Mapped workspaces get an uncommitted token beside Herdr's built-in `branch` and `git_status`; the root workspace gets the root's summary.
- A `cd` inside a pane fires no hookable event, so the mapping can lag until the next refresh (ADR-0003).
