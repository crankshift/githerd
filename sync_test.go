package main_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
)

// A mapped workspace gets its repo's uncommitted count; the root's summary
// and the stale mark don't apply to it, so they are cleared.
func TestSyncReportsUncommittedChangesToAMappedWorkspace(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Write(api, "README.md", "# api, edited\n")
	f.Write(api, "src/auth.go", "package auth\n")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("w1", api))

	h.assertSynced(h.sync(), map[string]tokens{
		"w1": {"changes": "●2", "repos": cleared, "stale": cleared},
	})
}

// A workspace whose cwd lies in a root but in no nested repo is a root
// workspace, and gets the root's summary: how many repos need attention, of
// how many, or a check mark when none does.
func TestSyncReportsTheRootSummaryToRootWorkspaces(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	work := f.EveryState() // 6 of its 10 repos need attention
	f.Clone(f.Remote("site"), "projects/site")
	f.Clone(f.Remote("blog"), "projects/blog")
	h := newFakeHerdr(t, f)
	h.roots(work + "\n" + f.Path("projects") + "\n")
	h.snapshot(
		at("w1", work),
		at("w2", filepath.Join(work, "repos")),
		at("w3", f.Path("projects")),
	)

	h.assertSynced(h.sync(), map[string]tokens{
		"w1": {"repos": "6/10", "changes": cleared, "stale": cleared},
		"w2": {"repos": "6/10", "changes": cleared, "stale": cleared},
		"w3": {"repos": "✓2", "changes": cleared, "stale": cleared},
	})
}

// The roots file lists one root per line, with ~ for the home dir; blank
// lines and # comments are ignored. Each workspace gets the root that
// contains it.
func TestSyncReadsTheRootsFile(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	site := f.Clone(f.Remote("site"), "home/projects/site")
	f.Write(site, "index.html", "<h1>hi</h1>\n")
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Clone(f.Remote("old"), "attic/old")
	h := newFakeHerdr(t, f)
	h.roots("# Where my repos live\n" +
		"~/projects\n" +
		"\n" +
		"   " + f.Path("work") + "  \n" +
		"# " + f.Path("attic") + "\n")
	h.snapshot(
		at("projects", f.Path("home/projects")),
		at("site", site),
		at("work", f.Path("work")),
		at("api", api),
		at("attic", f.Path("attic/old")),
	)

	h.assertSynced(h.sync(), map[string]tokens{
		"projects": {"repos": "1/1", "changes": cleared, "stale": cleared},
		"site":     {"changes": "●1", "repos": cleared, "stale": cleared},
		"work":     {"repos": "✓1", "changes": cleared, "stale": cleared},
		"api":      {"changes": cleared, "repos": cleared, "stale": cleared},
	})
}

// Hooks run in the plugin root, which moves on every update, so a relative
// root would mean nothing: it is named on stderr and the others still sync.
func TestSyncRejectsARelativeRoot(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	h := newFakeHerdr(t, f)
	h.roots("projects\n" + f.Path("work") + "\n")
	h.snapshot(at("api", api))

	got := h.sync()

	if got.code != 1 || !strings.Contains(got.stderr, "roots:1: projects: not an absolute path") {
		t.Errorf("want exit 1 and line 1 of the roots file named on stderr; got %+v", got)
	}
	assertTokens(t, tokensByWorkspace(t, h.calls()), map[string]tokens{
		"api": {"changes": cleared, "repos": cleared, "stale": cleared},
	})
}

// A root or a pane cwd reached through a symlink still matches: Herdr may
// report a cwd as the shell spelled it or as the OS resolved it.
func TestSyncMatchesPathsThroughSymlinks(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Write(api, "notes.txt", "todo\n")
	site := f.Clone(f.Remote("site"), "projects/site")
	f.Write(site, "index.html", "<h1>hi</h1>\n")
	symlink(t, f.Path("work"), f.Path("work-link"))
	symlink(t, f.Path("projects"), f.Path("projects-link"))
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work-link") + "\n" + f.Path("projects") + "\n")
	h.snapshot(at("api", api), at("site", f.Path("projects-link/site")))

	h.assertSynced(h.sync(), map[string]tokens{
		"api":  {"changes": "●1", "repos": cleared, "stale": cleared},
		"site": {"changes": "●1", "repos": cleared, "stale": cleared},
	})
}

