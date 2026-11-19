# Herdr v0.9.5: facts for plugin design

Facts only. Collected 2026-11-19 from the docs at tag `v0.9.5`, which are indexed at <https://herdr.dev/llms.txt>.

- **Docs** are cited as `[page § Heading]`.
- **Source** is cited as `[src: path]`. These are facts read from the v0.9.5 source where the docs say nothing. Each one is labelled "Docs silent; source shows…".
- **"Not documented"** means neither the docs nor a targeted source read gave an answer.

## Source keys

| Key | URL |
| --- | --- |
| plugins | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/plugins.mdx |
| cli | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/cli-reference.mdx |
| config | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/configuration.mdx |
| socket | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/socket-api.mdx |
| market | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/marketplace.mdx |
| install | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/install.mdx |
| win | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/windows-beta.mdx |
| session | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/session-state.mdx |
| concepts | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/concepts.mdx |
| machines | https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/connecting-machines.mdx |
| schema | https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/api/herdr-api.schema.json (the same document `herdr api schema --json` prints) |
| src: path | https://github.com/herdrdev/herdr/blob/v0.9.5/ followed by the path |
| examples | https://github.com/ogulcancelik/herdr-plugin-examples (default branch, commit `18709cd`) |

---

## 1. OS support and the manifest `platforms` field

### OS support

- Herdr publishes stable binaries for Linux, macOS and Windows. Windows is "generally available, with documented platform-specific limitations" [install § Install Herdr (intro)].
- Release assets [install § Download manually]:
  - `herdr-linux-x86_64`
  - `herdr-linux-aarch64`
  - `herdr-macos-x86_64`
  - `herdr-macos-aarch64`
  - `herdr-windows-x86_64.zip`
- "Windows ARM64 runs the x86_64 build under Windows emulation" [install § Requirements].
- Windows support:
  - "Native Windows support is generally available" [win § Windows support (intro)].
  - The capability table lists **`Plugins | preview`** [win § Supported on Windows].
  - "GitHub install, local link, build commands, actions, events, and plugin panes are best-effort on Windows. Commands are argv commands and must be Windows-compatible … Unix-only examples using `sh` or Bash need Windows-specific alternatives." [win § Supported on Windows]
- Windows command resolution: build, action and event commands resolve `PATHEXT` shims such as `npm.cmd` and `bun.cmd`. "Pane commands use Herdr's normal Windows pane launcher and must still be valid Windows argv commands." [plugins § Panes]
- Socket transport: a Unix socket on Unix, a named pipe on Windows. Calling the CLI through `HERDR_BIN_PATH` hides the difference [plugins § Commands and environment].

### `platforms` semantics

- Allowed identifiers are `linux`, `macos` and `windows` [socket § Plugin APIs].
- An unknown value causes the error `invalid_plugin_platform: unknown platform '…'` [src: src/app/api/plugins/manifest.rs].
- The top-level `platforms` declares where the plugin can run. Each `[[build]]`, `[[startup]]`, `[[actions]]`, `[[events]]`, `[[panes]]` and `[[link_handlers]]` entry may set its own `platforms`. "item-level platforms override the top-level list" [plugins § Manifest]. If an item omits it, the item inherits the plugin-level list [socket § Plugin APIs].
- "Local plugins without top-level `platforms` link with a warning" [plugins § Manifest]. The warning text is `manifest does not declare platforms; platform support unknown` [src: manifest.rs].
- Invoking an action or opening a pane whose effective platforms exclude the current OS returns `platform_unsupported` [socket § Plugin APIs].
- "Platform filters skip unsupported build commands" [win § Supported on Windows].
- Docs silent; source shows that startup hooks and event hooks whose platforms don't match are silently skipped [src: src/app/api/plugins/runtime.rs].
- `plugin.action.list` returns each action's effective `platforms` [socket § Plugin APIs].
- The marketplace index records `platforms` [market § What a listing shows].
- Not documented: whether `plugin install` refuses a plugin whose top-level `platforms` excludes the current OS.

## 2. `[[build]]`, distribution and examples

### When builds run

- "Build commands run during GitHub `plugin install` after confirmation and before Herdr registers the plugin." [plugins § Build commands]
- "`plugin link` does not run build commands; local authors build their working tree themselves." [plugins § Build commands]
- `plugin update`:
  - It refetches the ref recorded at install and skips unchanged commits [plugins § Install and link].
  - Docs silent on builds during update; source shows update goes through the same install routine. It runs the build commands inside a new managed installation directory. On failure it prints "Plugin was not updated." [src: src/cli/plugin.rs `install_github_plugin`]
- Updates and reinstalls leave the previous installation in place. Running commands and panes keep their old files, and new invocations use the new installation [plugins § Install and link].

### Working directory

- Not stated in the docs. The failure report does include the "working directory" [plugins § Build commands].
- Docs silent; source shows it is the manifest directory inside the new installation's checkout: the repo root, or the `subdir` given at install [src: src/cli/plugin.rs `run_plugin_build_commands`, `manifest_root`].

