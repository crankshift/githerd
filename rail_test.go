package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
)

// The rail as Herdr runs it in the pane it opened: it marks its pane, shows
// the root of the pane it was opened from with the cursor on that pane's
// repo, runs the sync at once, moves with the keys, redraws on resize, and
// on q leaves the terminal as it found it.
func TestRailRunsInItsPaneAndRestoresTheTerminalOnQuit(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	work := f.EveryState()
	api := filepath.Join(work, gitfixture.UncommittedRepo)
	h := newFakeHerdr(t, f)
	h.roots(work + "\n")
	h.snapshot(at("w1", api), at("w2", work))
	term := newPTY(t, 60, 20)
	before := term.termios()

	exited := h.startRail(term, api)
	pos := term.waitFor(0, "\x1b[?1049h")
	pos = term.waitFor(pos, "\x1b[7mrepos/acme/api")
	term.waitFor(pos, "need attention")
	pos = h.waitForSync(term, pos)

	term.typed("j")
	pos = term.waitFor(pos, "\x1b[7mrepos/acme/billing")
	term.typed("\x1b[B")
	pos = term.waitFor(pos, "\x1b[7mrepos/acme/mobile")
	term.resize(40, 12)
	pos = term.waitFor(pos, "\x1b[0;2m"+strings.Repeat("─", 40)+"\x1b[0m")

	term.typed("q")
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("the rail exited with %v; terminal:\n%q", err, term.output())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("the rail did not quit on q; terminal:\n%q", term.output()[pos:])
	}
	if out := term.output(); !strings.HasSuffix(out, "\x1b[?25h\x1b[?1049l") {
		t.Errorf("the rail did not leave the alternate screen and show the cursor; it ended with %q", out[max(0, len(out)-40):])
	}
	if after := term.termios(); after.Lflag != before.Lflag || after.Iflag != before.Iflag || after.Oflag != before.Oflag {
		t.Errorf("the terminal's mode was not restored:\n before %+v\n  after %+v", before, after)
	}
	if mark := []string{"pane", "report-metadata", "w1:p9", "--source", "githerd", "--token", "githerd=rail"}; !slices.ContainsFunc(h.peekCalls(), func(c []string) bool { return slices.Equal(c, mark) }) {
		t.Errorf("the rail did not mark its pane with %q; herdr calls %q", mark, h.peekCalls())
	}
}

// f fetches every repo of the root at once, even one fetched within the
// TTL, so commits pushed since show up as behind.
func TestRailFetchesEveryRepoAtOnceWithF(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	remote := f.Remote("api")
	api := f.Clone(remote, "work/api")
	f.FetchedAgo(api, time.Minute) // within the 15-minute TTL
	f.PushElsewhere(remote, "main", 2)
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("w1", api))
	term := newPTY(t, 60, 20)

	exited := h.startRail(term, api)
	pos := h.waitForSync(term, 0)
	if strings.Contains(term.output(), "⇣2") {
		t.Fatalf("the rail fetched a repo within the TTL before f; terminal:\n%q", term.output())
	}
	term.typed("f")
	pos = term.waitFor(pos, "fetching…")
	term.waitFor(pos, "⇣2")

	term.typed("q")
	if err := <-exited; err != nil {
		t.Fatalf("the rail exited with %v", err)
	}
}

// f fetches in a sync, under the sync lock, so it never races a hook's
// fetch of the same repo: it waits for the hook's sync to end, then fetches
// whatever the TTL.
func TestRailForcesItsFetchUnderTheSyncLock(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	f.FakeSSH()
	api := f.Clone(f.Remote("api"), "work/api")
	f.Git(api, "remote", "set-url", "origin", "ssh://git@"+gitfixture.HangingHost+"/acme/api.git")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("w1", api))
	hook := h.startSync("HERDR_PLUGIN_EVENT=workspace.focused")
	h.waitForCalls(2) // the snapshot and the first round: the hook's sync fetches next, and hangs
	term := newPTY(t, 60, 20)

	exited := h.startRail(term, api)
	pos := term.waitFor(0, "↻ ")
	term.typed("f")
	pos = term.waitFor(pos, "fetching…")
	term.waitFor(pos, "↻ ")
	term.typed("q")
	if err := <-exited; err != nil {
		t.Fatalf("the rail exited with %v", err)
	}
	if got := hook(); got.code != 0 || got.stderr != "" {
		t.Errorf("the hook's sync: want exit 0 and no output; got %+v", got)
	}

	var fetches []string
	for _, call := range f.SSHCalls() {
		if strings.HasPrefix(call, "outlived the fetch") {
			fetches = append(fetches, "ended")
		} else {
			fetches = append(fetches, "started")
		}
	}
	if want := []string{"started", "ended", "started", "ended"}; !slices.Equal(fetches, want) {
		t.Errorf("fetches over ssh %q, want the hook's, then f's: %q", fetches, want)
	}
}