// With no roots configured, a fresh session gets nothing reported.
func TestSyncReportsNothingWithoutRoots(t *testing.T) {
	t.Parallel()
	for name, roots := range map[string]*string{
		"no roots file":    nil,
		"only comments":    ptr("# ~/work\n\n   \n"),
		"empty roots file": ptr(""),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := gitfixture.New(t)
			api := f.Clone(f.Remote("api"), "work/repos/acme/api")
			f.Write(api, "notes.txt", "todo\n")
			h := newFakeHerdr(t, f)
			if roots != nil {
				h.roots(*roots)
			}
			h.snapshot(
				at("w1", f.Path("work")),
				workspace{id: "w2", tabs: [][]pane{{{cwd: api}}}, tokens: map[string]string{"load": "90"}},
			)

			h.assertSynced(h.sync(), map[string]tokens{})
		})
	}
}

// A workspace outside every root, say after its pane left the root or the
// root left the roots file, has the tokens githerd reported to it before
// cleared. Other workspaces outside are left alone, and so are other
// sources' tokens.
func TestSyncClearsItsTokensFromWorkspacesOutsideEveryRoot(t *testing.T) {
	t.Parallel()
	for name, roots := range map[string]string{
		"root configured": "work\n",
		"no roots left":   "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := gitfixture.New(t)
			f.Clone(f.Remote("api"), "work/repos/acme/api")
			elsewhere := f.Path("elsewhere")
			h := newFakeHerdr(t, f)
			if roots != "" {
				roots = f.Path(roots)
			}
			h.roots(roots)
			h.snapshot(
				workspace{id: "w1", tabs: [][]pane{{{cwd: elsewhere}}}, tokens: map[string]string{"changes": "●3", "load": "90"}},
				workspace{id: "w2", tabs: [][]pane{{{cwd: elsewhere}}}, tokens: map[string]string{"repos": "2/5", "stale": "⚠1"}},
				workspace{id: "w3", tabs: [][]pane{{{cwd: elsewhere}}}, tokens: map[string]string{"load": "90"}},
				at("w4", elsewhere),
			)

			h.assertSynced(h.sync(), map[string]tokens{
				"w1": {"changes": cleared, "repos": cleared, "stale": cleared},
				"w2": {"changes": cleared, "repos": cleared, "stale": cleared},
			})
		})
	}
}

// Herdr ignores a report whose --seq is not above the last one from the
// same source, so each sync's --seq is the Unix time in milliseconds, or the
// previous sync's plus one when the clock is behind it.
func TestSyncSequencesEachSyncAfterTheLast(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	web := f.Clone(f.Remote("web"), "work/repos/acme/web")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("api", api), at("web", web))
	seqs := func() []uint64 {
		t.Helper()
		if got := h.sync(); got.code != 0 {
			t.Fatalf("sync failed: %+v", got)
		}
		var seqs []uint64
		for _, rep := range reports(t, h.calls()) {
			seqs = append(seqs, rep.seq)
		}
		return seqs
	}

	before := uint64(time.Now().UnixMilli())
	first := seqs()
	after := uint64(time.Now().UnixMilli())
	if len(first) != 2 || first[0] < before || first[0] > after || first[1] != first[0] {
		t.Errorf("want both reports sequenced with the Unix time in ms, %d to %d; got %v", before, after, first)
	}
	if second := seqs(); second[0] <= first[0] {
		t.Errorf("want the next sync sequenced after %d; got %v", first[0], second)
	}

	ahead := uint64(time.Now().Add(time.Hour).UnixMilli()) // as if the clock moved back an hour
	writeFile(t, filepath.Join(h.stateDir(), "seq"), strconv.FormatUint(ahead, 10)+"\n")
	if got, want := seqs(), []uint64{ahead + 1, ahead + 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("after %d, want %v; got %v", ahead, want, got)
	}
	if got, want := seqs(), []uint64{ahead + 2, ahead + 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("after %d, want %v; got %v", ahead+1, want, got)
	}
}

