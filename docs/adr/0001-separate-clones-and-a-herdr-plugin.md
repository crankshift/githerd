# Separate clones and a Herdr plugin, not submodules or lazygit

A root directory holds many independent git clones, gitignored by the root and not registered as submodules; githerd reports their state, and its UI is a Herdr plugin.

## Considered Options

- **Git submodules**: rejected. A submodule pins a commit, so every commit in a child repo would need a matching bump commit in the root.
- **lazygit `customCommands`**: rejected. lazygit has no plugin system; a custom command only binds a key to a shell command whose output lands in a popup or terminal, which is not real UI integration.
- **Herdr plugin**: chosen. Herdr shows custom tokens on its sidebar rows and runs plugin actions and panes, so repo state can sit next to the workspace it belongs to.