### Environment

- "Build commands are plain argv commands too, but they do not receive runtime plugin context or Herdr socket env." [plugins § Build commands]
- Commands otherwise "run as your user, inherit your environment" [plugins § Trust and security].
- Docs silent; source shows the build process has these removed: `HERDR_SOCKET_PATH`, the client socket variable, `HERDR_SESSION`, `HERDR_BIN_PATH`, `HERDR_ENV`, `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID`, `HERDR_PANE_ID` and every `HERDR_PLUGIN_*` variable. Stdin is null [src: src/cli/plugin.rs `scrub_herdr_runtime_env`].

### Failure handling

- "If a build command fails, install aborts and the plugin is not registered." [plugins § Build commands]
- The report shows "the plugin id, build index, working directory, command, exit status or spawn error, and capped stdout/stderr" [plugins § Build commands].
- "changing `herdr-plugin.toml` after the install preview aborts install" [plugins § Build commands].
- Timeout: not documented. Docs silent; source shows no timeout on build commands [src: src/cli/plugin.rs].

### Toolchains and distribution

- "Plugin authors should document required system tools such as `cargo`, `npm`, `bun`, or `lua`; Herdr reports build failures but does not install missing toolchains." [plugins § Build commands]
- Prebuilt binaries, release assets or per-platform downloads: **not documented**. The manifest has no field for them; the fields are `id`, `name`, `version`, `min_herdr_version`, `description`, `platforms`, `build`, `startup`, `actions`, `events`, `panes` and `link_handlers` [src: manifest.rs `RawPluginManifest`].
- How install fetches the repo:
  - "`plugin install` accepts GitHub shorthand only … It clones with `git`, shows a preview in interactive terminals, builds in a new Herdr-managed installation directory, and registers it." [plugins § Install and link]
  - Docs silent; source shows `git init`, then `git fetch --depth 1 origin <ref|HEAD>`, then `git checkout --detach FETCH_HEAD`. The whole repo is checked out. Source shows no submodule step [src: src/cli/plugin.rs `git_checkout`].
- `--ref REF` pins a revision, and `--yes` skips the prompt [cli § Plugins].

```bash
herdr plugin install <owner>/<repo>[/subdir...] [--ref REF] [--yes]
herdr plugin update [<plugin_id|owner/repo[/subdir...]>...] [--yes]
```

### Example plugins

- Docs: the cookbook repo `ogulcancelik/herdr-plugin-examples` contains `agent-telegram-notify`, `github-link-preview` and `dev-layout-bootstrap`. These are "examples to copy; Herdr does not maintain them as official plugins" [plugins § Install and link].
- The CLI reference also names `…/worktree-bootstrap` [cli § Plugins]. It is not present in the examples repo at commit `18709cd` [examples].
- The examples repo has 4 plugins [examples]:

| Plugin | Language / platforms | What it shows |
| --- | --- | --- |
| `agent-telegram-notify` | Node, all 3 OSes | `[[events]] on = "pane.agent_status_changed"`. Reads `event.data.agent_status` from `HERDR_PLUGIN_EVENT_JSON`. Loads `$HERDR_PLUGIN_CONFIG_DIR/.env`, then `<plugin root>/.env`, and ships `.env.example`. |
| `dev-layout-bootstrap` | Lua, linux/macos | `herdr pane split … --no-focus`, then `herdr pane run <pane> "<cmd>"` |
| `github-link-preview` | Bash, linux/macos | Link handler → action → `plugin pane open --placement split --env GITHUB_URL=…` |
| `rust-release-check` | Rust, linux/macos | `[[build]] command = ["cargo", "build", "--release"]`; action `command = ["./target/release/rust-release-check"]`; README: "Requires `cargo` and `git` on the user's machine." |

## 3. `[[events]]`: event names, hook inputs, timeouts

### Docs coverage

- "Event hooks run for enabled installed plugins when Herdr emits a matching event name such as `worktree.created`." [socket § Plugin APIs]
- `on` values are checked against known names at link time. An unknown name still links, but the link and list responses carry a warning such as `"unknown event 'worktree.craeted'"` [socket § Plugin APIs].
- The docs list subscription events but not the set that can trigger hooks. Subscription events [socket § Event subscriptions]:
  - Workspace: `workspace.created`, `.updated`, `.metadata_updated`, `.renamed`, `.moved`, `.reordered`, `.closed`, `.focused`
  - Tab: `tab.created`, `.closed`, `.focused`, `.renamed`, `.moved`
  - Pane: `pane.created`, `.updated`, `.closed`, `.focused`, `.moved`, `.exited`, `.agent_detected`, `.output_matched`, `.agent_status_changed`, `.scroll_changed`
  - Layout: `layout.updated`
  - Worktree: `worktree.created`, `.opened`, `.removed`
- `workspace.metadata_updated` "is available to API subscribers but does not invoke plugin event hooks" [socket § Agent state reporting].

