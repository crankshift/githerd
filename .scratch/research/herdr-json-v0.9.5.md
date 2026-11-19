# Herdr v0.9.5: CLI/JSON facts for the githerd implementation

Facts only. These extend `.scratch/research/herdr-v0.9.5.md` and don't repeat it.

Citation keys:
- `src: path symbol` is the Herdr source at tag v0.9.5 (commit `2ecf19a`).
- `schema: path` is the output of `herdr api schema --json`. It is byte-identical to `docs/next/api/herdr-api.schema.json`.
- `docs: page § heading` is the v0.9.5 docs.
- `help:` is `herdr <cmd> --help` from the local v0.9.5 binary.

All JSON below is trimmed. Paths and ids are placeholders.

---

## 0. CLI output conventions (apply to every command below)

- **Success with a result:** one line of JSON on **stdout**, exit 0. Shape: `{"id":"<cli id>","result":{"type":"<snake_case>",…}}` [src: src/cli.rs `print_response`].
- **"Ok-only" commands** (`pane run`, `workspace report-metadata`, `pane report-metadata`): they print **nothing** on success and exit 0 [src: src/cli.rs `send_ok_request`].
- **Server-side error:** `{"id":"…","error":{"code":"…","message":"…"}}` on **stderr**, exit **1** [src: cli.rs `print_response` / `send_ok_request`; schema: `error_response`].
- **Usage error** (missing value, unknown option, bad direction or ratio, missing `--source`): plain text on stderr, exit **2** [src: src/cli/{workspace,pane,plugin}.rs parsers].
- **Non-integer `--seq` / `--ttl-ms`:**
  - The error escapes `main` as an `io::Error`.
  - stderr gets `Error: Custom { kind: Other, error: "invalid value for --seq: X" }` and the exit code is **1**, not 2 [src: src/cli.rs `parse_u64_flag`; src/main.rs `finish_cli` / `main`].
- **No server running:** a `server_not_running` error JSON on stderr, exit 1. Each socket command first pings the server and checks the protocol; a mismatch also gives an error JSON on stderr and exit 1 [src: main.rs `finish_cli`; cli.rs `ensure_server_protocol_compatible`].
- **Help:** `herdr <group> <sub> --help` prints clap help to stdout and exits 0 without contacting the server [src: src/cli/spec.rs `write_requested_help`].
- **IDs:**
  - Workspace ids look like `w<N>`, tab ids `<ws>:t<N>` and pane ids `<ws>:p<N>`. N uses the alphabet `123456789ABCDEFGHJKMNPQRSTVWXYZ0` [src: src/workspace.rs `generate_workspace_id`, `public_pane_id_for_number`, `public_tab_id_for_number`].
  - Treat ids as opaque.
  - Workspace lookup also accepts a bare 1-based **position** (`"3"`) or `w_3` [src: src/app/ids.rs `parse_workspace_id`].

## 1. `herdr api snapshot`

### Envelope and top level

- The result is `{"type":"session_snapshot","snapshot":{…}}`. The request id is `cli:api:snapshot` [src: src/cli/api.rs `api_snapshot`].
- `snapshot` has these fields:
  - `version`, `protocol` (22)
  - `focused_workspace_id?`, `focused_tab_id?`, `focused_pane_id?`
  - `workspaces[]`, `tabs[]`, `panes[]`, `layouts[]`, `agents[]`

  [schema: `success_response/$defs/SessionSnapshot`; src: src/api/schema/session.rs].

### Records

- **WorkspaceInfo** [src: src/api/schema/workspaces.rs; creation.rs `workspace_info`]:
  - Fields: `workspace_id`, `number`, `label`, `focused`, `pane_count`, `tab_count`, `active_tab_id`, `agent_status`, `tokens?` (omitted when empty) and `worktree?`.
  - `number` is the sidebar **position** + 1.
  - There is no cwd field.
- **TabInfo** [src: src/api/schema/tabs.rs; creation.rs `tab_info`]:
  - Fields: `tab_id`, `workspace_id`, `number`, `label`, `focused`, `pane_count`, `agent_status`.
  - `number` is the **stable id number**, not the position. It does not change when tabs are moved.
