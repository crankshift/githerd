# Code review: githerd v1 (`539b873...` main)

Status: ready-for-agent

Two-axis review after tickets 01–08. One implementer fixes every item below, then ticks it.

## Standards

Hard (documented standard breached):

- [x] S1. AGENTS.md "Run git reads with `--no-optional-locks`": the fetch module's git reads (`config --get core.sshCommand`, `for-each-ref`) lack the flag.
- [x] S2. Glossary vocabulary: the manifest's top comment says "unpushed or unpulled". Use ahead and behind. User-facing messages say "clone" (glossary: avoid) in the status command and the repo page; use repo. Also fix fixture and test comments where cheap. A method named after the git verb `clone` is fine.
- [x] S3. ADR-0004 says githerd never writes to a repo, but a failed fetch empties `FETCH_HEAD` (ticket 08). Amend the ADR with a Consequences note: githerd writes only git's own fetch metadata, never the working tree, index, refs or config.

Judgement calls (smells). Fix the ones where the copies behave differently; fix the rest where it's straightforward and tests stay green:

- [x] S4. Duplicated root scan: the rail's loader re-implements the Herdr adapter's scan (roots → discovery → last fetch → read all) without its dedupe of nested roots. One shared scan.
- [x] S5. Duplicated path helpers that differ: `canonical` (rail vs herdr, differ on ""), `under`/`underCanon`, `within` (string prefix vs `filepath.Rel`), and the TUI's root and start-repo pick, which repeats the adapter's deepest-match search. One implementation each.
- [x] S6. Duplicated git status parsing: the status and repo page packages have identical `git` helpers, and both parse the porcelain v2 `branch.*` headers, the `+N -M` counts and the no-upstream `rev-list` fallback. The repo page should reuse Status.
- [x] S7. Worker pool written twice (fetch, status read-all); attention/stale tally counted three times (status table, root tokens, TUI counts); stale map from fetch outcomes built three times; `OpenRail` parses the snapshot itself instead of the shared snapshot reader; the "no … id in %s" unmarshal shape appears three times; rail and page key handlers duplicate the `z/f/o/s/L/q` cases.
- [ ] S8. Repeated switches: entry kind in rows and detail lines; Remote/Gone/no-upstream in tracking and detail lines; the "exactly one field set" item switched again in Show.
- [x] S9. Names: `fetch.one` vs `fetchOne`; `detail.d`; `gitLines(…, differ bool)` (means "exit 1 is OK"); `hr`.
- [x] S10. Primitive obsession: the adapter's `Zoom(mode string)` while the TUI has a zoom-mode type. Give the adapter a typed mode (the adapter must not import the TUI).
- [ ] S11. Divergent change: `internal/herdr/rail.go` also holds the roots file and the opened-from context; split them out. The TUI page file (≈700 lines) mixes messages, state, keys and rendering; split by concern.
- [x] S12. Exported but package-local: `herdr.Source`, `status.FetchedAt`. Unexport.

## Spec

- [x] P1. US6 (scan depth) holds only for `githerd status --depth`; sync and the rail always scan 3 levels. Let a roots-file line carry an optional depth (`~/work depth=5`), used by sync and the rail; document it.
- [x] P2. `o` and `⧉` names are missing right after the rail opens. Opening the rail fires `workspace.focused`, so the hook's sync holds the lock, the rail's first sync returns not-run, and the fallback reads repos from disk without workspaces. Reading the snapshot needs no lock: the fallback must still read it and map workspaces.
- [x] P3. The rail's forced fetch (`f`) runs outside the sync lock. It can race a hook's fetch of the same repo, and the failure path then empties a `FETCH_HEAD` the other fetch just wrote. Route forced fetches through the sync (a force option) so they hold the lock.
- [x] P4. Lock race in sync: after finishing, a sync that finds the pending mark retries the lock. If another sync took it in between, it returns an empty result and throws away the run it already did. Return the completed result.
- [ ] P5. `s`/`L` split the rail downwards, so the shell or lazygit gets a quarter of the width. Spec: "beside the rail". Split the pane the rail was opened from (from the plugin context) to the right; fall back to splitting the rail down when that pane is gone.
- [ ] P6. `s`/`L` from a zoomed repo page leave the zoom on, so the new split is hidden. Unzoom the rail's pane first.
- [ ] P7. The rail's key-hint line omits `L` and `z`.
- [ ] P8. The `tab_bar_right` entry in the README runs githerd outside the plugin, without the hooks' state dir or lock. Add a `sync` action to the manifest and make the README entry `"$HERDR_BIN_PATH" plugin action invoke githerd.sync`. Check the qualified action id form in the research note.
- [ ] P9. The spec's JSON example shows `"root": "~/work"`; the code prints an absolute path (ticket 01 decision). Update the spec example.

## Accepted as recorded

- githerd writes `FETCH_HEAD` after a failed fetch (S3 documents it).
- With no roots, githerd still clears tokens it reported earlier.
- A clean repo with no upstream shows `∅` alone.
- Small extras (`q`/Ctrl-C, problem line, `help`, `--fetch-timeout`, unmerged files first, gone branches, fetch age in the page header) stay.