### Events that can trigger hooks

Docs silent; source has an explicit list [src: src/api/schema/events.rs `PLUGIN_HOOK_EVENT_KINDS`]. 22 names:

```text
workspace.created  workspace.updated  workspace.closed  workspace.renamed
workspace.moved    workspace.reordered  workspace.focused
worktree.created   worktree.opened    worktree.removed
tab.created  tab.closed  tab.renamed  tab.moved  tab.focused
pane.created  pane.closed  pane.focused  pane.moved  pane.exited
pane.agent_detected  pane.agent_status_changed
```

- Excluded from hooks: `workspace.metadata_updated`, `pane.updated`, `layout.updated` and `pane.output_changed`. `pane.output_matched` and `pane.scroll_changed` are subscription-only.
- Source comment: "intentionally narrower than `EventKind` until high-volume output-change hook semantics are implemented."

### Events relevant to githerd

- **Agent state change:** `pane.agent_status_changed`.
  - Payload: `pane_id`, `workspace_id`, `agent_status` (`idle|working|blocked|done|unknown`), and optionally `agent`, `title`, `display_agent`, `state_labels` [src: events.rs `EventData`; schema].
  - Meanings: `done` = "finished and you have not looked at it yet"; `idle` = finished or waiting and already seen; `blocked` = needs input [concepts § Agent].
  - "`idle` and `done` both mean ready for input" [cli § Agents].
- **Agent appears or exits:** `pane.agent_detected`. Payload: `pane_id`, `workspace_id`, `agent?`, `released` (omitted when false), `final_status?` [src: events.rs].
- **Focus or switch:** `workspace.focused` (`workspace_id`), `tab.focused` (`tab_id`, `workspace_id`) and `pane.focused` (`pane_id`, `workspace_id`) [src: events.rs]. `pane.focused` "also reports manual pane selection changes from any client … Selecting an already-selected pane does not emit it again" [socket § Event subscriptions].
- **Pane exit:** `pane.exited` and `pane.closed` (`pane_id`, `workspace_id`) [src: events.rs].
- **Workspace created:** `workspace.created` (`workspace`: WorkspaceInfo). It includes optional `workspace.worktree` provenance [socket § Event subscriptions].

### What a hook receives

- **Environment** [plugins § Commands and environment]:
  - The standard runtime set: `HERDR_SOCKET_PATH`, `HERDR_BIN_PATH`, `HERDR_ENV=1`, `HERDR_PLUGIN_ID`, `HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR`, `HERDR_PLUGIN_STATE_DIR` and `HERDR_PLUGIN_CONTEXT_JSON`.
  - Any available `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID` and `HERDR_PANE_ID`.
  - `HERDR_PLUGIN_EVENT` and `HERDR_PLUGIN_EVENT_JSON`.
  - Working directory is the plugin directory.
- **`HERDR_PLUGIN_EVENT`** holds the dot name, for example `pane.agent_status_changed` [src: runtime.rs].
- **`HERDR_PLUGIN_EVENT_JSON`** is the serialized event envelope (docs silent) [src: events.rs; schema `EventEnvelope`]. Note the snake_case names, not dot names:

```json
{"event":"pane_agent_status_changed","data":{"type":"pane_agent_status_changed","pane_id":"w1:p1","workspace_id":"w1","agent_status":"done","agent":"claude"}}
```

- **Context for events** (docs silent) [src: src/app/api/plugins/context.rs `plugin_context_for_event`]:
  - Pane events: the context is built for the event's pane, so `HERDR_PANE_ID` is the event's pane.
  - Workspace events: the context uses that workspace's active tab and focused pane.
  - Tab events: the context uses that tab.
  - `invocation_source = "api"`, and `correlation_id` is the event name.

### Timeouts and concurrency

- The docs give none for actions or events. Startup hooks are described as "Herdr starts them asynchronously" [plugins § Startup hooks].
- Docs silent; source shows [src: runtime.rs]:
  - Each matching hook is spawned as its own process on a background thread. There is no timeout and no dedup or serialization.
  - There is a global cap of **32 plugin commands in flight** (`MAX_PLUGIN_COMMANDS_IN_FLIGHT`), shared by all plugins and by actions, events and startup. Past the cap, a command is not started and is logged as failed with `maximum concurrent plugin commands reached (32)`.
  - stdout and stderr are each capped at 64 KiB.
  - Only the last 200 command logs are kept in memory (`herdr plugin log list`).
- "Plugins must stop detached background processes before their parent command or pane exits" [plugins § Install and link].
- The socket offers a separate long-lived stream through `events.subscribe` [socket § Event subscriptions]:
  - Event history is not durable.
  - A slow reader gets `events_lost` and its connection is closed.
  - `events.wait` handles one-shot waits.

## 4. `HERDR_PLUGIN_CONTEXT_JSON` schema

### Docs

