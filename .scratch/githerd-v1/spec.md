# Spec: githerd v1

Status: ready-for-agent

## Problem Statement

A developer keeps one root directory with many independent git clones nested inside it. The clones are gitignored by the root and are not submodules. To learn which clones have uncommitted changes, commits not yet pushed or commits not yet pulled, they `cd` into each one or run a hand-written loop over `git status` and `git rev-list`. Nothing shows this at a glance. Nothing shows it inside Herdr, the terminal workspace manager where their coding agents run, either. When an agent finishes, they cannot tell which repos it left uncommitted without going to look. Herdr's sidebar shows a workspace's branch and ahead/behind counts, but only for one repo per workspace, and never uncommitted changes.

## Solution

githerd is a command-line tool plus a Herdr plugin, written in Go with only the standard library.

- `githerd status` scans a root, finds every repo under it and prints a table of their states. It has a JSON mode for scripts. It needs only git.
- The Herdr plugin reports tokens to Herdr's sidebar:
  - The root workspace gets a summary: how many repos need attention, and how many are stale.
  - Each mapped workspace gets its repo's uncommitted count.
- A keybinding opens the rail: a narrow split pane that lists every repo under the root with its state and refreshes itself.
- Enter on a repo opens its repo page and zooms the pane. The page shows changes, commits to push and to pull, branches and log, with a diff of the selected line.
- From the rail or the repo page the developer can:
  - jump to the workspace that maps to the repo, or create one;
  - open a shell or lazygit beside the rail.
- githerd never changes a repo itself.

## User Stories

