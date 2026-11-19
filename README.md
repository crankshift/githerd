# githerd

githerd shows which git repos under a root have uncommitted changes, commits to push or commits to pull. It finds every repo under the root on its own, nested repos included, so you don't keep a list. Use it as a command, `githerd status`, or as a [Herdr](https://herdr.dev) plugin that puts each repo's state in Herdr's sidebar and in a rail beside your agents.

githerd only reads your repos. It never stages, commits, checks out, pulls or pushes. When you want to change something, it opens a shell or lazygit in the repo.

The rail is a narrow pane listing every repo under the root, the ones that need attention first:

```text
│6/10 need attention · 1 stale                ↻ 30s│
│~/work                                            │
│──────────────────────────────────────────────────│
│repos/acme/api              ●3 ⇡2 ⧉api ⧉api-review│
│repos/acme/billing                              ⇡2│
│repos/acme/mobile                            ⇡1 ⇣2│
│repos/acme/web                           ⇣4 ⚠ ⧉web│
│repos/tools/cli                               ∅ ⇡3│
│sandbox/experiments                           ∅ ⇡2│
│. (root)                                         ✓│
│repos/acme/docs                                  ✓│
│repos/tools/lint-rules                           ⌀│
│sandbox/experiments/data                   ✓ ⧉data│
│──────────────────────────────────────────────────│
│enter page / filter o ws s shell f fetch q quit   │
```

Enter on a repo opens its repo page, zoomed to the whole tab, with a diff of the selected line:

```text
│repos/acme/api  feature/login → origin/feature/login  ●3 ⇡2                           fetched 4m ago│
│────────────────────────────────────────────────────────────────────────────────────────────────────│
│Changes 3                             │src/routes.go                                           +3 −1│
│  M  src/routes.go                    │staged                                                       │
│   M src/auth.go                      │@@ -1,7 +1,8 @@                                              │
│  ?? src/auth_test.go                 │   1    1  package routes                                    │
│To push 2                             │   2    2                                                    │
│  b81e0d4 Validate login payload    2h│   3    3  import "example.com/api/auth"                     │
│  3f2a9c1 Add session refresh endp… 5h│   4    4                                                    │
│To pull 0                             │   5    5  func Register(r *Router) {                        │
│  nothing to pull                     │   6    6      r.Post("/login", auth.Login)                  │
│Branches 5                            │        7 +    r.Post("/session/refresh", auth.Refresh)      │
│  * feature/login                   ⇡2│   7    8  }                                                 │
│    fix/token-expiry     ∅ no upstream│@@ -40,3 +41,4 @@ func Register(r *Router) {                 │
│    main                            ⇣3│  40   41  func health(w Writer) {                           │
│    old/feature                   gone│  41      -    w.Write("ok")                                 │
│    origin/release/1.9          remote│       42 +    w.WriteHeader(200)                            │
│Log                                   │       43 +    w.Write("ok\n")                               │
│  9c0d1e2 Add login form            1d│  42   44  }                                                 │
│  77a1f3b Set up the router         3d│                                                             │
│  e4b2c90 Initial commit           1mo│                                                             │
│                                      │                                                             │
│────────────────────────────────────────────────────────────────────────────────────────────────────│
│j/k move  J/K scroll  esc back  o ws  s shell  L lazygit  z zoom  f fetch  q quit                   │
```

These are text captures from the test suite. On screen the selected line is highlighted, clean repos are dimmed and the glyphs are coloured.

## Repo states

| State | Rail glyph | Meaning |
| --- | --- | --- |
| Uncommitted | `●N` | N changes in the working tree or index, untracked files included |
| Ahead | `⇡N` | N commits on the current branch that its upstream lacks |
| Behind | `⇣N` | N commits on the upstream that the branch lacks, as of the last fetch |
| No upstream | `∅` | the branch has no upstream; ahead then counts its commits that are on no remote branch |
| Detached | `⌀` | HEAD points at a commit, not a branch |
| Stale | `⚠` | the last fetch failed, so behind may be out of date |
| Clean | `✓` | none of uncommitted, ahead, behind or no upstream |

A repo **needs attention** when it is uncommitted, ahead, behind or has no upstream. Detached alone does not count, so a repo pinned to a tag stays clean. On the rail, `⧉name` follows the glyphs for each Herdr workspace mapped to the repo.

## Requirements

- Linux or macOS.
- git.
- For the Herdr plugin:
  - Herdr 0.9.5 or later.
  - Go 1.22 or later on the `PATH` of the shell you run `herdr plugin install` from. Herdr builds the plugin from source when it installs or updates it.
- For `go install`: Go 1.22 or later.
- Optional: lazygit, on the `PATH` of the Herdr server, for the `L` key.

githerd uses only Go's standard library.

## Install

### As a Herdr plugin

Install githerd from the Herdr marketplace, or by name:

```sh
herdr plugin install crankshift/githerd
```

Herdr clones the repo, shows a preview of what the plugin runs, then builds it with `go build -o bin/githerd .`. `herdr plugin update githerd` fetches and builds it again, so Go must still be on the `PATH` then.

Then set it up, as described in the next section.

