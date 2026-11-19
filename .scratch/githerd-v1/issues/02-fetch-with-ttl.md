# 02: Fetch with a TTL, and stale repos

**What to build:** `githerd status --fetch` updates remote state before reporting. A fetch skips repos whose `FETCH_HEAD` is younger than the TTL, unless forced. It runs in parallel, never prompts and times out. A repo whose fetch fails is shown as stale with the error. See the spec: Fetch.

**Blocked by:** 01

**Status:** resolved

- [x] Fetch uses the remote of the current branch's upstream; repos with no upstream remote are skipped
- [x] The TTL clock is the mtime of `FETCH_HEAD`; a force flag ignores it; the TTL is configurable
- [x] Fetches run with bounded concurrency and a per-repo timeout
- [x] Fetch never prompts: terminal prompts off; SSH in batch mode unless the user set their own SSH command
- [x] A failed or timed-out fetch marks the repo stale with its error; the table notes it and JSON carries it; the summary counts stale repos
- [x] Tests: TTL skip and refetch (set `FETCH_HEAD` mtime), a new upstream commit becomes behind after fetch, an unreachable remote becomes stale without hanging

## Comments

Decisions made while building, where the spec left room:

- **A failed fetch is retried whatever the TTL.** Git empties `FETCH_HEAD` and bumps its mtime when a fetch fails. Read naively, the TTL would then skip the repo for 15 minutes and its stale mark would vanish on the next sync. So an empty `FETCH_HEAD` counts as never fetched: the repo is due again, and `fetchedAt` is null. The prototype does the same: stale repos are always due. Still no state file.
- **The module.** `fetch.Run(ctx, dirs, fetch.Options{TTL, Force, Timeout})` returns one `Outcome{Dir, Remote, Fetched, Err}` per repo; `Err` set means stale. Zero `Timeout` means 20s; 8 fetches run at once.
- **The CLI.** `--fetch` uses the plugin's 15-minute TTL. `--ttl D`, `--force` and `--fetch-timeout D` tune it and are usage errors without `--fetch`.
- **The error.** It is the first line of git's stderr, such as `fatal: '…' does not appear to be a git repository`, or `timed out after 20s`. JSON `stale` carries it. The table notes `stale`, the summary adds `, K stale`, and the reasons go to stderr. A stale repo does not exit non-zero.
- **Never prompting.** `GIT_TERMINAL_PROMPT=0` is always set. `GIT_SSH_COMMAND="ssh -o BatchMode=yes"` is set unless `GIT_SSH_COMMAND`, `GIT_SSH` or `core.sshCommand` is set. Git also runs with stdin closed, in its own session with no controlling terminal. On timeout, the whole process group is killed, so a hung ssh dies with its fetch.
