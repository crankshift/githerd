package tui_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/status"
	"github.com/crankshift/githerd/internal/tui"
)

var update = flag.Bool("update", false, "rewrite the golden files from View")

// t0 is when the rail opens in these tests.
var t0 = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

// work is a root holding a repo in each state, in discovery order: the root
// itself, clean, first.
func work() tui.Root {
	return tui.Root{Name: "~/work", Dir: "/work", Repos: []tui.Repo{
		repo("/work", status.Repo{Branch: "main", Upstream: "origin/main"}),
		repo("/work/repos/acme/api", status.Repo{Branch: "feature/login", Upstream: "origin/feature/login", Uncommitted: 3, Ahead: 2}),
		repo("/work/repos/acme/billing", status.Repo{Branch: "main", Upstream: "origin/main", Ahead: 2}),
		repo("/work/repos/acme/docs", status.Repo{Branch: "main", Upstream: "origin/main"}),
		repo("/work/repos/acme/mobile", status.Repo{Branch: "main", Upstream: "origin/main", Ahead: 1, Behind: 2}),
		stale(repo("/work/repos/acme/web", status.Repo{Branch: "main", Upstream: "origin/main", Behind: 4})),
		repo("/work/repos/tools/cli", status.Repo{Branch: "spike/parser", Ahead: 3}),
		repo("/work/repos/tools/lint-rules", status.Repo{Detached: "3f2a9c1"}),
		repo("/work/sandbox/experiments", status.Repo{Branch: "main", Ahead: 2}),
		repo("/work/sandbox/experiments/data", status.Repo{Branch: "main", Upstream: "origin/main"}),
	}}
}

func repo(dir string, r status.Repo) tui.Repo {
	r.Dir = dir
	return tui.Repo{Repo: r}
}

func stale(r tui.Repo) tui.Repo {
	r.Stale = "ssh: connect to host git.example.com: Connection refused"
	return r
}

// railModel is a rail opened from cwd that has loaded roots and finished its
// first refresh, at t0.
func railModel(t *testing.T, cwd string, roots ...tui.Root) tui.Model {
	t.Helper()
	m, _ := tui.New(tui.Options{Now: t0, Cwd: cwd, RootsFile: "~/.config/herdr/plugins/config/githerd/roots"})
	m, _ = tui.Update(m, tui.Resize{Width: 50, Height: 18})
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: roots})
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: roots, Refresh: true, Synced: true})
	return m
}

// press sends each key to the model in turn and returns the commands of the
// last one.
func press(m tui.Model, keys ...tui.Key) (tui.Model, []tui.Cmd) {
	var cmds []tui.Cmd
	for _, k := range keys {
		m, cmds = tui.Update(m, k)
	}
	return m, cmds
}

// assertView fails unless View at width and height matches the golden file
// testdata/name.golden. With -update, it rewrites the file instead.
func assertView(t *testing.T, m tui.Model, width, height int, name string) {
	t.Helper()
	got := tui.View(m, width, height).String()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("View(%d×%d) does not match %s\n--- got ---\n%s--- want ---\n%s", width, height, path, got, want)
	}
}

// The header counts the repos that need attention and the stale ones, and
// counts down to the next refresh. Repos that need attention come first,
// with their state glyphs; clean repos are dimmed below them. Each group
// keeps the discovery order.
func TestRailListsReposThatNeedAttentionFirstAndCleanReposDimmedBelow(t *testing.T) {
	m := railModel(t, "/work/repos/acme/api", work())
	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(7 * time.Second)})

	assertView(t, m, 50, 18, "rail-wide")
}

// A narrow rail shortens the header and the keys, and cuts long paths from
// the left, by whole directories where it can, so the repo's name stays. It
// starts on the repo the rail was opened in, scrolled into view.
func TestRailFitsANarrowPane(t *testing.T) {
	m := railModel(t, "/work/sandbox/experiments/data/raw", work())

	assertView(t, m, 24, 10, "rail-narrow")
}
