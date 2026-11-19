# 08: Report before fetching; back off from failing remotes

**What to build:** `githerd herdr sync` reports tokens from local state first, then fetches the repos that are due and reports again only if something changed. One slow or hanging remote no longer delays the sidebar. A repo whose last fetch failed stays stale and is retried only once the TTL has passed since that failure, or when forced. Every caller of the Fetch module follows the same rule: `githerd status --fetch`, the sync, and later the rail.

**Blocked by:** 03

**Status:** resolved

- [x] Sync sends its first reports before any fetch starts; after fetching it reports again only for workspaces whose tokens changed
- [x] Both reports use increasing `--seq` values and respect the sync lock and pending mark
- [x] A failed fetch leaves an empty `FETCH_HEAD` with the failure time as its mtime. Treat that as stale ("last fetch failed" when the original error isn't known), and retry only when that mtime is older than the TTL or the fetch is forced
- [x] `githerd status --fetch` follows the same backoff; `--force` retries stale repos at once
- [x] Black-box tests: a hanging remote doesn't delay the first report; a stale repo isn't refetched within the TTL; it is refetched after the TTL and with force

## Comments

Decisions made while building, where the ticket left room:

- **This reverses ticket 02's "a failed fetch is retried whatever the TTL".** An empty `FETCH_HEAD` still means the last fetch failed, and `fetchedAt` is still null. Its mtime now starts the TTL like any other fetch.
- **The two rounds.**
  - Sync reads the roots, discovers and reads every repo, takes the snapshot, and sends the first round. Then it fetches what is due, rereads the repos it fetched, and sends the second round.
  - The second round reuses the first round's snapshot and mapping. It reports only to workspaces whose token set differs from the first round. When none differ, it takes no `--seq`.
  - Each round takes its own `--seq` under the lock, so the second is always above the first. The lock covers both rounds and the fetches: a sync triggered meanwhile leaves the pending mark, and the running sync reruns after its second round.
  - `Result` holds the state after the fetches. `herdr.Sync`'s signature is unchanged.
  - Cost: a workspace closed during a long fetch has its second report refused. That error goes to stderr and the sync exits 1, as ticket 03 does for any refused report.
- **Stale before fetching.** `fetch.Last(ctx, dirs)` returns, without fetching, the outcome `fetch.Run` would give each repo if none were due. It applies the same rule as `Run`: the branch has an upstream remote and its `FETCH_HEAD` is empty. So the first round and the second agree on repos that aren't fetched, and their stale mark doesn't flicker.
- **Marking a failure.**
  - Git empties `FETCH_HEAD` when a fetch *starts*. So a fetch killed on timeout left the start time as the mtime, and a late failure could leave partial content.
  - After any failed `git fetch`, githerd writes `FETCH_HEAD` empty and sets its mtime to now. This is the file git itself rewrites on every fetch; no ref, index, work tree or config is touched (ADR-0004).
  - If githerd is stopped mid-fetch, git has already emptied `FETCH_HEAD`, so that repo counts as failed too until the TTL passes or a forced fetch.
- **Where the rule lives.** It is in the Fetch module (`fetch.Run`, `fetch.Last`), so every caller gets it. `status.LastFetch` reads the mtime and whether `FETCH_HEAD` is empty, and `status.FetchHead` gives its path.
- **The CLI.**
  - `githerd status --fetch` skips a repo whose fetch failed within the TTL. It reports that repo stale with `last fetch failed`, in JSON and on stderr. `--force` and `--ttl` behave as before.
  - Plain `githerd status` still shows no stale marks.
