# 06: Opening repos and syncing with workspaces

**What to build:** From the rail or the repo page:
- `o` focuses the workspace that maps to the selected repo, or creates one at the repo when none does, so there are never duplicates;
- `s` opens a shell split in the repo beside the rail;
- `L` opens lazygit there when it is installed.

Rail lines show the name of each workspace mapped to the repo. Creating, closing or focusing workspaces in Herdr updates those names through the sync. See the spec: Rail and repo page, Herdr adapter.

**Blocked by:** 04

**Status:** ready-for-agent

- [ ] `o` returns `workspace focus <id>` when a workspace maps to the repo (root repo → root workspace), else `workspace create --cwd <repo> --label <name> --focus`
- [ ] `s` returns a `pane split` beside the rail with the repo as cwd
- [ ] `L` returns the split plus `pane run <pane> lazygit` when lazygit is on PATH; otherwise the rail shows a message
- [ ] Rail lines carry `⧉<workspace>` for every mapped workspace, refreshed from the snapshot on each sync
- [ ] Tests on the model's returned commands for each case, and a golden view with workspace names