- **PaneInfo** [src: src/api/schema/panes.rs `PaneInfo`]:
  - Fields: `pane_id`, `terminal_id`, `workspace_id`, `tab_id`, `focused`, `cwd?`, `foreground_cwd?`, `restore_error?`, `label?`, `agent?`, `title?`, `terminal_title?`, `terminal_title_stripped?`, `display_agent?`, `agent_status`, `state_labels?`, `tokens?`, `agent_session?`, `scroll?`, `revision`.
- **PaneLayoutSnapshot** (one per tab):
  - Fields: `workspace_id`, `tab_id`, `zoomed`, `area{x,y,width,height}`, `focused_pane_id`, `panes[{pane_id,focused,rect}]`, `splits[{id,direction,ratio,rect}]`.
  - These are flat rectangles in cells. There is **no tree and no root-pane field** [src: src/app/api/panes.rs `pane_layout_snapshot`].

### Array order

Source: src/app/api/session.rs `session_snapshot_with_pane_info`.

- Workspaces come in sidebar order.
- For each workspace, tabs come in tab-bar order (`ws.tabs[0..]`). Each tab pushes its TabInfo, then its layout, then its panes.
- Panes come in layout-tree DFS order: first child before second [src: src/layout.rs `pane_ids` / `collect_ids`].

### First tab and its root pane

- **First tab** of workspace W: the first element of `tabs` whose `workspace_id == W`. `tabs` order is the `ws.tabs` order, and `move_tab` reorders that vector [src: src/workspace.rs `move_tab`].
- **Root pane** as Herdr defines it [src: src/workspace/tab.rs `root_pane`, `promoted_root_if_needed`]:
  - It is `Tab.root_pane`: the pane the tab was created with.
  - If that pane closes, the first remaining pane in DFS order replaces it.
- **Workspace identity cwd**, which Herdr uses for the label and git: the `cwd` of the first tab's root pane, else the workspace's initial cwd [src: src/workspace.rs `resolved_identity_cwd_from`].
- **`root_pane` is not in the snapshot.** The schema has `root_pane` only in the results `workspace_created`, `tab_created`, `worktree_created` and `worktree_opened` [schema: `success_response/$defs/ResponseResult`].
- The nearest snapshot equivalent is the first `panes[]` entry whose `tab_id` is the first tab's id, which is the first DFS leaf. It differs from Herdr's `root_pane` only after a swap or move puts another pane before the root pane in the tree. `root_pane` follows the pane, not the position.

### `cwd` vs `foreground_cwd`

- **`cwd`** is, in order of preference:
  - the cwd the shell reported (OSC 7);
  - else the OS cwd of the pane's child (shell) process;
  - else the pane's launch cwd.

  It is the field Herdr itself uses ("the pane/workspace cwd used for labels and follow-cwd behavior") [src: src/workspace/tab.rs `cwd_for_pane`; src/pane.rs `cwd`; docs: cli-reference § Panes].
- **`foreground_cwd`** is the cwd of the foreground process-group leader of the PTY, for example an agent that ran `cd`. It is Unix-only and omitted when unknown [src: tab.rs `foreground_cwd_for_pane`; pane.rs `foreground_cwd`; docs: socket-api (foreground_cwd paragraph)].
- Both fields are optional.

### Example

```json
{"id":"cli:api:snapshot","result":{"type":"session_snapshot","snapshot":{
 "version":"0.9.5","protocol":22,"focused_workspace_id":"w1","focused_tab_id":"w1:t1","focused_pane_id":"w1:p1",
 "workspaces":[{"workspace_id":"w1","number":1,"label":"api","focused":true,"pane_count":2,"tab_count":1,
   "active_tab_id":"w1:t1","agent_status":"idle","tokens":{"changes":"●3"}}],
 "tabs":[{"tab_id":"w1:t1","workspace_id":"w1","number":1,"label":"…","focused":true,"pane_count":2,"agent_status":"idle"}],
 "panes":[
   {"pane_id":"w1:p1","terminal_id":"…","workspace_id":"w1","tab_id":"w1:t1","focused":true,
    "cwd":"/work/repos/api","foreground_cwd":"/work/repos/api/src","agent":"claude","agent_status":"idle","revision":42},
   {"pane_id":"w1:p2","terminal_id":"…","workspace_id":"w1","tab_id":"w1:t1","focused":false,
    "cwd":"<plugin root>","label":"<[[panes]].title>","agent_status":"unknown","revision":7}],
 "layouts":[{"workspace_id":"w1","tab_id":"w1:t1","zoomed":false,"area":{"x":0,"y":0,"width":200,"height":50},
   "focused_pane_id":"w1:p1","panes":[{"pane_id":"w1:p1","focused":true,"rect":{"x":0,"y":0,"width":150,"height":50}}],
   "splits":[{"id":"split_0_root","direction":"right","ratio":0.75,"rect":{"x":0,"y":0,"width":200,"height":50}}]}],
 "agents":[]}}}
```