// The rail's first view, read from disk before the sync ends, already
// shows a repo whose last fetch failed as stale.
func TestRailShowsStaleReposBeforeTheSyncEnds(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/api")
	fetchHead := f.Git(api, "rev-parse", "--path-format=absolute", "--git-path", "FETCH_HEAD")
	if err := os.WriteFile(fetchHead, nil, 0o644); err != nil { // as a failed fetch leaves it
		t.Fatal(err)
	}
	f.Backdate(api, time.Minute) // within the TTL, so the sync doesn't retry it
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("w1", api))
	release := h.hold()
	term := newPTY(t, 60, 20)

	exited := h.startRail(term, api)
	pos := term.waitFor(0, "1 stale")
	release()
	h.waitForSync(term, pos)

	term.typed("q")
	if err := <-exited; err != nil {
		t.Fatalf("the rail exited with %v", err)
	}
}

// Opening the rail moves the focus, which fires workspace.focused, so the
// hook's sync often holds the lock when the rail's first sync starts. The
// rail's sync then doesn't run, but the rail still reads the snapshot,
// which takes no lock, so its lines name the workspaces and o can focus
// them at once.
func TestRailMapsTheWorkspacesWhileAHookSyncHoldsTheLock(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	work := f.EveryState()
	api := filepath.Join(work, gitfixture.UncommittedRepo)
	h := newFakeHerdr(t, f)
	h.roots(work + "\n")
	h.snapshot(at("w1", work), at("w2", api))
	release := h.hold()
	hook := h.startSync("HERDR_PLUGIN_EVENT=workspace.focused")
	h.waitForCalls(1) // the hook's sync holds the lock, waiting on its snapshot
	term := newPTY(t, 60, 20)

	exited := h.startRail(term, api)
	h.waitForSnapshots(2) // the rail's, waiting too
	release()
	term.waitFor(0, "⧉ws-w2")
	if got := hook(); got.code != 0 || got.stderr != "" {
		t.Errorf("the hook's sync: want exit 0 and no output; got %+v", got)
	}
	h.calls()

	term.typed("o")
	h.expectCalls("o on api", []string{"workspace", "focus", "w2"})
	term.typed("q")
	if err := <-exited; err != nil {
		t.Fatalf("the rail exited with %v", err)
	}
}

// waitForSnapshots waits until herdr was asked for the snapshot n times
// since the last calls, and fails the test if that takes over 10 seconds.
func (h *fakeHerdr) waitForSnapshots(n int) {
	h.t.Helper()
	snapshots := func() int {
		got := 0
		for _, c := range h.peekCalls() {
			if slices.Equal(c, []string{"api", "snapshot"}) {
				got++
			}
		}
		return got
	}
	for deadline := time.Now().Add(10 * time.Second); snapshots() < n; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			h.t.Fatalf("waited 10s for %d snapshots; herdr calls %q", n, h.peekCalls())
		}
	}
}

// startRail starts githerd rail on term as Herdr runs it in the pane it
// opened, as w1:p9, split from a pane whose cwd is cwd, with extra env added.
// The channel gets the rail's exit.
func (h *fakeHerdr) startRail(term *pty, cwd string, extra ...string) <-chan error {
	h.t.Helper()
	cmd := exec.Command(githerdBin, "rail")
	cmd.Dir = h.f.Path(".")
	cmd.Env = append(append(h.env(), "PWD="+cmd.Dir, "HERDR_PANE_ID=w1:p9",
		`HERDR_PLUGIN_CONTEXT_JSON={"workspace_id":"w1","focused_pane_id":"w1:p1","focused_pane_cwd":"`+cwd+`"}`), extra...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = term.slave, term.slave, term.slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		h.t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	h.t.Cleanup(func() { cmd.Process.Kill() })
	return exited
}

// waitForSync waits until the rail has run a sync, which reported to a
// workspace, and drawn its countdown after the output at pos. It returns
// where the countdown ends.
func (h *fakeHerdr) waitForSync(term *pty, pos int) int {
	h.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !slices.ContainsFunc(h.peekCalls(), isWorkspaceReport); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			h.t.Fatalf("the rail ran no sync; herdr calls %q", h.peekCalls())
		}
	}
	return term.waitFor(pos, "↻ ")
}

func isWorkspaceReport(argv []string) bool {
	return len(argv) > 2 && argv[0] == "workspace" && argv[1] == "report-metadata"
}

// The rail runs only in a terminal inside Herdr.
func TestRailFailsOutsideHerdrAndOutsideATerminal(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	h := newFakeHerdr(t, f)

	got := githerd(t, f.Env(), f.Path("."), "rail")
	if want := "githerd rail: HERDR_BIN_PATH is not set; run this from Herdr\n"; got.code != 2 || got.stderr != want {
		t.Errorf("outside Herdr: want exit 2 and %q; got %+v", want, got)
	}
	got = githerd(t, h.env(), f.Path("."), "rail")
	if want := "githerd rail: not a terminal\n"; got.code != 1 || got.stderr != want {
		t.Errorf("outside a terminal: want exit 1 and %q; got %+v", want, got)
	}
}

