# Refresh without Herdr timers, and tokens without a TTL

Herdr gives plugins no timers, so githerd refreshes from three sources: the rail's own 30-second loop while it is open; Herdr hooks (`[[startup]]`, `workspace.created`, `workspace.closed`, `workspace.focused`, and `pane.agent_status_changed` when an agent turns done or idle); and the keybinding. Sidebar tokens are reported without `--ttl-ms`, because an expired token vanishes and a missing token reads as "all clean".

## Consequences

Herdr does not restore tokens after a restart, so the `[[startup]]` hook must report them again. A `tab_bar_right` command entry can act as an extra timer, but it lives in the user's config, so it is documented as optional.
