package main_test

import (
	"path/filepath"
	"testing"

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
