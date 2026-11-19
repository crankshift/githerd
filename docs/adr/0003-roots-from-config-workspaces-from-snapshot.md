# Roots come from config; workspaces map through their first pane

githerd reads its roots from a `roots` file in `$HERDR_PLUGIN_CONFIG_DIR`, one path per line, and maps each workspace with `herdr api snapshot`: a workspace belongs to the deepest repo that contains its first pane's cwd. Herdr workspaces have no root directory, and the plugin context's `workspace_cwd` follows whichever pane has focus, so an agent `cd`-ing into a nested repo would otherwise move the whole workspace.

## Consequences

A `cd` inside a pane fires no hookable Herdr event, so a workspace's mapping can lag until the next refresh. Without a `roots` file githerd reports nothing.
