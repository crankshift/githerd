package main_test

import (
	"path/filepath"
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