- The context "can include workspace, tab, focused pane, worktree, agent, selected text, clicked URL, and link handler fields when they are available for that invocation" [plugins § Commands and environment].
- Link-click actions get `invocation_source = "link_click"`, `clicked_url` and `link_handler_id` [plugins § Link handlers].
- For `plugin.action.invoke`, "Missing context fields are filled from the active workspace, tab, focused pane, worktree provenance, and request id" [socket § Plugin APIs].
- In a popup, "the underlying tiled pane remains available through `HERDR_PLUGIN_CONTEXT_JSON`" [plugins § Panes].

### Fields

All fields are optional [schema; src: src/api/schema/plugins.rs `PluginInvocationContext`]:

```text
workspace_id, workspace_label, workspace_cwd,
worktree { repo_key, repo_name, repo_root, checkout_path, is_linked_worktree },
tab_id, tab_label,
focused_pane_id, focused_pane_cwd, focused_pane_agent, focused_pane_status,
selected_text, invocation_source, correlation_id, clicked_url, link_handler_id
```

### Per invocation kind

Docs silent; source shows [src: context.rs, runtime.rs, src/app/api/plugins/mod.rs, panes.rs, src/cli/plugin.rs]:

| Kind | Context target | `invocation_source` | `correlation_id` |
| --- | --- | --- | --- |
| Action via keybinding | active workspace, tab and focused pane; plus `selected_text` | `keybinding` | `keybinding` |
| Action via `herdr plugin action invoke` | active workspace | `cli` | request id |
| Action via socket `plugin.action.invoke` | the caller's `context` fields win; the rest are filled in | caller's value | caller's value or request id |
| Action via link click | active workspace, plus `clicked_url` and `link_handler_id` | `link_click` | — |
| Event hook | the event's pane, tab or workspace | `api` | event name |
| Startup hook | the workspace active at startup | `startup` | `plugin.startup` |
| Pane (popup or overlay) | active workspace | `api` | `plugin-pane` |
| Pane (split or zoomed) | the target pane | `api` | `plugin-pane` |
| Pane (tab) | the target workspace | `api` | `plugin-pane` |

### Does it include cwd?

- **Yes**, through `workspace_cwd` and `focused_pane_cwd`.
- Docs silent; source shows `workspace_cwd` is the focused pane's cwd when known. Otherwise it falls back to the workspace "identity cwd": the cwd of the first tab's root pane, else the workspace's initial cwd. Whenever the pane cwd is known, `workspace_cwd` equals `focused_pane_cwd` [src: context.rs `plugin_context_from_parts`; src/workspace.rs `resolved_identity_cwd_from`].
- Docs silent; source shows `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID` and `HERDR_PANE_ID` are copied from the context's `workspace_id`, `tab_id` and `focused_pane_id` [src: runtime.rs].
- Link handlers also get the `HERDR_PLUGIN_CLICKED_URL` and `HERDR_PLUGIN_LINK_HANDLER_ID` environment variables [plugins § Link handlers].

## 5. Workspace cwd, the `branch` and `git_status` tokens, listing commands

### Does a workspace have a root dir?

- A workspace is "the top-level project container. Use one workspace per repo, task, or investigation." [concepts § Workspace]
- It is created with `herdr workspace create [--cwd PATH] [--label TEXT] [--env KEY=VALUE] [--focus] [--no-focus]` [cli § Workspaces].
- "Without `--cwd`, new terminals follow the configured `terminal.new_cwd` policy, which follows the source pane or workspace by default." [cli § Tabs]
- `new_cwd` accepts `follow`, `home`, `current` or a fixed path [config § Terminal defaults].
- The pane `cwd` field is "the pane/workspace cwd used for labels and follow-cwd behavior". `foreground_cwd` is the cwd of the foreground process [cli § Panes; socket § Agent state reporting].
- Restore saves the cwd of each pane [session § Snapshot restore].
- The docs never describe a single fixed root dir for a workspace.
- WorkspaceInfo JSON has **no cwd field**. Its fields are `workspace_id`, `number`, `label`, `focused`, `pane_count`, `tab_count`, `active_tab_id`, `agent_status`, `tokens` and `worktree?` [schema; src: src/api/schema/workspaces.rs].
- `worktree` provenance (`repo_key`, `repo_name`, `repo_root`, `checkout_path`, `is_linked_worktree`) appears only "when a workspace belongs to a Herdr worktree group" [socket § Raw methods (worktree paragraph)].

### `branch` and `git_status`

- Docs: "`branch` — Git branch when available." "`git_status` — Git ahead and behind counts when nonzero." [config § Sidebar row layouts]
- The Windows table lists "Git/worktree detection from known cwd" as supported [win § Supported on Windows].
- Which repo a workspace maps to is not documented. Docs silent; source shows:
  - The repo is discovered from the workspace identity cwd: the live cwd of the **first tab's root pane**, falling back to the workspace's initial cwd. The automatic workspace label comes from the same cwd [src: src/workspace.rs `resolved_identity_cwd_from`; src/app/git_refresh.rs].
  - Git data is refreshed only when the active layout uses these tokens [src: git_refresh.rs].
