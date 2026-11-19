# 04: The rail

**What to build:** `prefix g` runs the `githerd.rail` action. It opens the rail as a narrow split beside the current pane, or focuses the rail if it is already open. The rail lists every repo of the current workspace's root with its state glyphs: repos that need attention first, clean repos dimmed below. It filters with `/`, refreshes every 30 seconds (fetching with the TTL and syncing tokens), and `f` forces a fetch. See the spec: Rail and repo page. Design reference: branch `prototype/herdr-ui`, `navigator.html` (variant D3, rail mode).

**Blocked by:** 03

**Status:** ready-for-agent

- [ ] Terminal layer: raw mode through `syscall`, alternate screen, redraw on resize, clean restore on exit
- [ ] TUI model with `Update(model, key)` and `View(model, width, height)`; the terminal layer only runs commands and draws
- [ ] The rail marks its pane with a pane token (`githerd` = `rail`); action `rail` focuses a pane in the current workspace with that token, else opens the plugin pane as a split with `--target-pane` set to the focused pane and narrows it with `pane resize --pane <rail> --direction right --amount 0.25`
- [ ] The rail reads where it was opened from `HERDR_PLUGIN_CONTEXT_JSON` (focused pane and cwd at open), shows that workspace's root (or the first configured root), starts on that workspace's repo, and shows "No roots configured" guidance when there are none
- [ ] Header with attention and stale counts and refresh countdown; `j`/`k` and arrows move; `/` filters (Enter keeps, Esc clears)
- [ ] A refresh every 30 seconds runs the same refresh-and-sync as `githerd herdr sync`; `f` forces a fetch
- [ ] Golden tests of `View` at a narrow and a wide size; tests of filter and ordering