1. As a developer with many clones under one root, I want one command that lists every repo and its state, so that I stop `cd`-ing into each one.
2. As a developer, I want repos found automatically under the root, so that I don't maintain a list of clones.
3. As a developer, I want repos nested inside other repos found too, so that a vendored or nested clone is not missed.
4. As a developer, I want the root itself listed when it is a repo, so that changes to the root are visible as well.
5. As a developer, I want hidden directories and `node_modules` skipped, so that scanning stays fast and free of noise.
6. As a developer, I want to set the scan depth, so that deeper layouts work.
7. As a developer, I want untracked files to count as uncommitted, so that a new file I forgot to add still shows up.
8. As a developer, I want to see commits that are ahead of the upstream, so that I know what is not pushed.
9. As a developer, I want to see commits that are behind the upstream, so that I know what to pull.
10. As a developer, I want a branch with no upstream flagged, together with how many of its commits are on no remote, so that never-pushed work is not lost.
11. As a developer, I want a detached HEAD shown but not counted as needing attention, so that repos pinned to a tag don't nag me.
12. As a developer, I want a single "needs attention" count, so that one number tells me whether anything is pending.
13. As a developer, I want `githerd status` to stay local by default, so that it is instant and works offline.
14. As a developer, I want `--fetch` to update remote state first, so that behind counts are fresh when I ask for it.
15. As a developer, I want fetches to skip repos fetched within the TTL, so that refreshing often doesn't hammer remotes.
16. As a developer, I want fetches never to prompt for credentials and to time out, so that one broken remote can't hang the tool.
17. As a developer, I want a repo whose fetch failed marked stale, so that I know its behind count may be out of date.
18. As a script author, I want `githerd status --json`, so that I can build on the same data.
19. As a Herdr user, I want my root workspace's sidebar row to show how many repos need attention, so that I see it without opening anything.
20. As a Herdr user, I want a workspace opened inside a repo to show that repo's uncommitted count next to Herdr's own branch and ahead/behind, so that the one thing Herdr lacks is filled in.
21. As a Herdr user, I want stale repos flagged in the sidebar, so that a failing remote doesn't go unnoticed.
22. As a Herdr user, I want the sidebar to refresh when an agent finishes, so that I see right away what the agent left uncommitted.
23. As a Herdr user, I want the sidebar to refresh when I create, close or focus a workspace, so that new workspaces get their token without a manual step.
24. As a Herdr user, I want tokens restored after Herdr restarts, so that the sidebar is never silently blank.
25. As a Herdr user, I want to list my roots in one plain file, so that githerd knows which directories to watch.
26. As a Herdr user with several roots, I want each workspace matched to the root that contains it, so that each root's summary lands on the right workspace.
27. As a Herdr user, I want a workspace whose cwd sits anywhere inside a repo mapped to that repo, so that a workspace opened in a subdirectory still counts.
28. As a Herdr user, I want one key to open the rail, or focus it if it is already open, so that the repo list is always one keystroke away.
29. As a Herdr user, I want the rail narrow beside my agent pane, so that it doesn't take over the screen.
30. As a Herdr user, I want the rail to refresh itself while it is open, so that it stays current without hooks.
31. As a Herdr user, I want repos that need attention at the top and clean repos dimmed below them, so that the list reads at a glance.
32. As a Herdr user with dozens of repos, I want to filter the rail by typing `/`, so that I can reach a repo quickly.
33. As a Herdr user, I want the rail to start on the repo of the workspace I opened it from, so that the context carries over.
34. As a Herdr user, I want each rail line to show which workspace maps to that repo, so that I can see which repos already have a workspace.
35. As a Herdr user, I want Enter on a repo to open its repo page and zoom the pane, so that diffs get the full width.
36. As a Herdr user, I want Esc on the repo page to return to the rail and unzoom, so that my layout comes back.
37. As a Herdr user, I want the repo page to list changed files with a diff of the selected one, so that I can review work without leaving Herdr.
38. As a Herdr user, I want staged and unstaged changes told apart, so that I know what a commit would include.
39. As a Herdr user, I want untracked files shown in full as added, so that new files are reviewable too.
40. As a Herdr user, I want commits to push and commits to pull listed, with each commit's diff, so that I know what pushing or pulling would do.
41. As a Herdr user, I want local and remote-only branches listed with their ahead/behind, so that I see branch state without `git branch -vv`.
42. As a Herdr user, I want the recent log, so that I have context for the current branch.
43. As a Herdr user, I want to scroll a long diff while keeping my place in the list, so that big changes are readable.
44. As a Herdr user, I want `o` to focus the workspace that maps to the repo, so that I don't end up with duplicate workspaces.
45. As a Herdr user, I want `o` to create a workspace for the repo when none exists, so that I can start working there at once.
46. As a Herdr user, I want `s` to open a shell in the repo beside the rail, so that I can act on what I just reviewed.
47. As a Herdr user, I want `L` to open lazygit in the repo beside the rail when it is installed, so that I can stage and commit visually.
48. As a Herdr user, I want `f` to force a fetch, so that I can refresh remote state on demand.
49. As a Herdr user, I want a clear message when no roots are configured, so that I know what to set up.
50. As a Herdr user, I want to install githerd from the Herdr marketplace, so that setup is one command.
51. As a Herdr user, I want copy-paste snippets for the sidebar rows and the keybinding, so that I can wire up the UI quickly.
52. As a contributor, I want the tool to depend on nothing beyond Go's standard library, git and Herdr, so that it is easy to audit and build.

## Implementation Decisions

**Vocabulary.** Root, repo, workspace, root workspace, mapped workspace, rail, repo page and the repo states (uncommitted, ahead, behind, no upstream, detached, stale, needs attention, clean) are defined in `GLOSSARY.md`.

**ADRs.** ADR-0001 to ADR-0005 apply: separate clones and a Herdr plugin; Go with the standard library, built at install; roots from config and workspaces mapped through their first pane; read-only; refresh without Herdr timers and tokens without a TTL.

**Modules.** Each one is a deep module behind a small interface.

- **Discovery.** Given a root and a depth (default 3), it returns the repos under it.
  - A repo is a directory that contains `.git`, either a directory or a file.
  - It descends into repos, so nested repos are found.
  - It skips hidden directories and `node_modules`.
  - The root comes first when it is a repo.
- **Status.** Given a repo, it returns its state: current branch or detached commit, upstream, uncommitted count (porcelain entries, untracked included), ahead and behind.
  - With no upstream, ahead counts the commits that are on no remote branch.
  - Needs attention = uncommitted or ahead or behind or no upstream. Detached alone does not count.
