# githerd

githerd reports the state of many independent git repos under one root, in the terminal and inside Herdr.

## Language

**Root**:
The directory githerd scans for repos.
_Avoid_: parent workspace, monorepo

**Repo**:
A git repository found under the root, including the root itself when it is one.
_Avoid_: clone, project, child repo

**Workspace**:
A Herdr workspace, shown as a space row in the Herdr sidebar. Never a synonym for root.

**Root workspace**:
A workspace whose first pane's cwd lies inside a root but inside no nested repo. It carries the root's summary.

**Mapped workspace**:
A workspace whose first pane's cwd lies inside a nested repo; it maps to the deepest such repo.
_Avoid_: repo workspace, linked workspace

### Views

**Rail**:
The githerd pane that lists every repo under a root with its state.
_Avoid_: dashboard, panel, sidebar (the sidebar is Herdr's)

**Repo page**:
The full-tab view of one repo: its changes, commits to push and to pull, branches and log, beside a diff of the selected line.
_Avoid_: detail view

### Repo states

**Uncommitted**:
The repo's working tree or index has changes, untracked files included.
_Avoid_: dirty, modified

**Ahead**:
The current branch has commits its upstream lacks.
_Avoid_: unpushed

**Behind**:
The upstream has commits the current branch lacks, as of the last fetch.
_Avoid_: unpulled

**No upstream**:
The current branch has no upstream branch. A detached repo is never no upstream, because it has no branch.

**Detached**:
HEAD points at a commit rather than a branch.

**Stale**:
The repo's last fetch attempt failed, so behind may be out of date.

**Needs attention**:
A repo that is uncommitted, ahead, behind or no upstream. Detached alone does not count.
_Avoid_: dirty

**Clean**:
A repo that does not need attention.
