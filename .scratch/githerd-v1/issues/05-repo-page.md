# 05: The repo page

**What to build:** Enter on a rail line opens that repo's repo page and asks Herdr to zoom the pane. The left side lists Changes (staged and unstaged told apart), To push (Local commits when there is no upstream), To pull, Branches and Log. The right side shows a live diff of the selected line. `J`/`K` scroll the diff. Esc returns to the rail and unzooms. See the spec: Rail and repo page. Design reference: branch `prototype/herdr-ui`, `navigator.html` (D3, page mode).

**Blocked by:** 04

**Status:** resolved

- [x] Enter returns a zoom-on command for the rail's own pane (`--pane`, never the focused pane); Esc returns zoom-off and goes back to the rail with the cursor kept
- [x] Changes: staged and unstaged diffs; untracked files shown in full as added
- [x] To push and To pull list commits; the selected commit shows its full diff
- [x] Branches: local branches with upstream ahead/behind or "no upstream", plus remote-only branches; the selected branch shows a summary and its recent commits
- [x] Log: recent commits of HEAD
- [x] `z` toggles zoom of the rail's own pane from the rail or the repo page (`pane zoom --pane <rail> --toggle`)
- [x] Diff lines show old and new line numbers when the width allows
- [x] Golden tests of `View` for a busy repo and a clean one; tests that Enter and Esc return the zoom commands

## Comments

Notes from the implementation:

- `internal/repopage` reads the page with git, read-only (`--no-optional-locks`, tested by hashing every file of the repo before and after): `Read` lists the sections, `Show` reads what shows beside one line. The TUI asks for that detail with `LoadDetail` when the cursor first lands on a line, so a busy repo costs one git call per line looked at, not per line listed.
- Commands and answers: `tui.Zoom{Pane, Mode}` (`on`/`off`/`toggle`), `tui.LoadPage{Dir}` → `tui.PageLoaded`, `tui.LoadDetail{Dir, Item}` → `tui.DetailLoaded`. A failed zoom answers `tui.Failed`, shown in 06's note line. The terminal layer runs every Herdr request (zoom, focus, create, split) one after another, so Enter then Esc in quick succession never ends zoomed.
- Log lists the recent commits of HEAD that are not already under To push, as the prototype does, so no commit is listed twice.
- While the page is open, each rail refresh reads it again; the cursor stays on its line, and the old detail shows until it is read again.
- Limits: To push and To pull list at most 100 commits (the heading counts all of them, with "… N more"), Log 10, a branch's summary 10. A diff is cut after 5000 lines and each line after 2000 bytes.
- A nested repo, which git lists as an untracked dir, shows as "a repo of its own". Unmerged files are listed first, with the combined diff.
- Line numbers show when the diff column is at least 48 wide.
- `o`, `s` and `L` work on the page too, on the page's repo, through 06's `act`. They don't unzoom first; whether they should is still open.
- The rail's keys line starts with `enter page`, as in the prototype's D3 rail.
