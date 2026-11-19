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