- Exact `git_status` format (docs silent; source shows) [src: src/ui/sidebar.rs; src/client/shell/sidebar.rs]:
  - `↑N` (green) when ahead and `↓M` (red) when behind, joined by one space, each shown only when nonzero.
  - Hidden entirely when both are 0.
  - Suppressed, together with `branch`, on indented worktree child rows.
- A styled `fg` replaces both colours [config § Sidebar row layouts].
- "`state_icon` and composite `git_status` accept fixed styles only, not rules." [config § Sidebar row layouts]
- No built-in token for a dirty or uncommitted count is documented.

### Listing commands

"Most commands print JSON responses for deterministic automation in scripts." [cli § CLI reference (intro)]

```bash
herdr workspace list                      # WorkspaceInfo[]: no cwd
herdr workspace get <workspace_id>
herdr pane list [--workspace <workspace_id>]   # PaneInfo incl. cwd, foreground_cwd
herdr pane get <pane_id>
herdr pane process-info [--pane ID|--current]  # foreground processes with cwd
herdr worktree list [--workspace ID | --cwd PATH] [--trust-repository]
herdr api snapshot                        # session.snapshot: workspaces, tabs, panes, layouts, agents
```

- Sources: [cli § Workspaces, § Panes, § Worktrees]; [socket § Raw methods].
- `pane.process_info` returns "foreground processes with pid, name, argv/cmdline, and cwd when the platform exposes them" [socket § Raw methods].
- A timeout or server error prints JSON on stderr and exits 1. CLI usage errors exit 2 [cli § Output waits].

## 6. `report-metadata` semantics

### Syntax

```bash
herdr workspace report-metadata <workspace_id> --source ID [--token NAME=VALUE] [--clear-token NAME] [--seq N] [--ttl-ms N]

herdr pane report-metadata <pane_id> \
  --source ID \
  [--agent LABEL] \
  [--applies-to-source ID] \
  [--title TEXT|--clear-title] \
  [--display-agent TEXT|--clear-display-agent] \
  [--state-label STATUS=TEXT] \
  [--clear-state-labels] \
  [--token NAME=VALUE] \
  [--clear-token NAME] \
  [--seq N] \
  [--ttl-ms N]
```

Sources: [cli § Workspaces, § Panes].

### Flags and limits

- **`--source`** is required. It must be ≤80 characters of ASCII letters, digits, colon, dot, underscore and hyphen [cli § Panes].
- **`--token` / `--clear-token`:**
  - "`--token` patches one named display value; `--clear-token` removes one. Unmentioned tokens remain unchanged." [cli § Panes]
  - "Empty normalized token values clear that key." [cli § Panes]
  - "The latest accepted update wins." [socket § Agent state reporting]
- **Value type:** string only. Over the socket, "A string sets a key, JSON `null` clears it, and omitted keys remain unchanged." [socket § Agent state reporting]
- **Normalization:** Herdr "trims surrounding whitespace, removes control characters, and caps … token values at 80 characters" [cli § Panes].
- **Token names:** "1–32 ASCII letters, digits, underscores, or hyphens" [socket § Agent state reporting].
- **Key counts:** "A report may mention at most 16 token keys, and a pane or workspace may retain at most 32 keys." [socket § Agent state reporting]
- **`--ttl-ms`:**
  - Range is `1`–`86400000`. It "makes metadata expire automatically". "TTL applies independently to the token keys updated by that call." [cli § Panes]
  - On expiry the key goes away, and unreported tokens disappear from the sidebar [config § Sidebar row layouts].
  - "Changes and TTL expiry emit `workspace.metadata_updated`", for subscribers only and not for hooks [socket § Agent state reporting].
  - Without a TTL, metadata stays "until replaced, cleared, or the pane closes" (or the workspace closes) [cli § Panes; socket § Agent state reporting].
  - Docs silent; source shows that an update to a key without a TTL cancels that key's previous expiry [src: src/metadata_tokens.rs test `update_without_ttl_cancels_previous_expiry`].
- **`--seq`:**
  - "`--seq` lets Herdr ignore stale reports from the same `--source`; stale reports are accepted by the API but ignored by pane state." A report counts as stale when its sequence is ≤ the last accepted one [cli § Panes; socket § Agent state reporting].
  - "A pane or workspace accepts sequenced token reports from at most 32 distinct sources during its lifetime; clearing or expiry does not release those source slots." [cli § Panes]

### Persistence and display

- **Restart:** "Token metadata is not restored after a server restart." [socket § Agent state reporting]
- **Live handoff:** it preserves "agent identity and durable metadata" [session § Live handoff]. Whether tokens count as durable metadata is not documented.
- **Where tokens show:**
  - "Pane tokens are available to Agent sidebar rows as `$name`; workspace tokens are available to Space rows." [cli § Panes]
  - `workspace get/list` expose a `tokens` map [socket § Agent state reporting].
  - `--agent` and `--applies-to-source` "do not guard token patches" [cli § Panes].