// An agent turning done or idle may have left changes behind, so sync runs;
// any other agent status leaves Herdr alone. Other hooks always sync.
func TestSyncOnAnAgentStatusChangeRunsOnlyWhenTheAgentIsDoneOrIdle(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("w1", api))
	agentStatus := func(status string) []string {
		return []string{"HERDR_PLUGIN_EVENT=pane.agent_status_changed",
			`HERDR_PLUGIN_EVENT_JSON={"event":"pane_agent_status_changed","data":{"type":"pane_agent_status_changed",` +
				`"pane_id":"w1:p1","workspace_id":"w1","agent_status":"` + status + `","agent":"claude"}}`}
	}

	for _, tc := range []struct {
		name  string
		env   []string
		syncs bool
	}{
		{"agent done", agentStatus("done"), true},
		{"agent idle", agentStatus("idle"), true},
		{"agent working", agentStatus("working"), false},
		{"agent blocked", agentStatus("blocked"), false},
		{"agent unknown", agentStatus("unknown"), false},
		{"workspace focused", []string{"HERDR_PLUGIN_EVENT=workspace.focused",
			`HERDR_PLUGIN_EVENT_JSON={"event":"workspace_focused","data":{"type":"workspace_focused","workspace_id":"w1"}}`}, true},
		{"startup", []string{"HERDR_PLUGIN_EVENT=startup"}, true},
	} {
		got := h.sync(tc.env...)
		if got.code != 0 || got.stdout != "" || got.stderr != "" {
			t.Errorf("%s: want exit 0 and no output; got %+v", tc.name, got)
		}
		if synced := len(h.calls()) > 0; synced != tc.syncs {
			t.Errorf("%s: synced %v, want %v", tc.name, synced, tc.syncs)
		}
	}
}

// Hooks fire in bursts. A sync that finds another one running leaves a mark
// and exits at once; the running sync then runs once more, so a burst costs
// two syncs and its last trigger is not lost.
func TestSyncDuringAnotherLeavesAMarkAndTheRunningSyncRunsOnceMore(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.FetchedAgo(api, time.Minute) // nothing to fetch: each sync reports once
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("api", api))
	snapshot := []string{"api", "snapshot"}

	release := h.hold()
	wait := h.startSync()
	h.waitForCalls(1) // the first sync is running, waiting on its snapshot

	f.Write(api, "notes.txt", "todo\n") // what the burst is about
	for i := 0; i < 2; i++ {
		start := time.Now()
		got := h.sync("HERDR_PLUGIN_EVENT=workspace.focused")
		if got.code != 0 || got.stdout != "" || got.stderr != "" || time.Since(start) > 5*time.Second {
			t.Errorf("sync during another: want exit 0 at once with no output; got %+v after %v", got, time.Since(start))
		}
	}
	if calls := h.peekCalls(); len(calls) != 1 {
		t.Errorf("syncs during another called herdr: %q", calls)
	}
	release()

	if got := wait(); got.code != 0 || got.stdout != "" || got.stderr != "" {
		t.Errorf("running sync: want exit 0 and no output; got %+v", got)
	}
	calls := h.calls()
	reps := reports(t, calls)
	if len(calls) != 4 || !reflect.DeepEqual(calls[0], snapshot) || !reflect.DeepEqual(calls[2], snapshot) || len(reps) != 2 {
		t.Fatalf("want two syncs, each a snapshot and a report; got %q", calls)
	}
	if reps[1].seq <= reps[0].seq || reps[1].tokens["changes"] != "●1" {
		t.Errorf("want the rerun to report the change, sequenced after the first; got %+v", reps)
	}

	// The mark is spent: the next sync runs once.
	h.sync()
	if calls := h.calls(); len(calls) != 2 {
		t.Errorf("want one snapshot and one report; got %q", calls)
	}
}

