# githerd is read-only in v1

githerd never writes to a repo: no stage, commit, checkout, pull or push. It shows state, commits and diffs, and hands off to a shell or lazygit in a split when you want to change something. Writing would need conflict, credential and confirmation handling inside the TUI, which the shell and lazygit already do well.
