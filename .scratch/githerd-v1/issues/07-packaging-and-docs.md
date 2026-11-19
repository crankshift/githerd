# 07: Packaging and docs

**What to build:** A new user can install githerd from the Herdr marketplace or with `go install`, configure roots, paste the sidebar and keybinding snippets, and get the rail working by following the README alone. The repo is tagged `v0.1.0` and carries the `herdr-plugin` topic.

**Blocked by:** 05, 06

**Status:** ready-for-human

- [x] README:
  - what githerd is, with a screenshot or text capture of the rail and repo page;
  - requirements (git, Go for the plugin build, Herdr 0.9.5+);
  - install via marketplace and `go install`;
  - the roots file;
  - `config.toml` snippets for space rows and `prefix+g`;
  - optional `tab_bar_right` entry;
  - keys;
  - limits (read-only, linux and macos, mapping lag after `cd`).
  All examples use made-up paths such as `~/work`.
- [x] AGENTS.md updated for contributors: layout, test seams, how to run tests (use the `writing-for-agents` skill)
- [ ] `herdr plugin install` from a clean machine works end to end (manual check, noted in the ticket)
  Needs a human on a clean machine with a live Herdr; agents never run the real `herdr`.
- [x] Tag `v0.1.0`; GitHub topic `herdr-plugin` set

## Comments

Docs done: `README.md` written, `AGENTS.md` updated for contributors. Left for the orchestrator:

- **The manual install check.** It needs a human running `herdr plugin install crankshift/githerd` on a clean machine.
- **The release.** Tag `v0.1.0`, push, and set the GitHub topic `herdr-plugin`. The README says githerd is in the marketplace, which holds only once the topic is set.
- **The `tab_bar_right` entry has no clean form.**
  - A tab-bar command gets `HERDR_BIN_PATH` but no `HERDR_PLUGIN_CONFIG_DIR` or `HERDR_PLUGIN_STATE_DIR`. Herdr has no command that prints the state dir, and the plugin's binary moves on every update.
  - So the README's entry runs the `go install` binary, finds the config dir with `herdr plugin config-dir githerd`, and keeps a state dir of its own. That sync doesn't share the hooks' lock. A repo whose fetch another sync has in flight reads as stale, because git empties `FETCH_HEAD` when a fetch starts, so it can show `⚠` until the next refresh. The README says so.
  - A `sync` action in the manifest running `./bin/githerd herdr sync` would fix this. The entry would become `"$HERDR_BIN_PATH" plugin action invoke githerd.sync >/dev/null`, with the plugin's binary, env and lock.

- Orchestrator: tagged `v0.1.0` and set the `herdr-plugin` topic after the v1 review was resolved. Only the manual `herdr plugin install` check is left, for a human.
