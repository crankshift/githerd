# 08: Report before fetching; back off from failing remotes

**What to build:** `githerd herdr sync` reports tokens from local state first, then fetches the repos that are due and reports again only if something changed. One slow or hanging remote no longer delays the sidebar. A repo whose last fetch failed stays stale and is retried only once the TTL has passed since that failure, or when forced. Every caller of the Fetch module follows the same rule: `githerd status --fetch`, the sync, and later the rail.

**Blocked by:** 03

**Status:** ready-for-agent

- [ ] Sync sends its first reports before any fetch starts; after fetching it reports again only for workspaces whose tokens changed
- [ ] Both reports use increasing `--seq` values and respect the sync lock and pending mark
- [ ] A failed fetch leaves an empty `FETCH_HEAD` with the failure time as its mtime. Treat that as stale ("last fetch failed" when the original error isn't known), and retry only when that mtime is older than the TTL or the fetch is forced
- [ ] `githerd status --fetch` follows the same backoff; `--force` retries stale repos at once
- [ ] Black-box tests: a hanging remote doesn't delay the first report; a stale repo isn't refetched within the TTL; it is refetched after the TTL and with force
