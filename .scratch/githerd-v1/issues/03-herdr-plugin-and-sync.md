# 03: Herdr plugin manifest and `githerd herdr sync`

**What to build:** githerd installs as a Herdr plugin and keeps sidebar tokens current. `githerd herdr sync` reads the roots file and maps workspaces through `herdr api snapshot`. It refreshes status, fetching with the 15-minute TTL, and reports `$repos`, `$changes` and `$stale` to the right workspaces. The startup hook and the workspace and agent event hooks run it. See the spec: Herdr adapter, Plugin manifest.

**Blocked by:** 02

**Status:** resolved

- [x] `herdr-plugin.toml` at the repo root: id `githerd`, min Herdr 0.9.5, platforms linux and macos, `[[build]]` with `go build`, `[[startup]]`, and `[[events]]` for `workspace.created`, `workspace.closed`, `workspace.focused` and `pane.agent_status_changed`, all running the sync
- [x] The roots file in the plugin config dir: one path per line, `~` expanded, blanks and `#` comments ignored; no roots means nothing is reported
- [x] Workspace mapping uses the cwd of the first pane listed for each workspace's first tab in the snapshot: root workspace, mapped workspace (deepest repo), or outside
- [x] Token values match the table in the spec; tokens that don't apply are cleared with `--clear-token`; an outside workspace has its earlier tokens cleared
- [x] One report per workspace with all its `--token`/`--clear-token` flags, `--source githerd`, no `--ttl-ms`; `--seq` = max(Unix ms, previous + 1), the previous value kept in the state dir
- [x] For `pane.agent_status_changed`, sync acts only when the event's agent status is done or idle
- [x] A lock in the plugin state dir: a sync that finds the lock held leaves a pending mark and exits; the running sync reruns once if it finds the mark
- [x] Black-box tests with a fake `herdr` behind `$HERDR_BIN_PATH` (canned snapshot, recorded argv) cover mapping, values, clearing, event filtering and the lock

## Comments

Decisions made while building, where the spec left room:

- **The module.** `herdr.Sync(ctx, env)` is the one entry point, with `env` from `herdr.EnvFrom(os.Getenv)`. The rail can call it as it is. It returns `Result{Ran, Roots, Workspaces, Errs}`:
  - each root carries its repos with their state and `Stale` error;
  - each workspace carries the root it lies in and the repo it maps to, `Repo` empty for a root workspace;
  - `Ran` is false when the event called for no sync, or when another sync held the lock and runs in its place.
  The roots file, the snapshot, the mapping, the tokens, the lock and `--seq` all stay inside the package. `status.ReadAll` moved out of the status command so both commands share it.
- **The `changes` token.** `●N` when uncommitted, `∅` when there is no upstream, `●N ∅` when both, cleared when neither. So a clean repo with no upstream shows `∅` alone. The spec's table doesn't cover that case; the prototype does it this way.
- **Every report names all three tokens.** It sets the ones that apply and clears the rest. A root workspace clears `changes` and a mapped workspace clears `repos`, so a workspace whose pane moved from one to the other never keeps the old token.
- **Outside workspaces.** One gets a report only when the snapshot shows it carrying a `repos`, `changes` or `stale` token, and that report clears all three. The snapshot is the record of what githerd reported earlier, so there is no state file, and nothing is cleared after a Herdr restart because Herdr restores no tokens. With no roots every workspace is outside. A fresh session then gets no report at all, and a roots file emptied after a sync clears what it left. The snapshot is still read when there are no roots.
- **`--seq`.** It is one value per sync, shared by all of that sync's reports, because Herdr checks freshness per workspace and source. The last value is kept in `$HERDR_PLUGIN_STATE_DIR/seq`. A missing or unreadable value counts as 0.
- **The lock.**
  - `sync.lock` in the state dir is held with `flock`, so the OS releases it when a sync crashes.
  - A sync that finds it held writes `sync.pending`. It then tries the lock once more, in case the holder had already looked for the mark and let go.
  - The holder looks for the mark after it releases the lock. If it finds one, it takes the lock again and reruns.
  - A sync that fails, for example because Herdr is down, does not rerun. The next sync clears the mark.
- **Order.** Sync reads the roots, discovers repos, fetches with the 15-minute TTL, reads status, then takes the snapshot. The snapshot comes last because fetches can take seconds, so the workspace list is as fresh as possible when reports go out. Several roots are fetched and read together. A repo shared by nested roots is fetched once, and a workspace belongs to the deepest root that contains it.
- **Paths.** Roots and pane cwds are compared with symlinks resolved, because a cwd may come from OSC 7 as the shell spelled it or from the OS resolved. Only `cwd` counts. `foreground_cwd`, the focused pane and later tabs are ignored. A pane with no cwd yet counts as outside.
- **Errors.**
  - With `HERDR_BIN_PATH`, `HERDR_PLUGIN_CONFIG_DIR` or `HERDR_PLUGIN_STATE_DIR` unset, sync exits 2 and calls nothing.
  - A failed snapshot exits 1 with Herdr's error message.
  - Each of these is named on stderr and exits 1, but the rest of the sync still runs: a relative root, which means nothing since hooks run in the plugin root; an unreadable repo, which is left out of the summary; a report Herdr refuses, such as one for a workspace closed since the snapshot.
  - Each herdr call times out after 10s.
- **Event filter.** Only `pane_agent_status_changed` with `agent_status` other than `done` or `idle` is skipped. A missing or unparseable `HERDR_PLUGIN_EVENT_JSON` syncs, because a spare sync costs less than a missed one.
- **Manifest.**
  - It adds `name`, `version = "0.1.0"` and `description` for the marketplace.
  - `[[build]]` runs `go build -o bin/githerd .`, and `bin/` is gitignored.
  - The hooks run `./bin/githerd herdr sync`.
  - The rail's `[[actions]]` and `[[panes]]` are left to ticket 04.
- **Known cost.** Ticket 02 makes a stale repo due for fetching on every run, so every sync re-fetches stale repos. While a remote hangs, each sync's report waits up to the 20s fetch timeout. The lock keeps the syncs from piling up.