// railAction runs githerd herdr rail as Herdr runs the rail action.
func (h *fakeHerdr) railAction() result {
	h.t.Helper()
	return githerd(h.t, h.env(), h.f.Path("."), "herdr", "rail")
}

// assertCalls fails unless the run exited 0 with no output, and herdr was
// called with exactly want, in order.
func (h *fakeHerdr) assertCalls(got result, want [][]string) {
	h.t.Helper()
	if got.code != 0 || got.stdout != "" || got.stderr != "" {
		h.t.Fatalf("want exit 0 and no output; got %+v", got)
	}
	if calls := h.calls(); !reflect.DeepEqual(calls, want) {
		h.t.Errorf("herdr calls:\n got %q\nwant %q", calls, want)
	}
}

// With no rail in the current workspace, the action opens one as a split
// to the right of the focused pane, marks it with the rail's pane token, and
// narrows it to about a quarter of the width: the split starts at half, and
// the amount is a ratio delta.
func TestRailActionOpensTheRailBesideTheFocusedPane(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	h := newFakeHerdr(t, f)
	h.snapshot(
		at("w1", f.Path("work")),
		workspace{id: "w2", tabs: [][]pane{{{cwd: f.Path("work")}, {cwd: f.Path("work/api"), focused: true}}}},
	)

	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "open", "--plugin", "githerd", "--entrypoint", "rail", "--placement", "split",
			"--target-pane", "w2:p2", "--direction", "right", "--focus"},
		{"pane", "report-metadata", "w2:p9", "--source", "githerd", "--token", "githerd=rail"},
		{"pane", "resize", "--pane", "w2:p9", "--direction", "right", "--amount", "0.25"},
	})
}

// A pane can keep the rail's token after Herdr has forgotten it is a plugin
// pane, as after a live handoff, so focusing it fails. The action then tries
// the next marked pane, and opens a fresh rail when none can be focused, so
// pressing the key again never piles up rails.
func TestRailActionOpensAFreshRailWhenTheMarkedPaneIsNoLongerAPluginPane(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	rail := map[string]string{"githerd": "rail"}
	h := newFakeHerdr(t, f)
	h.forget("w1:p2")
	h.snapshot(workspace{id: "w1", tabs: [][]pane{{{cwd: f.Path("work"), focused: true}, {cwd: f.Path("work"), tokens: rail}}}})

	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "focus", "w1:p2"},
		{"plugin", "pane", "open", "--plugin", "githerd", "--entrypoint", "rail", "--placement", "split",
			"--target-pane", "w1:p1", "--direction", "right", "--focus"},
		{"pane", "report-metadata", "w1:p9", "--source", "githerd", "--token", "githerd=rail"},
		{"pane", "resize", "--pane", "w1:p9", "--direction", "right", "--amount", "0.25"},
	})

	h.snapshot(workspace{id: "w1", tabs: [][]pane{{{cwd: f.Path("work"), focused: true}, {cwd: f.Path("work"), tokens: rail}, {cwd: f.Path("work"), tokens: rail}}}})
	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "focus", "w1:p2"},
		{"plugin", "pane", "focus", "w1:p3"},
	})
}

// A rail already open anywhere in the current workspace, in any tab, is
// focused instead of opening a second one. A rail in another workspace
// doesn't count.
func TestRailActionFocusesTheRailOfTheCurrentWorkspace(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	rail := map[string]string{"githerd": "rail"}
	h := newFakeHerdr(t, f)
	h.snapshot(
		workspace{id: "w1", tabs: [][]pane{{{cwd: f.Path("work")}, {cwd: f.Path("work"), tokens: rail}}}},
		workspace{id: "w2", tabs: [][]pane{
			{{cwd: f.Path("work/api"), focused: true}},
			{{cwd: f.Path("work/api")}, {cwd: f.Path("work/api"), tokens: map[string]string{"other": "rail"}}, {cwd: f.Path("work"), tokens: rail}},
		}},
	)

	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "focus", "w2:p4"},
	})

	h.snapshot(
		workspace{id: "w1", tabs: [][]pane{{{cwd: f.Path("work")}, {cwd: f.Path("work"), tokens: rail}}}},
		workspace{id: "w2", tabs: [][]pane{{{cwd: f.Path("work/api"), focused: true}}}},
	)
	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "open", "--plugin", "githerd", "--entrypoint", "rail", "--placement", "split",
			"--target-pane", "w2:p1", "--direction", "right", "--focus"},
		{"pane", "report-metadata", "w2:p9", "--source", "githerd", "--token", "githerd=rail"},
		{"pane", "resize", "--pane", "w2:p9", "--direction", "right", "--amount", "0.25"},
	})
}