func TestSyncFailsOutsideHerdrAndWhenHerdrIsNotRunning(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	f.Clone(f.Remote("api"), "work/repos/acme/api")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")

	got := githerd(t, f.Env(), f.Path("."), "herdr", "sync")
	if want := "githerd herdr sync: HERDR_BIN_PATH is not set; run this from Herdr\n"; got.code != 2 || got.stderr != want {
		t.Errorf("outside Herdr: want exit 2 and stderr %q; got %+v", want, got)
	}

	got = h.sync() // no snapshot: the fake herdr answers as herdr does with no server
	if want := "githerd herdr sync: herdr api snapshot: herdr server is not running\n"; got.code != 1 || got.stderr != want {
		t.Errorf("Herdr not running: want exit 1 and stderr %q; got %+v", want, got)
	}
	if reps := reports(t, h.calls()); len(reps) != 0 {
		t.Errorf("want no reports; got %+v", reps)
	}
}

// A report Herdr refuses, say for a workspace closed since the snapshot, is
// named on stderr, and the other workspaces still get theirs.
func TestSyncReportsEveryOtherWorkspaceWhenHerdrRefusesOne(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("w1", api), at("w2", api), at("w3", api))
	writeFile(t, filepath.Join(h.dir, fakeClosedFile), "w2\n")

	got := h.sync()

	want := "githerd herdr sync: herdr workspace report-metadata: workspace w2 not found\n"
	if got.code != 1 || got.stderr != want {
		t.Errorf("want exit 1 and stderr %q; got %+v", want, got)
	}
	if reps := reports(t, h.calls()); len(reps) != 3 || reps[2].workspace != "w3" {
		t.Errorf("want a report for each workspace, w3 last; got %+v", reps)
	}
}

// A hanging remote must not hold up the sidebar: sync reports what the
// repos hold locally before it fetches anything. Once the fetches end, it
// reports again, under a later --seq, to the workspaces whose tokens changed.
func TestSyncReportsLocalStateBeforeFetching(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	f.FakeSSH()
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Write(api, "notes.txt", "todo\n")
	f.Git(api, "remote", "set-url", "origin", "ssh://git@"+gitfixture.HangingHost+"/acme/api.git")
	docsRemote := f.Remote("docs")
	docs := f.Clone(docsRemote, "work/repos/acme/docs")
	f.PushElsewhere(docsRemote, "main", 1) // behind once fetched
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("root", f.Path("work")), at("api", api), at("docs", docs))

	wait := h.startSync()
	h.waitForCalls(4) // the snapshot and a report to each workspace
	for _, call := range f.SSHCalls() {
		if strings.HasPrefix(call, "outlived the fetch") {
			t.Errorf("the first reports waited for api's hanging remote: %s", call)
		}
	}
	if got := wait(); got.code != 0 || got.stdout != "" || got.stderr != "" {
		t.Errorf("want exit 0 and no output; got %+v", got)
	}

	assertRounds(t, h.calls(),
		map[string]tokens{ // before fetching
			"root": {"repos": "1/2", "changes": cleared, "stale": cleared},
			"api":  {"changes": "●1", "repos": cleared, "stale": cleared},
			"docs": {"changes": cleared, "repos": cleared, "stale": cleared},
		},
		map[string]tokens{ // api's fetch failed, docs is behind; docs's own tokens didn't change
			"root": {"repos": "2/2", "stale": "⚠1", "changes": cleared},
			"api":  {"changes": "●1", "stale": "⚠", "repos": cleared},
		},
	)
}