## 2. Recognising a plugin pane (finding an open `rail`)

- **No plugin fields in pane data.** PaneInfo and the snapshot have no `plugin_id` or `entrypoint`. Herdr keeps an in-memory map from pane to `{plugin_id, entrypoint}` (`AppState.plugin_panes`) [src: src/app/state.rs; src/app/api/plugins/panes.rs `finish_plugin_pane_open`].
- **Where the plugin id and entrypoint do appear:** only in the responses of `plugin pane open` and `plugin pane focus`, as `result.plugin_pane.{plugin_id, entrypoint, pane}` [src: plugins/mod.rs `handle_plugin_pane_focus`; panes.rs `finish_plugin_pane_open`].
- **`plugin pane focus <pane_id>`:**
  - On a pane that is not in the map, it returns `plugin_pane_not_found` (exit 1).
  - On success it also switches to that pane's workspace and tab and focuses the pane [src: mod.rs `handle_plugin_pane_focus`; src/app/actions.rs `focus_pane_in_workspace`].
- **Snapshot hints:**
  - `label` is the manifest `[[panes]].title`, set as the manual label. The user can change it with `pane rename` [src: panes.rs `finish_plugin_pane_open` → `set_manual_label`; test in plugins/mod.rs asserts `pane.label == "Plugin Board"`].
  - `cwd` is the plugin root unless `--cwd` was given.
- **`plugin pane open` never deduplicates.** Each call opens a new pane [src: mod.rs `handle_plugin_pane_open`].
- **The map is not persisted:**
  - After a server restart, the pane comes back as a **plain shell** with its saved label and cwd. The plugin argv is not re-run, and `plugin pane focus` on it fails.
  - After live handoff the process survives, but the map is empty too. `persist/` and `server/` contain no `plugin_panes` references [src: src/persist/restore.rs (spawns a shell unless the pane was handoff-imported); src/persist/snapshot.rs `PaneSnapshot`].
  - `plugin unlink` drops that plugin's records; its panes keep running [src: mod.rs `handle_plugin_unlink`].
- **Pane tokens:**
  - Pane tokens set with `pane report-metadata <pane> --source S --token K=V` appear in the snapshot as `PaneInfo.tokens`.
  - They disappear when the pane closes and are not restored after a server restart [docs: cli-reference § Panes; socket-api § Agent state reporting].
  - The sidebar shows them only in Agent rows that reference `$K`.

## 3. Commands: flags, stdout, exit codes

### `plugin pane open`

- **Flags** [help]:
  - `--plugin <ID>`, `--entrypoint <ID>`
  - `--placement overlay|split|tab|zoomed`
  - `--workspace <ID>`, `--target-pane <PANE>`, `--direction right|down`
  - `--cwd <PATH>`, `--env <KEY=VALUE>` (repeatable)
  - `--focus`, `--no-focus`
- **Also accepted, though help omits them:** `--placement popup`, `--placement fullscreen` (same as zoomed), `--width` and `--height` [src: src/cli/plugin.rs `plugin_pane_open`, `parse_pane_placement`].
- **Defaults** [src: cli/plugin.rs; plugins/panes.rs `open_plugin_split_pane`, `current_public_pane_id`]:
  - Placement: the manifest's placement, which itself defaults to overlay.
  - **Focus: true.** This is the CLI default, unlike `pane split` and `workspace create`.
  - Target pane: the **focused pane of the active workspace**, not the caller's `HERDR_PANE_ID`.
  - Direction: `right`.
  - Split ratio: fixed at 0.5. There is no ratio flag. The new pane is the second child, so it sits right of or below the target.
