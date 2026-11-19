# 03: Herdr plugin manifest and `githerd herdr sync`

**What to build:** githerd installs as a Herdr plugin and keeps sidebar tokens current. `githerd herdr sync` reads the roots file and maps workspaces through `herdr api snapshot`. It refreshes status, fetching with the 15-minute TTL, and reports `$repos`, `$changes` and `$stale` to the right workspaces. The startup hook and the workspace and agent event hooks run it. See the spec: Herdr adapter, Plugin manifest.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] `herdr-plugin.toml` at the repo root: id `githerd`, min Herdr 0.9.5, platforms linux and macos, `[[build]]` with `go build`, `[[startup]]`, and `[[events]]` for `workspace.created`, `workspace.closed`, `workspace.focused` and `pane.agent_status_changed`, all running the sync
- [ ] The roots file in the plugin config dir: one path per line, `~` expanded, blanks and `#` comments ignored; no roots means nothing is reported
- [ ] Workspace mapping uses the cwd of the first pane listed for each workspace's first tab in the snapshot: root workspace, mapped workspace (deepest repo), or outside
- [ ] Token values match the table in the spec; tokens that don't apply are cleared with `--clear-token`; an outside workspace has its earlier tokens cleared
- [ ] One report per workspace with all its `--token`/`--clear-token` flags, `--source githerd`, no `--ttl-ms`; `--seq` = max(Unix ms, previous + 1), the previous value kept in the state dir
- [ ] For `pane.agent_status_changed`, sync acts only when the event's agent status is done or idle
- [ ] A lock in the plugin state dir: a sync that finds the lock held leaves a pending mark and exits; the running sync reruns once if it finds the mark
- [ ] Black-box tests with a fake `herdr` behind `$HERDR_BIN_PATH` (canned snapshot, recorded argv) cover mapping, values, clearing, event filtering and the lock
