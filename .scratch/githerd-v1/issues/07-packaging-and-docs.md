# 07: Packaging and docs

**What to build:** A new user can install githerd from the Herdr marketplace or with `go install`, configure roots, paste the sidebar and keybinding snippets, and get the rail working by following the README alone. The repo is tagged `v0.1.0` and carries the `herdr-plugin` topic.

**Blocked by:** 05, 06

**Status:** ready-for-agent

- [ ] README:
  - what githerd is, with a screenshot or text capture of the rail and repo page;
  - requirements (git, Go for the plugin build, Herdr 0.9.5+);
  - install via marketplace and `go install`;
  - the roots file;
  - `config.toml` snippets for space rows and `prefix+g`;
  - optional `tab_bar_right` entry;
  - keys;
  - limits (read-only, linux and macos, mapping lag after `cd`).
  All examples use made-up paths such as `~/work`.
- [ ] AGENTS.md updated for contributors: layout, test seams, how to run tests (use the `writing-for-agents` skill)
- [ ] `herdr plugin install` from a clean machine works end to end (manual check, noted in the ticket)
- [ ] Tag `v0.1.0`; GitHub topic `herdr-plugin` set
