# githerd is read-only in v1

githerd never changes a repo's branches, index or working tree: no stage, commit, checkout, pull or push. It shows state, commits and diffs, and hands off to a shell or lazygit in a split when you want to change something. Writing would need conflict, credential and confirmation handling inside the TUI, which the shell and lazygit already do well.

## Consequences

To learn about behind commits, githerd runs `git fetch`, which writes git's own fetch metadata: remote-tracking branches and `FETCH_HEAD`. After a failed fetch githerd also empties `FETCH_HEAD` and sets its mtime to the failure time, so the repo stays stale and the TTL counts from the failure (ticket 08). Those are its only writes: it never touches the working tree, the index, local branches or other refs, or the config.
