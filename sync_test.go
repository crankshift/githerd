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
	f := gitfixture.New(t)
	site := f.Clone(f.Remote("site"), "home/projects/site")
	f.Write(site, "index.html", "<h1>hi</h1>\n")
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Clone(f.Remote("old"), "attic/old")
	h := newFakeHerdr(t, f)
	h.roots("# Where my clones live\n" +
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
	for name, roots := range map[string]*string{
		"no roots file":    nil,
		"only comments":    ptr("# ~/work\n\n   \n"),
		"empty roots file": ptr(""),
	} {
		t.Run(name, func(t *testing.T) {
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
	for name, roots := range map[string]string{
		"root configured": "work\n",
		"no roots left":   "",
	} {
		t.Run(name, func(t *testing.T) {
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
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
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

// Sync fetches repos last fetched over 15 minutes ago, so behind counts are
// fresh, and flags the repos whose fetch failed: the root workspace counts
// them, and a mapped workspace marks its own.
func TestSyncFetchesWithTheTTLAndFlagsStaleRepos(t *testing.T) {
	f := gitfixture.New(t)
	docsRemote := f.Remote("docs")
	docs := f.Clone(docsRemote, "work/repos/acme/docs")
	f.PushElsewhere(docsRemote, "main", 1) // never fetched since the clone: fetched now, so behind
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

	h.assertSynced(h.sync(), map[string]tokens{
		"root":    {"repos": "1/3", "stale": "⚠1", "changes": cleared},
		"docs":    {"changes": cleared, "repos": cleared, "stale": cleared},
		"billing": {"changes": cleared, "repos": cleared, "stale": cleared},
		"web":     {"stale": "⚠", "changes": cleared, "repos": cleared},
	})
}

// A workspace maps through the cwd of the first pane listed for its first
// tab. The focused pane, other tabs, and the cwd an agent moved its own
// process to don't move the workspace.
func TestSyncMapsAWorkspaceThroughTheFirstPaneOfItsFirstTab(t *testing.T) {
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