- **Rejected with `invalid_params`:**
  - split or zoomed with `--workspace`;
  - `--width` or `--height` without popup [src: mod.rs `handle_plugin_pane_open`].
- **Error codes:**
  - registry and plugin: `plugin_registry_load_failed`, `invalid_plugin_id`, `plugin_not_found`, `plugin_manifest_unavailable`, `plugin_disabled`
  - entrypoint and platform: `invalid_plugin_entrypoint`, `plugin_pane_not_found`, `platform_unsupported`
  - parameters and UI: `invalid_params`, `ui_busy`
  - target: `no_active_pane`, `pane_not_found`
  - launch: `plugin_pane_open_failed`, `plugin_user_dir_create_failed`
- **Success** (exit 0). A popup returns `{"type":"ok"}` instead.

```json
{"id":"cli:plugin","result":{"type":"plugin_pane_opened","plugin_pane":{"plugin_id":"githerd","entrypoint":"rail",
 "pane":{"pane_id":"w1:p2","workspace_id":"w1","tab_id":"w1:t1","focused":true,"cwd":"<plugin root>","label":"<title>","agent_status":"unknown",…}}}}
```

### `plugin pane focus <PANE_ID>`

- Usage [help]: `herdr plugin pane focus <PANE_ID>`.
- Success: `{"id":"cli:plugin","result":{"type":"plugin_pane_focused","plugin_pane":{"plugin_id":"…","entrypoint":"…","pane":{…}}}}`.
- Error: `{"id":"cli:plugin","error":{"code":"plugin_pane_not_found","message":"plugin pane not found"}}`, exit 1.

### `workspace create`

- **Flags** [help]: `--cwd <PATH>`, `--label <TEXT>`, `--env <KEY=VALUE>`, `--focus`, `--no-focus`.
- **Behaviour:**
  - Focus defaults to **false** [src: src/cli/workspace.rs `workspace_create`].
  - `--label` sets a custom name. The new workspace goes last in the sidebar.
- **Events:** emits `workspace.created`, `tab.created`, `pane.created` and `layout.updated`. With `--focus`, the focus events follow [src: src/app/api/workspaces.rs `handle_workspace_create`; creation.rs `emit_workspace_open_events`].
- **Errors:** `workspace_create_failed`, plus env validation errors.

```json
{"id":"cli:workspace:create","result":{"type":"workspace_created","workspace":{"workspace_id":"w3",…},
 "tab":{"tab_id":"w3:t1",…},"root_pane":{"pane_id":"w3:p1","cwd":"/work/repos/api",…}}}
```

### `workspace focus <workspace_id>`

- Success: `{"id":"cli:workspace:focus","result":{"type":"workspace_info","workspace":{WorkspaceInfo}}}`.
- Error: `workspace_not_found` with message `workspace <id> not found`, exit 1 [src: workspaces.rs `handle_workspace_focus`].

### `pane split [PANE_ID]`

- **Flags** [help]:
  - `--pane <ID>`, `--current`
  - `--direction right|down` (required)
  - `--ratio <FLOAT>`, `--cwd <PATH>`, `--env <KEY=VALUE>`
  - `--right-click herdr|pane`
  - `--focus`, `--no-focus`
- **Target pane:**
  - The positional id or `--pane`.
  - Else the **caller's `HERDR_PANE_ID`**, else the focused pane.
  - `--current` fails without `HERDR_PANE_ID` [src: src/cli/pane.rs `parse_pane_split_args`; docs: cli-reference § Panes].
- **Other defaults:**
  - Focus: false.
  - Ratio: 0.5.
  - cwd: follows `terminal.new_cwd` from the target pane [src: src/app/api/panes.rs `handle_pane_split`].
- **Success:** `{"id":"cli:pane:split","result":{"type":"pane_info","pane":{PaneInfo}}}`. The new id is `.result.pane.pane_id`.
- **Errors:** `pane_not_found`, `pane_split_failed`.

### `pane run <PANE_ID> <COMMAND>...`

- The arguments after the pane id are joined with single spaces. The text is sent with an Enter key (`pane.send_input`) into whatever is running in the pane [src: cli/pane.rs `pane_run`].
- Success: **no stdout**, exit 0.
- Error: stderr `{"id":"cli:request","error":{…}}` (e.g. `pane_not_found`, `pane_send_failed`, `invalid_key`), exit 1.