- **Fetch.** Given repos, a TTL and a force flag, it fetches the remote of each repo's upstream whose `FETCH_HEAD` is older than the TTL, or every repo when forced.
  - The TTL clock is the mtime of `FETCH_HEAD`, so no state file is needed.
  - Fetches run in parallel with bounded concurrency and a per-repo timeout.
  - They never prompt: terminal prompts are off, and SSH runs in batch mode unless the user has set their own SSH command.
  - A failed fetch marks the repo stale and keeps the error message.
  - Defaults: `githerd status` does not fetch; the plugin uses a 15-minute TTL.
- **Status command.** `githerd status [root] [--json] [--fetch] [--depth N]`. It prints a table (repo, branch, uncommitted, ahead, behind, upstream, note) and a summary line. JSON mode prints the same data. The JSON shape below came from the prototype:

  ```json
  {"root": "~/work", "repos": [
    {"path": "repos/acme/api", "branch": "feature/login", "detached": null,
     "upstream": "origin/feature/login", "uncommitted": 3, "ahead": 2, "behind": 0,
     "needsAttention": true, "stale": null, "fetchedAt": "…"}
  ]}
  ```

- **Herdr adapter.** It runs as `githerd herdr sync`, from the startup hook, the event hooks and the rail.
  - It reads the roots file in the plugin config dir: one path per line, `~` expanded, blank lines and `#` comments ignored. A line may end in `depth=N`, the scan depth below that root (default 3), as in `~/work depth=5`; the sync and the rail both scan that deep. With no roots it reports nothing.
  - It maps workspaces through `herdr api snapshot`, using the cwd of the first pane listed for each workspace's first tab. The snapshot has no layout tree, so this stands in for Herdr's root pane; the two differ only when the user has moved panes in that tab.
    - a cwd inside a root but inside no nested repo makes a root workspace;
    - a cwd inside a nested repo makes a mapped workspace, mapped to the deepest repo;
    - anything else is outside, and githerd clears any token it reported earlier.
  - It reports through `$HERDR_BIN_PATH workspace report-metadata` with `--source githerd`, one call per workspace carrying every `--token` and `--clear-token`. There is never a `--ttl-ms`.
  - Herdr silently ignores a report whose `--seq` is not higher than the last one from the same source. So `--seq` is the larger of the Unix time in milliseconds and the previous value plus one, and the previous value is kept in the plugin state dir under the sync lock.
  - The token values below came from the prototype:

    | Workspace | Token | Value |
    | --- | --- | --- |
    | root | `repos` | `N/T` when N repos need attention, else `✓T` |
    | root | `stale` | `⚠K` when K repos are stale, else cleared |
    | mapped | `changes` | `●N`, with ` ∅` appended when the repo has no upstream; cleared when neither applies |
    | mapped | `stale` | `⚠` when its repo is stale, else cleared |

  - For `pane.agent_status_changed` it reads the event JSON and acts only when the agent is done or idle.
  - Hooks fire in bursts: `workspace.focused` fires on every change of focused pane, and one finished agent usually reports `done` and then `idle`. So a sync takes a lock in the plugin state dir. A sync that finds the lock held leaves a pending mark and exits; the running sync checks for the mark when it finishes and runs once more.
- **Rail and repo page (TUI).** A pure model with `Update(model, key)` and `View(model, width, height)`, driven by a thin terminal layer: raw mode through `syscall`, alternate screen, resize on `SIGWINCH`.
  - **Rail.**
    - It learns where it was opened from `HERDR_PLUGIN_CONTEXT_JSON`: the focused pane and its cwd at open time. `HERDR_PANE_ID` is the rail's own pane.
    - It lists the repos of the root of the workspace it was opened from, or the first configured root when that workspace is outside every root.
    - It marks its own pane with a pane token (`githerd` = `rail`), so the action can find it in the snapshot. Herdr's own plugin-pane registry can't be queried and is lost on restart. Needs attention comes first, then clean repos dimmed.
    - Each line shows the state glyphs and the name of any mapped workspace.
    - `/` filters the list.
    - It refreshes every 30 seconds. Each refresh fetches with the TTL and also runs a sync.
  - **Repo page.**
    - Sections: Changes (staged and unstaged told apart), To push (Local commits when there is no upstream), To pull, Branches (local with ahead/behind, plus remote-only), Log.
    - A live diff of the selected line: a file diff, a whole commit, or a branch summary with its recent commits. Untracked files show as added in full.
    - `J`/`K` scroll the diff.
  - **Keys.**
    - Movement: `j`/`k` and arrows move. Enter/`l`/→ opens the repo page. Esc/`h`/← goes back.
    - Actions: `o`, `s`, `L`, `f`, `z`.
  - **Herdr requests.** The model returns these as commands; the terminal layer runs them. Zoom, resize and split always name the pane with `--pane`; without it Herdr acts on whichever pane has focus.
    - zoom on with the repo page and off with Esc;
    - `workspace focus <id>` when a workspace maps to the repo;
    - otherwise `workspace create --cwd <repo> --label <name> --focus`;
    - `pane split` beside the rail with the repo as cwd;
    - `pane run <pane> lazygit` only when lazygit is on PATH; otherwise a message.