### As a command

```sh
go install github.com/crankshift/githerd@latest
```

This installs only the `githerd` command, in `$(go env GOPATH)/bin` unless `GOBIN` is set. `githerd status` needs nothing but git. `githerd herdr …` and `githerd rail` work only when Herdr runs them.

## Set up the plugin

### 1. List your roots

A root is a directory githerd scans for repos. List yours in the file `roots` in the plugin's config directory:

```sh
"$EDITOR" "$(herdr plugin config-dir githerd)/roots"
```

Write one path per line. Each path is absolute or starts with `~`. Blank lines and lines starting with `#` are ignored:

```text
# githerd roots
~/work
~/src/oss depth=5
```

githerd reads the file again on every refresh, so there is nothing to restart. With no roots it reports nothing, and the rail shows where the file goes.

githerd scans 3 directory levels below each root. It skips hidden directories and `node_modules`, and descends into repos to find nested ones. To scan a root deeper or shallower, put `depth=N` after its path, as for `~/src/oss` above: N levels below the root, or `0` for the root alone. The sidebar tokens and the rail both use it.

### 2. Add the tokens to the sidebar

githerd reports three workspace tokens to Herdr:

| Workspace | Token | Value |
| --- | --- | --- |
| Root workspace | `$repos` | `3/12` when 3 of the root's 12 repos need attention, `✓12` when none do |
| Root workspace | `$stale` | `⚠2` when 2 of its repos are stale |
| Mapped workspace | `$changes` | `●3` for 3 uncommitted changes, `●3 ∅` when the branch also has no upstream, `∅` for no upstream alone |
| Mapped workspace | `$stale` | `⚠` when its repo is stale |

A workspace is a root workspace when its first pane's cwd lies inside a root but inside no nested repo. It is a mapped workspace when that cwd lies inside a repo nested under the root; it maps to the deepest such repo. A token that doesn't apply is cleared, and Herdr hides it.

Show the tokens in the space rows of `~/.config/herdr/config.toml`, here beside Herdr's own `branch` and `git_status`:

```toml
[ui.sidebar.spaces]
rows = [
  ["state_icon", "workspace"],
  ["branch", "git_status", "$repos", "$changes", "$stale"],
]
```

To colour them, use style tables instead. `$repos` turns green when it starts with `✓`:

```toml
[ui.sidebar.spaces]
rows = [
  ["state_icon", "workspace"],
  ["branch", "git_status",
    { token = "$repos", fg = "#e5b567", rules = [{ starts_with = "✓", fg = "#9ccc7a" }] },
    { token = "$changes", fg = "#e5b567" },
    { token = "$stale", fg = "#f07178" }],
]
```

### 3. Bind a key to the rail

```toml
[[keys.command]]
key = "prefix+g"
type = "plugin_action"
command = "githerd.rail"
```

`prefix g` focuses the rail of the current workspace. When the workspace has none, it opens the rail to the right of the focused pane, a quarter of the width. With Herdr's default prefix, press `ctrl+b`, then `g`.

### Optional: refresh on a timer

Herdr gives plugins no timers. The sidebar refreshes when Herdr starts, when you create, close or focus a workspace, when an agent finishes (turns done or idle), and every 30 seconds while a rail is open. To refresh on a timer as well, add a `tab_bar_right` command entry to `config.toml`. Herdr runs it with `/bin/sh -lc`:

```toml
[ui]
tab_bar_right = [
  { type = "command", command = 'HERDR_PLUGIN_CONFIG_DIR="$("$HERDR_BIN_PATH" plugin config-dir githerd)" HERDR_PLUGIN_STATE_DIR="$HOME/.cache/githerd-tab-bar" "$HOME/go/bin/githerd" herdr sync', interval_seconds = 300, timeout_seconds = 120 },
]
```

- **The binary.** This runs the `githerd` from `go install`, not the plugin's own build, so install it first and keep it at the plugin's version. Change `$HOME/go/bin` if `go env GOPATH` says otherwise.
- **The state directory.** Herdr gives this command no `HERDR_PLUGIN_STATE_DIR`, so the entry keeps its own, and its sync doesn't share the plugin's lock. When the two sync at once, a repo one of them is fetching can show `⚠` until the next refresh.
- **The tab bar.** The sync prints nothing, so the entry adds nothing to the tab bar.

## Use

### `githerd status`

```text
githerd status [root] [--json] [--depth N] [--fetch [--ttl D] [--force] [--fetch-timeout D]]
```

It lists every repo under `root`, by default the current directory, with a summary line:

```console
$ githerd status ~/work
REPO                      BRANCH        UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE
.                         main          0            0      0       origin/main
repos/acme/api            main          3            0      0       origin/main
repos/acme/billing        main          0            2      0       origin/main
repos/acme/docs           main          0            0      0       origin/main
repos/acme/mobile         main          0            1      2       origin/main
repos/acme/web            main          0            0      4       origin/main
repos/tools/cli           spike/parser  0            3      0       -            no upstream
repos/tools/lint-rules    7c41d0e       0            0      0       -            detached
sandbox/experiments       main          0            2      0       -            no upstream
sandbox/experiments/data  main          0            0      0       origin/main
6 of 10 repos need attention
```