// A sync holds the lock through its fetches and second round: a sync
// triggered meanwhile leaves the mark and exits at once, and the running
// sync then runs once more, under a later --seq.
func TestSyncWhileAnotherFetchesLeavesAMarkForIt(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	f.FakeSSH()
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Git(api, "remote", "set-url", "origin", "ssh://git@"+gitfixture.HangingHost+"/acme/api.git")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("api", api))
	stillFetching := func(what string) {
		t.Helper()
		for _, call := range f.SSHCalls() {
			if strings.HasPrefix(call, "outlived the fetch") {
				t.Errorf("%s only after the running sync's fetch ended", what)
			}
		}
	}

	wait := h.startSync()
	h.waitForCalls(2) // the snapshot and the first round: the fetch is next
	got := h.sync("HERDR_PLUGIN_EVENT=workspace.focused")
	stillFetching("the sync during another returned")
	if got.code != 0 || got.stdout != "" || got.stderr != "" {
		t.Errorf("sync during another: want exit 0 and no output; got %+v", got)
	}
	if calls := h.peekCalls(); len(calls) != 2 {
		t.Errorf("the sync during another called herdr: %q", calls[2:])
	}
	if got := wait(); got.code != 0 || got.stdout != "" || got.stderr != "" {
		t.Errorf("running sync: want exit 0 and no output; got %+v", got)
	}

	calls := h.calls()
	if len(calls) != 5 || !reflect.DeepEqual(calls[3], []string{"api", "snapshot"}) {
		t.Fatalf("want a sync of two rounds, then one of a single round; got %q", calls)
	}
	assertRounds(t, calls,
		map[string]tokens{"api": {"changes": cleared, "repos": cleared, "stale": cleared}},
		map[string]tokens{"api": {"stale": "⚠", "changes": cleared, "repos": cleared}}, // its fetch failed
		map[string]tokens{"api": {"stale": "⚠", "changes": cleared, "repos": cleared}}, // the rerun: still stale, not fetched
	)
}

// A repo whose fetch failed stays stale, and sync fetches it again only once
// 15 minutes have passed since the failure. Until then each sync reports it
// stale from the start and leaves it alone, even if its remote is back.
func TestSyncRetriesAStaleRepoOnlyAfterTheTTL(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	f.FakeSSH() // every host refuses the connection
	apiRemote := f.Remote("api")
	api := f.Clone(apiRemote, "work/repos/acme/api")
	f.Git(api, "remote", "set-url", "origin", "ssh://git@git.example.com/acme/api.git")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("root", f.Path("work")), at("api", api))
	fresh := map[string]tokens{
		"root": {"repos": "✓1", "changes": cleared, "stale": cleared},
		"api":  {"changes": cleared, "repos": cleared, "stale": cleared},
	}
	stale := map[string]tokens{
		"root": {"repos": "✓1", "stale": "⚠1", "changes": cleared},
		"api":  {"stale": "⚠", "changes": cleared, "repos": cleared},
	}
	sync := func() [][]string {
		t.Helper()
		if got := h.sync(); got.code != 0 || got.stdout != "" || got.stderr != "" {
			t.Fatalf("want exit 0 and no output; got %+v", got)
		}
		return h.calls()
	}

	assertRounds(t, sync(), fresh, stale)

	f.Git(api, "remote", "set-url", "origin", apiRemote) // the remote is back
	f.Backdate(api, 14*time.Minute)
	assertRounds(t, sync(), stale)

	f.Backdate(api, 16*time.Minute)
	assertRounds(t, sync(), stale, fresh)
	if calls := f.SSHCalls(); len(calls) != 1 {
		t.Errorf("want one fetch over ssh, the first; got %q", calls)
	}
}