### `pane zoom [PANE_ID]`

- **Flags** [help]: `--pane <ID>`, `--current`, and one of `--toggle` (default), `--on`, `--off`.
- **Targeting:** an omitted id **and `--current`** both send `pane_id: null`. The server then zooms the **focused pane**, not the caller's `HERDR_PANE_ID`. The docs say `--current` means the caller's pane; the source does not do that [src: cli/pane.rs `parse_pane_zoom_args`; panes.rs `resolve_optional_pane`].
- **Success:**

```json
{"id":"cli:pane:zoom","result":{"type":"pane_zoom","zoom":{"changed":true,"zoom_changed":true,"focus_changed":false,
 "pane_id":"w1:p2","focused_pane_id":"w1:p2","zoomed":true,"layout":{PaneLayoutSnapshot}}}}
```

- **`reason`** is present only when nothing changed: `single_pane`, `already_zoomed` or `already_unzoomed` [src: panes.rs `handle_pane_zoom`; schema: `PaneZoomResult`].

### `pane resize`

- **Flags** [help]: `--direction left|right|up|down` (required), `--amount <FLOAT>`, `--pane <ID>`, `--current`.
- **Targeting:** the same caveat as zoom. An omitted target or `--current` means the server's focused pane.
- **`--amount`:**
  - It is a split-**ratio** delta, not cells.
  - Default 0.05. The absolute value is used, capped at 0.5.
  - The resulting ratio is clamped to 0.1–0.9.

  [src: panes.rs `handle_pane_resize`; src/layout.rs `resize_focused`, `set_ratio_at`]