| Flag | Default | What it does |
| --- | --- | --- |
| `--depth N` | `3` | how many directory levels below the root to scan |
| `--json` | | print JSON instead of the table |
| `--fetch` | | fetch each repo's upstream remote before reporting |
| `--ttl D` | `15m` | with `--fetch`, skip a repo fetched less than `D` ago, or whose fetch failed less than `D` ago |
| `--force` | | with `--fetch`, fetch every repo, whatever the TTL |
| `--fetch-timeout D` | `20s` | with `--fetch`, give up on a repo's fetch after `D` |

Flags may come before or after `root`. `D` is a Go duration, such as `90s` or `1h`.

- **Without `--fetch`**, githerd stays local. It is instant, works offline, and behind counts are as of the last fetch.
- **With `--fetch`**, up to 8 repos fetch at once. A fetch never prompts: git runs with `GIT_TERMINAL_PROMPT=0`, and ssh in batch mode unless you set `GIT_SSH_COMMAND`, `GIT_SSH` or `core.sshCommand` yourself.
- **A failed fetch** marks the repo stale. The table notes `stale`, the summary adds `, K stale`, and the reason goes to stderr.

`--json` prints the same data. `fetchedAt` is when the repo was last fetched. `stale` holds why its last fetch failed, and is set only with `--fetch`:

```json
{
  "root": "/home/you/work",
  "repos": [
    {
      "path": "repos/acme/api",
      "branch": "feature/login",
      "detached": null,
      "upstream": "origin/feature/login",
      "uncommitted": 3,
      "ahead": 2,
      "behind": 0,
      "needsAttention": true,
      "stale": null,
      "fetchedAt": "2026-03-04T05:06:07Z"
    }
  ]
}
```

The exit code is 0 even when repos need attention or are stale. It is 1 when a repo or the root can't be read, and 2 for a usage error.

### The rail

The rail shows the root holding the pane you opened it from, or the first root in the `roots` file when that pane lies outside every root. It starts on the repo you were in. Every 30 seconds it refreshes: it fetches repos last fetched more than 15 minutes ago and updates the sidebar tokens. The header counts down to the next refresh.

| Key | What it does |
| --- | --- |
| `j` / `k`, `↓` / `↑` | move |
| `enter`, `l`, `→` | open the repo page and zoom the pane |
| `/` | filter: type letters that appear in that order in the repo's path, in any case; `enter` keeps the filter, `esc` clears it |
| `esc` | clear the filter |
| `o` | focus the workspace mapped to the repo, or create one there when none is |
| `s` | open a shell in the repo beside the rail: in a split to the right of the pane you opened the rail from, or below the rail when that pane is gone |
| `L` | open lazygit in the repo, where `s` opens a shell, when lazygit is on the `PATH` |
| `f` | fetch every repo of the root now, whatever the TTL |
| `z` | zoom the rail's pane, or unzoom it |
| `q`, `ctrl+c` | quit |

`o` on the root's own line focuses a root workspace of that root. Until the rail's first sync has read the workspaces, `o` opens nothing and says so, so it never creates a duplicate.

### The repo page

The repo page lists:

- **Changes.** Each file has git's two status columns, staged then unstaged. Untracked files show as `??` and their diff shows them added in full.
- **To push.** When the branch has no upstream, this section is Local commits instead.
- **To pull.**
- **Branches.** Local branches with their ahead and behind, `∅ no upstream`, or `gone` when the upstream was deleted. Then remote-only branches.
- **Log.** Recent commits of HEAD.

Beside it is the selected line's detail: a file's diff, a whole commit, or a branch's summary and recent commits.

| Key | What it does |
| --- | --- |
| `j` / `k`, `↓` / `↑` | move through the lines |
| `J` / `K` | scroll the diff |
| `esc`, `h`, `←` | go back to the rail and unzoom |
| `o`, `s`, `L` | as on the rail, for the page's repo |
| `f`, `z`, `q`, `ctrl+c` | as on the rail |

## Limits

- **Read-only.** githerd changes no branch, index or working tree. To learn about behind commits, the plugin and `--fetch` run `git fetch`, which updates remote-tracking branches. After a failed fetch githerd empties `FETCH_HEAD`, the file git rewrites on every fetch, to remember the failure.
- **Linux and macOS only.** Windows is out of scope: Herdr's plugins are in preview there, and githerd's terminal layer and sync lock are Unix-only.
- **Local Herdr only.** Herdr's remote machines are out of scope.
- **Mapping lags after `cd`.** A workspace maps through its first pane's cwd, and a `cd` fires no Herdr event githerd can hook. After a `cd` into another repo, the workspace keeps its old tokens until the next refresh: focus another workspace and come back, or press `f` in the rail.
- **First pane.** githerd takes the first pane listed in the workspace's first tab as the workspace's first pane. If you moved panes around in that tab, it may be a different pane from the one Herdr uses for its own `branch` token.

## Contributing

See [`AGENTS.md`](AGENTS.md) for the layout and the tests.

## License

[MIT](LICENSE)