- **Source naming:**
  - Doc examples use `user:jj`, `my-agent-hook` and `custom:docs`. No metadata naming convention is documented.
  - The `plugin:<HERDR_PLUGIN_ID>` convention is documented only for `agent.view.set` [socket § Agent view queries].

## 7. Opening panes; `[[panes]]` fields; actions

### `[[panes]]` fields

- Documented fields: `id`, `title`, `placement` (default `overlay`), `width`, `height` (popup only), `command` (argv) and `platforms` [plugins § Manifest, § Panes].
- Docs silent; source shows one more optional field, `description`. The manifest pane has **no `cwd` or `env` fields** [schema `PluginManifestPane`; src: src/api/schema/plugins.rs].

```toml
[[panes]]
id = "picker"
title = "Picker"
platforms = ["linux", "macos"]
placement = "popup"
width = "80%"
height = 20
command = ["sh", "picker.sh"]
```

Source: [plugins § Panes].

### Width and height units

- Numbers are outer terminal cells, including the border. Strings like `"80%"` are a percentage of the terminal area.
- Omitting them gives the default half-size popup. Values below the popup minimum are clamped [plugins § Panes; config § Custom command keybindings].
- Docs silent; source shows `width`/`height` on a non-popup placement returns `invalid_params` [src: src/app/api/plugins/mod.rs].

### Opening a declared pane

```bash
herdr plugin pane open --plugin ID --entrypoint ID [--placement overlay|popup|split|tab|zoomed] [--width SIZE] [--height SIZE] [--workspace ID] [--target-pane PANE] [--direction right|down] [--cwd PATH] [--env KEY=VALUE] [--focus|--no-focus]
herdr plugin pane focus <pane_id>
herdr plugin pane close <pane_id>
```

```json
{"id":"req_plugin_pane","method":"plugin.pane.open","params":{"plugin_id":"example.board","entrypoint":"board","placement":"zoomed","target_pane_id":"w1:p1","env":{"HERDR_ROLE":"board"},"focus":true}}
```

Sources: [cli § Plugins]; [socket § Plugin APIs].

- **Targeting:**
  - "Overlay and popup placements use the active tiled pane as launch context … Split and zoomed panes target an existing pane; tab panes can target a workspace." [socket § Plugin APIs]
  - Docs silent; source shows: overlay and popup reject `--workspace`, `--target-pane` and `--direction`; split and zoomed reject `--workspace`; tab rejects `--target-pane` and `--direction`. A split defaults to `right` at ratio 0.5. The CLI defaults to focus=true; the socket defaults to `focus: false` [src: mod.rs, panes.rs, src/cli/plugin.rs].
- **cwd:**
  - The `--cwd` flag exists, but its default is not documented.
  - Docs silent; source shows the pane cwd is `--cwd` when given, else the **plugin root**.
  - When `command[0]` is a relative path with a separator (for example `./bin/x`), it is resolved against the plugin root even when `--cwd` is set. Other argv elements are not rewritten [src: panes.rs `plugin_pane_cwd`; mod.rs uses `program_for_cwd(..., plugin_root)`].
- **env:** "`--env KEY=VALUE` can be repeated … It applies to the newly launched process only." Herdr-managed variables stay authoritative [cli § Plugins].
- **Passing args to a declared pane:**
  - There is no args parameter; the argv is fixed in the manifest. "runtime argv pane creation [is] not part of v1" [socket § Plugin APIs].
  - The example plugin passes data with `--env`: `plugin pane open --plugin examples.github-link-preview --entrypoint preview --placement split --direction right --env "GITHUB_URL=$url" --focus` [examples: github-link-preview/open.sh].
- **From an action:** the action command calls the CLI. "The entire Herdr CLI is the plugin API." [plugins § Plugins (intro)]
- **Responses:**
  - A popup returns `ok` [plugins § Panes].
  - Docs silent; source shows non-popup placements return `plugin_pane { plugin_id, entrypoint, pane: PaneInfo }` [src: panes.rs].
- **After opening:**
  - "Split, tab, zoomed, and overlay plugin panes are normal Herdr panes after they open." [plugins § Panes]
  - What happens when their command exits is not documented. Docs silent; source shows a dying pane process emits `pane.exited` and the pane is removed from the layout [src: src/app/api.rs]. The github-link-preview example waits for Enter before exiting.

### Arbitrary command in a split or tab (non-plugin-pane)

```bash
herdr pane split [<pane_id>|--pane ID|--current] --direction right|down [--ratio FLOAT] [--cwd PATH] [--env KEY=VALUE] [--right-click herdr|pane] [--focus] [--no-focus]
herdr tab create [--workspace <workspace_id>] [--cwd PATH] [--label TEXT] [--env KEY=VALUE] [--focus] [--no-focus]
herdr pane run <pane_id> <command>
```

Source: [cli § Panes, § Tabs].