- **Direction:**
  - `right`/`down` **raise** the ratio (first child's share) of the nearest split on that side of the pane. When that side has no split, the opposite side's split is used.
  - So for a rail that is the right child at the right edge, `--direction right --amount 0.25` takes 0.5 to 0.75, and the rail gets about 25 %.
- **Success:** `{"id":"cli:pane:resize","result":{"type":"pane_resize","resize":{"changed":true,"pane_id":"…","focused_pane_id":"…","layout":{…}}}}`.
  - `reason:"unchanged"` appears when nothing changed.
  - `layout.area` and the rects are in cells, which is enough to compute an amount for a target column count.

### `workspace report-metadata`

See §6. Flags [help]: `<WORKSPACE_ID> --source <ID> [--token NAME=VALUE]… [--clear-token NAME]… [--seq N] [--ttl-ms N]`.

## 4. Inside a plugin split pane

### Environment

Sources: plugins/panes.rs `plugin_pane_launch_env`; src/workspace.rs `split_pane_with_runtime` → `launch_env_for_new_pane`; src/pane.rs `apply_pane_launch_env`; the test in plugins/mod.rs asserts `HERDR_PANE_ID == plugin_pane.pane.pane_id`.

- **The pane's own identity:**
  - `HERDR_PANE_ID`, `HERDR_TAB_ID` and `HERDR_WORKSPACE_ID` describe **the plugin pane itself**.
  - They are set at launch and are not updated if the pane later moves.
- **Plugin variables:**
  - `HERDR_PLUGIN_ID`
  - `HERDR_PLUGIN_ENTRYPOINT_ID` (the `[[panes]].id`, e.g. `rail`)
  - `HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR`, `HERDR_PLUGIN_STATE_DIR`
  - `HERDR_PLUGIN_CONTEXT_JSON`
  - `HERDR_SOCKET_PATH`, `HERDR_BIN_PATH`, `HERDR_ENV=1`
  - `PWD` = the pane cwd
- **Not set:** `HERDR_PLUGIN_ACTION_ID`, `HERDR_PLUGIN_EVENT`.
- **`--env`** values are added, but they cannot override any of the protected keys above.
- **Inherited:** everything else comes from the Herdr server process env.

### cwd and program

- The pane's cwd is `--cwd` if given, else the plugin root.
- `command[0]` of the form `./bin/x` resolves against the plugin root even when `--cwd` is set.

### `HERDR_PLUGIN_CONTEXT_JSON` for split or zoomed placement

- It is the context of the **target** pane (the pane split from), frozen at open time [src: panes.rs `open_plugin_split_pane` → `plugin_context_for_pane`; context.rs `plugin_context_from_parts`].
- `focused_pane_id` is therefore the target pane, **not** the rail.
- `workspace_cwd` equals `focused_pane_cwd` when known.

```json
{"workspace_id":"w1","workspace_label":"api","workspace_cwd":"/work/repos/api","tab_id":"w1:t1","tab_label":"…",
 "focused_pane_id":"w1:p1","focused_pane_cwd":"/work/repos/api","focused_pane_agent":"claude","focused_pane_status":"idle",
 "invocation_source":"api","correlation_id":"plugin-pane"}
```

- Every field is omitted when absent; `worktree` appears only for worktree workspaces [src: src/api/schema/plugins.rs `PluginInvocationContext`].

## 5. Event hooks: `HERDR_PLUGIN_EVENT_JSON`

### Envelope

- The variable holds the serialized `EventEnvelope`: `{"event":"<snake_case kind>","data":{"type":"<same snake_case>",…}}`.
- `HERDR_PLUGIN_EVENT` holds the dot name [src: src/api/schema/events.rs `EventEnvelope`, `EventKind`, `EventData`; plugins/runtime.rs `run_plugin_event_hooks`; schema: `event`].
- `AgentStatus` values: `idle`, `working`, `blocked`, `done`, `unknown`.

### `pane.agent_status_changed`

- Required fields: `pane_id`, `workspace_id`, `agent_status`.
- Optional fields: `agent`, `title`, `display_agent`, `state_labels`.

```json
{"event":"pane_agent_status_changed","data":{"type":"pane_agent_status_changed","pane_id":"w1:p1","workspace_id":"w1","agent_status":"done","agent":"claude"}}
```

- It fires when the computed status changes, **or** when `title`, `display_agent` or `state_labels` change while the status stays the same [src: src/app/api.rs (agent update → `PaneAgentStatusChanged`)].
- `done` means finished and not yet seen. When the user looks at the pane it becomes `idle`, so one finished turn usually fires `done` and then `idle`.

### `workspace.created`

```json
{"event":"workspace_created","data":{"type":"workspace_created","workspace":{"workspace_id":"w3","number":3,"label":"api","focused":false,"pane_count":1,"tab_count":1,"active_tab_id":"w3:t1","agent_status":"unknown"}}}
```

### `workspace.closed`

```json
{"event":"workspace_closed","data":{"type":"workspace_closed","workspace_id":"w3","workspace":{"workspace_id":"w3",…}}}
```

- `workspace` holds the WorkspaceInfo from before the close. It is optional and absent when a pane move closes the workspace [src: panes.rs L1330].
- The hook's context comes from that pre-close snapshot: `workspace_id`, `workspace_label` and `tab_id`, with no pane. When `workspace` is absent, the context is `{workspace_id}` only [src: context.rs `plugin_context_for_workspace_snapshot`].

### `workspace.focused`

```json
{"event":"workspace_focused","data":{"type":"workspace_focused","workspace_id":"w1"}}
```

- It is emitted with `tab.focused` and `pane.focused` **whenever the focused (workspace, pane) pair changes.** That includes switching panes inside the same workspace, such as moving focus into or out of the rail [src: src/app/api.rs `emit_focus_api_events` and its callers; src/server/headless/client_views.rs].

### Hook `HERDR_WORKSPACE_ID` and `HERDR_PANE_ID`

- For `workspace.*` events: that workspace, and its active tab's focused pane.
- For `pane.*` events: the event's pane [src: plugins/context.rs `plugin_context_for_event`].

## 6. `workspace report-metadata`

- **`--seq`:**
  - It is a `u64`, from 0 to 18446744073709551615. It is parsed by Rust's `u64::from_str`, which also accepts a leading `+` [schema: `request/$defs/WorkspaceReportMetadataParams.seq` (`format: uint64, minimum: 0`); src: cli.rs `parse_u64_flag`].
  - There is no upper bound beyond u64.
- **Freshness** [src: src/metadata_tokens.rs `sequence_is_fresh`, `accept_sequence`; workspaces.rs `handle_workspace_report_metadata`]:
  - The check is per workspace and per source. A report is accepted only if that source has no earlier sequence or `seq > last`.
  - A stale report still **succeeds** (exit 0, no output) and is silently ignored.
  - A report without `--seq` always applies and doesn't move `last`.
  - For a given workspace and source, two calls in the same millisecond with seq = Unix ms: the second is dropped.
  - The sequence map starts empty for workspaces restored after a restart [src: src/persist/restore.rs L429].
- **Required arguments:**
  - At least one `--token` or `--clear-token`. Without one: `missing token to set or clear`, exit 2.
  - A non-blank `--source`. Without it: `missing required --source`, exit 2.
  - `--token` splits at the first `=` [src: cli/workspace.rs; cli.rs `parse_token_assignment`].
- **Success:** **prints nothing**, exit 0 (`send_ok_request`, request id `cli:request`).
- **Unknown or closed workspace id:** the server checks this first, before source, ttl and token validation. stderr gets the following, with exit 1:

```json
{"id":"cli:request","error":{"code":"workspace_not_found","message":"workspace w7 not found"}}
```

- **Other error codes:** `invalid_metadata_source`, `invalid_metadata_ttl`, `invalid_metadata_token`, `metadata_token_limit`, `metadata_sequence_source_limit`.

## 7. How plugin commands resolve programs

- **Runtime commands** (`[[actions]]`, `[[events]]`, `[[startup]]`) [src: src/plugin_command.rs `program_for_cwd`, `command_for_argv_in_dir`; plugins/runtime.rs `start_plugin_command`; mod.rs `handle_plugin_pane_open`]:
  - They run as `command[0]` with arguments `command[1..]`, and **cwd = plugin root**.
  - If `command[0]` is relative and contains `/`, it is joined to the plugin root: `./bin/githerd` becomes `<root>/bin/githerd`, with the leading `./` stripped.
  - Only `command[0]` is rewritten.
  - Bare names (`githerd`) are looked up on PATH.
- **The same rule applies to:**
  - `[[build]]`, resolved against the manifest root;
  - `[[panes]]`, resolved against the plugin root even when `--cwd` is set.
- **`HERDR_PLUGIN_ROOT`** is the canonicalized directory of `herdr-plugin.toml` [src: plugins/manifest.rs `load_plugin_manifest`; src/plugin_paths.rs `managed_installations_dir`, `create_managed_installation`]:
  - For an installed plugin: `<herdr config dir>/plugins/github-installations/<id>/<pid>-<gen>/checkout[/subdir]`. It is a **new path after every `plugin update`**.
  - For a linked plugin: the linked directory.
  - So a running binary can find itself with `$HERDR_PLUGIN_ROOT/bin/githerd` or `os.Executable()`.
- **Runtime command environment:**
  - The Herdr **server** process env, not cleared, plus the plugin variables.
  - stdout and stderr are piped and capped at 64 KiB, so they are not a TTY. stdin is not set and is inherited from the server.
- **Variables by command kind:**
  - Action commands get `HERDR_PLUGIN_ACTION_ID`.
  - Only pane commands get `HERDR_PLUGIN_ENTRYPOINT_ID`.

## 8. `[[build]]` environment

- **Where it runs:** inside the `herdr plugin install` / `herdr plugin update` **CLI process**, not the server. The installer is only reached from the CLI (`install_github_plugin` callers in src/cli/plugin.rs).
- **Environment:**
  - Inherited from the shell that ran the command, including `PATH`, `HOME`, `GOPATH` and `GOCACHE`.
  - **Removed:** `HERDR_SOCKET_PATH`, the client-socket variable, `HERDR_SESSION`, `HERDR_BIN_PATH`, `HERDR_ENV`, `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID`, `HERDR_PANE_ID` and every `HERDR_PLUGIN_*` variable.
  - So `go` is found only if it is on that shell's PATH [src: cli/plugin.rs `run_plugin_build_command`, `scrub_herdr_runtime_env`].
- **Working directory:** the manifest root inside the new installation's `checkout`, which is a git checkout made with `git init` + fetch + `checkout --detach`.
- **Process:** stdin is null. stdout and stderr are captured (tail-capped). There is no timeout. Platform filters apply.
- **Abort conditions:**
  - A non-zero exit aborts the install with "Plugin was not installed/updated."
  - A build that changes `herdr-plugin.toml` aborts the install [src: cli/plugin.rs `ensure_manifest_unchanged_after_build`].
