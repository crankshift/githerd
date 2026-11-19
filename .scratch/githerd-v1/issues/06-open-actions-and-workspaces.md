# 06: Opening repos and syncing with workspaces

**What to build:** From the rail or the repo page:
- `o` focuses the workspace that maps to the selected repo, or creates one at the repo when none does, so there are never duplicates;
- `s` opens a shell split in the repo beside the rail;
- `L` opens lazygit there when it is installed.

Rail lines show the name of each workspace mapped to the repo. Creating, closing or focusing workspaces in Herdr updates those names through the sync. See the spec: Rail and repo page, Herdr adapter.

**Blocked by:** 04

**Status:** resolved

- [x] `o` returns `workspace focus <id>` when a workspace maps to the repo (root repo → root workspace), else `workspace create --cwd <repo> --label <name> --focus`
- [x] `s` returns a `pane split --pane <rail>` with the repo as cwd
- [x] `L` returns the split plus `pane run <pane> lazygit` when lazygit is on PATH; otherwise the rail shows a message
- [x] Rail lines carry `⧉<workspace>` for every mapped workspace, refreshed from the snapshot on each sync
- [x] Tests on the model's returned commands for each case, and a golden view with workspace names

## Comments

Notes from the implementation:

- Commands: `tui.FocusWorkspace{ID}`, `tui.CreateWorkspace{Dir, Label}` and `tui.Split{Pane, Dir, Run}`. The terminal layer runs them through `herdr.FocusWorkspace`, `herdr.CreateWorkspace` and `herdr.OpenShell`. Answers come back as `tui.Created{Dir, ID, Err}` and `tui.Failed{Err}`. The exact argv are checked through the binary with the fake `herdr`.
- The split is `pane split --pane <rail> --direction down --cwd <repo> --focus`. Herdr splits only `right` or `down`, and the new pane is the second child. Splitting the narrow rail to the right would halve it, so the shell opens below the rail.
- The label is the repo dir's base name, as in the prototype. Paths sent to Herdr have symlinks resolved, as the rail's repos do.
- Which workspace `o` focuses: the first in sidebar order whose first pane maps to the repo. On the root's own repo, that is a root workspace of that root.
- No duplicates:
  - Until a sync has read the workspaces, `o` opens nothing and says so.
  - A workspace the rail creates counts from the moment it asks for it. While Herdr creates it, `o` waits. Then `o` focuses it by the id Herdr returned.
  - It stays known until a sync that started after Herdr answered; that sync's snapshot is the truth again.
  - A refresh that could not read the workspaces (another sync held the lock, or the snapshot failed) keeps the ones the rail had.
- `⧉` names show only mapped workspaces, as the prototype's `wsBadge` does. A root workspace gets no badge, though `o` on the root's line focuses it. The names are left out where they don't fit beside the repo's own name. A clean repo keeps its `✓` before the names.
- lazygit is looked up once, when the rail starts, on the rail's PATH, which is the Herdr server's. The black-box tests set PATH to a dir with a git wrapper, with or without a fake `lazygit`.
- Messages (lazygit missing, a request Herdr refused) show on the line above the keys until the next key, in place of the problem line.
- The rail's keys line now reads `/ filter  o ws  s shell  f fetch  q quit`, as in the prototype's rail. The existing goldens changed only on that line.
- For the repo page (05): `o`, `s` and `L` act on `Model.selectedRepo()` in `internal/tui/open.go`, through `act(m, k)`. Page mode only has to make `selectedRepo` return the page's repo and send those keys to `act`. Not decided here: whether `s`/`L` from a zoomed repo page should unzoom first.