- Neither `pane split` nor `tab create` takes a command.
- `pane run` "submits text plus Enter atomically" into the pane's shell [cli § Panes].
- Response fields:
  - Split: `.result.pane.pane_id`
  - Tab: `.result.tab.tab_id` and `.result.root_pane.pane_id` [cli § Panes, § Tabs]
- Socket `layout.apply` "creates a fresh tab from a declarative tree", with per-pane `cwd`, argv `command` and `env` [socket § Raw methods].
- No CLI wrapper for `layout.apply` is documented. Docs silent; source shows `herdr api` has only `schema` and `snapshot` subcommands [src: src/cli/api.rs].

### Running an action directly

- `herdr plugin action invoke <action_id> [--plugin ID]` starts the action and prints the started command log record as JSON. Use the qualified id `plugin.id.action` when needed [cli § Plugins].
- The CLI has no flags for args or env. Docs silent; source shows the only option is `--plugin` [src: src/cli/plugin.rs].
- Socket `plugin.action.invoke` accepts a `context` object with the fixed fields from §4 [socket § Plugin APIs].
- Keybinding form: `[[keys.command]] type = "plugin_action"` [config § Custom command keybindings].
- Action `contexts` values in the schema are `global|workspace|tab|pane|selection`. Their effect is not documented [src: schema/plugins.rs `PluginActionContext`].

## 8. Popup panes

- **Keyboard input:** yes. A popup "receives all terminal input, including Escape, and closes when the command exits or a `popup.close` request is sent" [plugins § Panes].
- **Other behaviour** [plugins § Panes]:
  - It is "a singleton session resource rather than a Herdr pane".
  - It "has no pane ID, does not change plugin focus context, emits no pane lifecycle events, and does not participate in pane, layout, persistence, or agent APIs."
  - It gets no `HERDR_PANE_ID`.
  - "Opening a popup returns `ui_busy` while Settings, Copy mode, or another Herdr modal is active."
- Docs silent; source shows opening a popup while one is open also returns `ui_busy` ("a popup pane is already open") [src: mod.rs].
- **Closing:**
  - `popup.close` closes the active popup and returns `popup_not_open` when there is none [socket § Plugin APIs].
  - No CLI wrapper for `popup.close` is documented.
- **Mouse input:** not documented.

## 9. Plugin user config

- Config files [plugins § Commands and environment]:
  - "Put user-editable config such as `.env` files under `HERDR_PLUGIN_CONFIG_DIR`, and put local runtime state under `HERDR_PLUGIN_STATE_DIR`."
  - Herdr "creates those directories and seeds `HERDR_PLUGIN_CONFIG_DIR` from the legacy plugin config locations when present, but it does not validate, sync, or delete their contents. The plugin owns the file format and lifecycle."
  - "Do not store user credentials or durable state" in `HERDR_PLUGIN_ROOT`.
- `herdr plugin config-dir <plugin_id>` prints the directory, creating it if needed, for "setup docs and shell scripts" [cli § Plugins].
- "There is no Herdr-managed plugin storage API in v1." [plugins § Storage]
- The config and state directories are "path discovery only" [socket § Plugin APIs].
- Config and state survive update and uninstall ("Config and state are preserved") [plugins § Install and link].
- **A per-plugin section in `config.toml`: not documented.** The config reference has no plugin keys (there is no `plugin` key in `config-reference.json`).
- Plugin-related config that does exist:
  - `[[keys.command]] type = "plugin_action"` [config § Custom command keybindings]
  - `ui.mouse_capture`, for link handlers [plugins § Link handlers]
  - Sidebar `$name` tokens [config § Sidebar row layouts]
- Docs silent; source shows the directory layout: the config dir is `<herdr config dir>/plugins/config/<plugin-id>` and the state dir is `<herdr state dir>/plugins/<plugin-id>` [src: src/plugin_paths.rs].
- Example convention: `.env` in the config dir, with `.env.example` shipped in the repo [examples: agent-telegram-notify].
- **Remote machines:**
  - "Custom commands and plugins advertised by the selected server still run there. Herdr does not copy local command plugins, configuration, executables, or secrets onto SSH hosts." [machines § Settings and automation]
  - Plugin installation is not forwarded by `--machine` [cli § Saved SSH machines].

## 10. Marketplace

- Listing requirements [market § Browse plugins, § Get your plugin listed, § What a listing shows]:
  - A **public** GitHub repo with the topic `herdr-plugin`.
  - The **default branch** holds at least one `herdr-plugin.toml` "whose required metadata can be parsed". Required means `id`, `name`, `version` and `min_herdr_version` [plugins § Manifest].
  - "Forks, archived repositories, repositories without a valid plugin manifest, and malformed manifest metadata are excluded."
- The index refreshes every 30 minutes and "rescans repositories when their default-branch head changes" [market § Get your plugin listed].
- Discovery is "automatic and unreviewed" [market § Browse plugins].
- **Subdirectories are allowed:**
  - "Manifests may be at the root or in subdirectories. The marketplace uses one card per repository and lists each valid manifest as a separately installable plugin." [market § Get your plugin listed]
  - Users install with `herdr plugin install owner/repo[/subdir...]` [market § Install a plugin].
