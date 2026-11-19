# 02: Fetch with a TTL, and stale repos

**What to build:** `githerd status --fetch` updates remote state before reporting. A fetch skips repos whose `FETCH_HEAD` is younger than the TTL, unless forced. It runs in parallel, never prompts and times out. A repo whose fetch fails is shown as stale with the error. See the spec: Fetch.

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] Fetch uses the remote of the current branch's upstream; repos with no upstream remote are skipped
- [ ] The TTL clock is the mtime of `FETCH_HEAD`; a force flag ignores it; the TTL is configurable
- [ ] Fetches run with bounded concurrency and a per-repo timeout
- [ ] Fetch never prompts: terminal prompts off; SSH in batch mode unless the user set their own SSH command
- [ ] A failed or timed-out fetch marks the repo stale with its error; the table notes it and JSON carries it; the summary counts stale repos
- [ ] Tests: TTL skip and refetch (set `FETCH_HEAD` mtime), a new upstream commit becomes behind after fetch, an unreachable remote becomes stale without hanging
