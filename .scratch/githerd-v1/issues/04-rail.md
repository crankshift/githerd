# 04: The rail

**What to build:** `prefix g` runs the `githerd.rail` action. It opens the rail as a narrow split beside the current pane, or focuses the rail if it is already open. The rail lists every repo of the current workspace's root with its state glyphs: repos that need attention first, clean repos dimmed below. It filters with `/`, refreshes every 30 seconds (fetching with the TTL and syncing tokens), and `f` forces a fetch. See the spec: Rail and repo page. Design reference: branch `prototype/herdr-ui`, `navigator.html` (variant D3, rail mode).

**Blocked by:** 03

**Status:** resolved

- [x] Terminal layer: raw mode through `syscall`, alternate screen, redraw on resize, clean restore on exit
- [x] TUI model with `Update(model, key)` and `View(model, width, height)`; the terminal layer only runs commands and draws
- [x] The rail marks its pane with a pane token (`githerd` = `rail`); action `rail` focuses a pane in the current workspace with that token, else opens the plugin pane as a split with `--target-pane` set to the focused pane and narrows it with `pane resize --pane <rail> --direction right --amount 0.25`
- [x] The rail reads where it was opened from `HERDR_PLUGIN_CONTEXT_JSON` (focused pane and cwd at open), shows that workspace's root (or the first configured root), starts on that workspace's repo, and shows "No roots configured" guidance when there are none
- [x] Header with attention and stale counts and refresh countdown; `j`/`k` and arrows move; `/` filters (Enter keeps, Esc clears)
- [x] A refresh every 30 seconds runs the same refresh-and-sync as `githerd herdr sync`; `f` forces a fetch
- [x] Golden tests of `View` at a narrow and a wide size; tests of filter and ordering

## Comments

Notes from the implementation:

- Commands: the action is `githerd herdr rail`; the pane runs `githerd rail`. The manifest's comment shows the `[[keys.command]]` snippet for `prefix+g` (`command = "githerd.rail"`).
- The action also marks the pane it opens, before the rail process starts. Without that, pressing the key twice quickly would open two rails.
- A pane can keep the token after Herdr has forgotten it as a plugin pane, as after a live handoff. Then `plugin pane focus` fails. The action tries the next marked pane, and opens a fresh rail when none can be focused.
- The root and start repo come from the context's `focused_pane_cwd`, falling back to `workspace_cwd`. That is the pane split from, at open time. The start repo is the deepest repo holding that cwd; in a root workspace that is the root's own repo.
- The rail reads the repos from disk first, which is quick, then runs `herdr.Sync`. When another sync holds the lock, `Sync` doesn't run, so the rail reads from disk again and keeps the stale marks it had.
- `f` fetches every repo of the root shown with `Force`, then runs the sync.
- Extras beyond the ticket: `q` and Ctrl-C quit the rail, and a line above the keys shows the first problem from the last refresh.
- Left to later tickets: `⧉<workspace>` on rail lines (06); Enter, zoom and the repo page (05). In `internal/tui`, page mode slots in beside `railKey`/`railView`, and Herdr requests become new `Cmd` types.
- Tests: Seam 2 goldens in `internal/tui/testdata` (wide, narrow, moved, filter, refreshed, no roots). Seam 1 covers the action and the manifest with the fake `herdr`, plus a pty smoke test of `githerd rail`.
