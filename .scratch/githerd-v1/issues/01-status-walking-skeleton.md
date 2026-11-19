# 01: `githerd status` walking skeleton

**What to build:** `githerd status [root] [--json] [--depth N]` finds every repo under a root and prints each one's state as a table plus a summary line, or as JSON. It stays local, with no fetch. This ticket also sets up the Go module (standard library only) and the git fixture helper that every later test uses. See the spec: Discovery, Status, Status command.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] Go module `github.com/crankshift/githerd` with a `githerd` command; no third-party dependencies
- [ ] Test fixture helper builds, in a temp dir, a bare "remote" plus clones in each state: uncommitted (modified and untracked), ahead, behind, diverged, no upstream with local commits, no remote at all, detached, and a repo nested in a repo
- [ ] Discovery finds repos with a `.git` directory or file, to depth 3 by default, descending into repos, skipping hidden directories and `node_modules`; the root is listed first when it is a repo
- [ ] Each repo reports branch or detached commit, upstream, uncommitted, ahead, behind; with no upstream, ahead counts commits on no remote branch
- [ ] Needs attention follows the glossary; detached alone does not count
- [ ] Table output is sorted by path with a summary line "N of T repos need attention"; `--json` matches the shape in the spec
- [ ] Black-box tests run the binary against the fixtures and assert on its output
