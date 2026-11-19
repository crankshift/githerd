# githerd

githerd reports which git repos under a root are uncommitted, ahead or behind, in the terminal and in a Herdr plugin. `README.md` is the user's view: setup, keys, tokens.

## Ground rules

- **Standard library only** (ADR-0002). `go.mod` has no `require`; keep it that way.
- **Read-only** (ADR-0004). The only writes to a repo are `git fetch` and, after a failed fetch, an empty `FETCH_HEAD` that records the failure (ticket 08). Run git reads with `--no-optional-locks`.
- **Linux and macOS.** `internal/term` builds only there, and the sync lock uses `flock`.
- **The fake `herdr` only.** Tests reach Herdr through `$HERDR_BIN_PATH`, pointed at the fake. The real `herdr` would act on your live Herdr session.
- **Glossary words** from `GLOSSARY.md` in code, tests and docs: repo, root workspace, mapped workspace, rail, repo page, uncommitted, ahead, behind, stale.

## Layout

- `main.go`, `status.go`, `herdr.go`, `rail.go`: the `githerd` command. `rail.go` is the rail's terminal layer: it runs the model's commands and draws its view.
- `internal/discovery`, `internal/status`, `internal/fetch`: find repos under a root, read each one's state, fetch with the TTL. The TTL and stale rule live in `fetch` (`Run`, `Last`), so every caller shares them.
- `internal/herdr`: the Herdr adapter. `Sync` is the one entry point for the roots file, workspace mapping, tokens, `--seq` and the lock. It also holds the rail action and the Herdr requests the rail makes.
- `internal/repopage`: git reads for the repo page. `Read` lists the sections; `Show` reads the detail of one line.
- `internal/tui`: the rail and the repo page as a pure model: `Update(model, msg)` returns the next model and `[]Cmd`, and `View(model, w, h)` returns a `Screen`. It does no I/O. Herdr requests are `Cmd` values.
- `internal/term`: raw mode, the alternate screen, size, key decoding.
- `internal/gitfixture`: real git repos in temp dirs for tests.
- `herdr-plugin.toml`: the plugin manifest. `manifest_test.go` checks it.
- `.scratch/githerd-v1/`: the spec and tickets. A ticket's `## Comments` record decisions the spec left open.

## Tests

```sh
go test ./...
go vet ./...
```

`go` must be on `PATH`: `TestMain` builds the binary with `go build`. The suite takes about 20 seconds.

A test asserts external behaviour: command output, the argv sent to Herdr, the rendered screen. It never calls an internal function, so it survives refactors.

### Seam 1: the binary

The `*_test.go` files at the root, package `main_test`.

- `TestMain` builds `githerd` once.
- The same test binary re-runs as the fake `herdr` when `GITHERD_TEST_FAKE_HERDR` names its dir (`fakeherdr_test.go`). It serves `snapshot.json` and appends each argv to `calls.jsonl`. `newFakeHerdr` gives a test the env a Herdr hook gets.
- Fixtures come from `internal/gitfixture`:
  - `EveryState()` builds a root with one repo in each state.
  - `FakeSSH()` gives unreachable and hanging remotes.
  - `Backdate` and `FetchedAgo` set `FETCH_HEAD` mtimes for TTL tests.
  - `Env()` is hermetic: no user or system git config.
- `pty_test.go` and its per-OS files open a real pty, in which `rail_test.go`, `page_test.go` and `open_test.go` run `githerd rail`.

### Seam 2: the TUI model

`internal/tui`.

- Tests feed `Update` keys and answers such as `tui.Loaded`, built from in-code data (`work()` in `rail_test.go`).
- They assert the `Cmd` values `Update` returns.
- They compare `View` at a fixed size with `testdata/*.golden`. A golden frames each row in `│` and marks `selected` and `dim` after it.

To rewrite the goldens after a deliberate change to the screen:

```sh
go test ./internal/tui -update
```

Then read the golden diff line by line: it is the spec of the screen. Only rows you meant to change should move.

`internal/repopage` tests read real fixture repos. One hashes every file of the repo before and after, to prove the reads write nothing.

### Adding a Herdr request

1. Add a `Cmd` type in `internal/tui/model.go`, returned by `Update`, and assert it in `internal/tui`.
2. Run it in the `switch` in `rail.go`, through a function in `internal/herdr`.
3. Assert its exact argv through the binary with the fake `herdr`, and teach `runFakeHerdr` Herdr 0.9.5's answer. `.scratch/research/herdr-json-v0.9.5.md` has the flags and JSON shapes.

## Agent skills

### Issue tracker

Issues live as local markdown files under `.scratch/`. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default triage labels (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