// Sync fetches repos last fetched over 15 minutes ago, so behind counts are
// fresh, and flags the repos whose fetch failed: the root workspace counts
// them, and a mapped workspace marks its own.
func TestSyncFetchesWithTheTTLAndFlagsStaleRepos(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	docsRemote := f.Remote("docs")
	docs := f.Clone(docsRemote, "work/repos/acme/docs")
	f.PushElsewhere(docsRemote, "main", 1) // never fetched since it was cloned: fetched now, so behind
	billingRemote := f.Remote("billing")
	billing := f.Clone(billingRemote, "work/repos/acme/billing")
	f.FetchedAgo(billing, 5*time.Minute)
	f.PushElsewhere(billingRemote, "main", 1) // fetched within the TTL: not fetched again
	webRemote := f.Remote("web")
	web := f.Clone(webRemote, "work/repos/acme/web")
	rename(t, webRemote, webRemote+".moved") // its fetch fails
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("root", f.Path("work")), at("docs", docs), at("billing", billing), at("web", web))

	if got := h.sync(); got.code != 0 || got.stdout != "" || got.stderr != "" {
		t.Fatalf("want exit 0 and no output; got %+v", got)
	}
	assertRounds(t, h.calls(),
		map[string]tokens{ // before fetching
			"root":    {"repos": "✓3", "changes": cleared, "stale": cleared},
			"docs":    {"changes": cleared, "repos": cleared, "stale": cleared},
			"billing": {"changes": cleared, "repos": cleared, "stale": cleared},
			"web":     {"changes": cleared, "repos": cleared, "stale": cleared},
		},
		map[string]tokens{ // after
			"root": {"repos": "1/3", "stale": "⚠1", "changes": cleared},
			"web":  {"stale": "⚠", "changes": cleared, "repos": cleared},
		},
	)
}

// A workspace maps through the cwd of the first pane listed for its first
// tab. The focused pane, other tabs, and the cwd an agent moved its own
// process to don't move the workspace.
func TestSyncMapsAWorkspaceThroughTheFirstPaneOfItsFirstTab(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Write(api, "notes.txt", "todo\n")
	web := f.Clone(f.Remote("web"), "work/repos/acme/web")
	f.Write(web, "a.txt", "a\n")
	f.Write(web, "b.txt", "b\n")
	docs := f.Clone(f.Remote("docs"), "work/repos/acme/docs")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(
		workspace{id: "w1", tabs: [][]pane{
			{{cwd: api, foregroundCwd: web}, {cwd: web, focused: true}},
			{{cwd: docs}},
		}},
		workspace{id: "w2", tabs: [][]pane{
			{{}}, // no cwd known yet
			{{cwd: web}},
		}},
	)

	h.assertSynced(h.sync(), map[string]tokens{
		"w1": {"changes": "●1", "repos": cleared, "stale": cleared},
	})
}

// A workspace whose cwd lies anywhere in a nested repo maps to the deepest
// such repo, and gets its uncommitted count, with ∅ when it has no upstream.
// Ahead and behind are Herdr's own git_status, so they don't show here.
func TestSyncReportsEachMappedWorkspaceItsReposChanges(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	work := f.EveryState()
	f.Write(filepath.Join(work, gitfixture.NoUpstreamRepo), "scratch.txt", "notes\n")
	in := func(rel string) string { return filepath.Join(work, filepath.FromSlash(rel)) }
	h := newFakeHerdr(t, f)
	h.roots(work + "\n")
	h.snapshot(
		at("api", in(gitfixture.UncommittedRepo+"/src")),
		at("cli", in(gitfixture.NoUpstreamRepo)),
		at("experiments", in(gitfixture.NoRemoteRepo)),
		at("data", in(gitfixture.NestedRepo)),
		at("billing", in(gitfixture.AheadRepo)),
		at("web", in(gitfixture.BehindRepo)),
	)

	h.assertSynced(h.sync(), map[string]tokens{
		"api":         {"changes": "●3", "repos": cleared, "stale": cleared},
		"cli":         {"changes": "●1 ∅", "repos": cleared, "stale": cleared},
		"experiments": {"changes": "∅", "repos": cleared, "stale": cleared},
		"data":        {"changes": cleared, "repos": cleared, "stale": cleared},
		"billing":     {"changes": cleared, "repos": cleared, "stale": cleared},
		"web":         {"changes": cleared, "repos": cleared, "stale": cleared},
	})
}

func ptr[T any](v T) *T { return &v }

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