- **Plugin manifest** at the repo root:
  - id `githerd`, `min_herdr_version` 0.9.5, platforms linux and macos;
  - `[[build]]` runs `go build`;
  - action `rail`: if the snapshot shows a pane in the current workspace carrying the rail's pane token, focus it. Otherwise open the rail pane as a split with `--target-pane` set to the focused pane, then narrow it with `pane resize --direction right --amount 0.25`. The amount is a ratio delta, so the rail ends up about a quarter of the width. After a Herdr restart the old rail comes back as a plain shell without the token, so the action opens a fresh one;
  - pane `rail`;
  - `[[startup]]` and `[[events]]` (`workspace.created`, `workspace.closed`, `workspace.focused`, `pane.agent_status_changed`) all run the sync.
- **Module path:** `github.com/crankshift/githerd`. The CLI installs with `go install`.
- **Environment.** The `[[build]]` step inherits the PATH of the shell that runs `herdr plugin install`, so `go` must be on it. Runtime commands get the Herdr server's environment, which includes the PATH used for the lazygit check. Relative commands such as `./bin/githerd` resolve against the plugin root, which changes on every update, so the plugin root is never stored.

## Testing Decisions

- **What makes a good test.** It asserts external behaviour: command output, the Herdr calls made, the rendered text. It never asserts internal functions. Tests survive refactors of the internals.
- **Seam 1: the githerd binary as a black box.**
  - Fixtures are real git repos in temp dirs: a bare "remote" plus clones, with commits arranged to produce every state (uncommitted, untracked, ahead, behind, diverged, no upstream with local commits, detached, nested repo, unreachable remote for stale).
  - Herdr is a fake `herdr` reached through `$HERDR_BIN_PATH`: a Go test binary that serves canned `api snapshot` JSON and records its argv.
  - Covers `status`, `--json`, fetch with TTL (by setting `FETCH_HEAD` mtimes), stale, sync, roots file, workspace mapping, token values, token clearing, event filtering and the lock.
- **Seam 2: the TUI model.**
  - Golden text files of `View` at fixed sizes after scripted key sequences, fed with fixture data.
  - Assertions on the Herdr commands `Update` returns.
  - Covers rail order and dimming, filter, repo page sections, diff scrolling, zoom requests, and the `o` / `s` / `L` decisions.
- **Prior art.** None in this repo yet; the first ticket builds the fixture helper.

## Out of Scope

- Any write to a repo: stage, commit, checkout, pull, push or stash (ADR-0004).
- Windows (Herdr plugins are in preview there) and Herdr remote machines.
- A popup or overlay UI. The rail and the repo page replace it.
- An explicit list of repos, and clones that are listed but missing.
- Stashes.
- A `tab_bar_right` timer by default. The README documents it as optional.
- Running the rail outside Herdr.

## Further Notes

- Prototype: branch `prototype/herdr-ui`. `navigator.html` is the D3 design with every decision applied; verdicts are in `.scratch/githerd-v1/prototype.md`.
- Herdr facts and their sources: `.scratch/research/herdr-v0.9.5.md`, plus exact CLI flags, JSON shapes and exit codes in `.scratch/research/herdr-json-v0.9.5.md`. Note that `report-metadata` and `pane run` print nothing on success, and errors are JSON on stderr with exit 1.
- Known lag: a `cd` inside a pane fires no hookable event, so a workspace's mapping refreshes only on the next trigger (ADR-0003).