- What the index records: "the manifest path, `id`, `name`, `version`, `platforms`, and `min_herdr_version` together with the exact default-branch commit" [market § What a listing shows].
- Docs silent; source shows install checks out the whole repo (shallow) and sets the plugin root to the subdir. Build and runtime commands run in that subdir [src: src/cli/plugin.rs `manifest_root`; runtime.rs].
- Plugin ids may use "ASCII letters, digits, dot, colon, underscore, and hyphen" [plugins § Manifest].
- Install and link fail if `min_herdr_version` is newer than the running binary [plugins § Manifest].

## 11. Sidebar rules

- **Token values are strings.** Over the socket, a string sets a key [socket § Agent state reporting].
- **`gt` / `lt`:**
  - "`gt` and `lt` take finite numeric thresholds and compare strictly greater or less, not equal. Values must parse completely as finite numbers; decimal and exponent forms work, but whitespace, units, `NaN`, and infinity do not match." [config § Sidebar row layouts]
  - Example from the docs: "`$load` reporting `\"90\"` selects red … `\"60\"` selects yellow, and `\"90%\"` keeps white." [config § Sidebar row layouts]
  - So `"2 dirty"` **does not match** `gt`/`lt`: there is no leading-number parse. The rule is skipped and the next rule, or the defaults, apply.
- **General rule behaviour** [config § Sidebar row layouts]:
  - Up to 16 ordered `rules` per token, each with exactly one condition: `equals`, `contains`, `starts_with`, `gt` or `lt`.
  - The first match wins. Matching "uses the full token value before display truncation".
  - String conditions are case-sensitive unless `ignore_case = true`. `ignore_case` is not accepted on numeric rules.
  - `hide = true` removes the token and its separator.
  - Regex, fuzzy matching and scripts are not supported.
  - A layout has at most 16 rows, with at most 16 tokens per row.

```toml
[{ token = "$load", fg = "#fff", rules = [{ gt = 80, fg = "#f55", bold = true }, { gt = 50, fg = "#fc0" }] }]
```

Source: [config § Sidebar row layouts].

## 12. `[[link_handlers]]` and `[[startup]]`

- **`[[link_handlers]]`:**
  - It routes Ctrl-clicks on terminal URLs that match a Rust-regex `pattern` to an `action` of the same plugin, instead of opening a browser.
  - It works only while `ui.mouse_capture` is enabled.
  - The action gets `invocation_source = "link_click"`, `clicked_url` and `link_handler_id`. Handlers are checked in manifest order [plugins § Link handlers].
- **`[[startup]]`:**
  - These are one-shot commands, run once for each enabled plugin after the session is restored and the API socket is ready.
  - They run again on live-handoff takeover, but not on client attach, config reload, link or enable.
  - They are started asynchronously, and a failure does not stop the server.
  - They get `HERDR_PLUGIN_EVENT=startup`. They are "one-shot initialization commands rather than supervised daemons" [plugins § Startup hooks].

## 13. Roadmap and v1 limitations

- **2026 roadmap: not documented.**
  - Nothing found in the v0.9.5 docs, in the `llms.txt` / `llms-full.txt` bundles, on herdr.dev/tui or in the herdr.dev blog index (checked 2026-11-19).
  - Nothing found in the repo either: no roadmap file, and no roadmap mentions outside vendored code.
- **v1 exclusions:**
  - "Runtime action registration and native non-terminal plugin UI are not part of plugin v1." [plugins § Plugins (intro)]
  - "runtime action registration and runtime argv pane creation are not part of v1" [socket § Plugin APIs]
  - "Native non-terminal plugin panes are outside plugin v1." [cli § Plugins]
  - There is no plugin storage API [plugins § Storage].
- **Timers / scheduled plugin runs: not documented.**
  - Startup hooks are not supervised daemons [plugins § Startup hooks].
  - Background processes must stop before their parent command or pane exits. "processes surviving server exit are not protected from cleanup" [plugins § Install and link].
- **Related periodic mechanism (config, not a plugin feature):**
  - `ui.tab_bar_right` entries `{ type = "command", command = "…", interval_seconds = 5, timeout_seconds = 2 }` run on the server on an interval of 1–31,536,000 s, with a timeout of 1–3,600 s, and without overlapping runs.
  - The tab bar shows the last line of a successful run's output.
  - The command gets the same context as custom commands (`HERDR_ACTIVE_PANE_CWD` and so on) [config § UI and sidebar; § Custom command keybindings].
- **Event hooks:**
  - The set that can trigger hooks deliberately excludes high-volume events (see §3) [src: events.rs].
  - `workspace.metadata_updated` never triggers hooks [socket § Agent state reporting].
- **Graphics:** the only image path is the Kitty graphics protocol in pane output. "The external socket pane-overlay API has been removed; there is no replacement socket image API." [config § Kitty graphics]
