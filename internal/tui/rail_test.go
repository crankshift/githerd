package tui_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: roots, Refresh: true})
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

// The rail shows the root that holds the pane it was opened from, the
// deepest one when roots nest, and starts on the deepest repo holding that
// pane's cwd: the root's own repo in a root workspace. Opened outside every
// root, it shows the first root from the top.
func TestRailShowsTheRootItWasOpenedInAndStartsOnItsRepo(t *testing.T) {
	projects := tui.Root{Name: "~/projects", Dir: "/projects", Repos: []tui.Repo{
		repo("/projects/blog", status.Repo{Branch: "main", Upstream: "origin/main"}),
		repo("/projects/site", status.Repo{Branch: "main", Upstream: "origin/main", Uncommitted: 1}),
	}}
	sandbox := tui.Root{Name: "~/work/sandbox", Dir: "/work/sandbox", Repos: []tui.Repo{
		repo("/work/sandbox/experiments", status.Repo{Branch: "main", Ahead: 2}),
		repo("/work/sandbox/experiments/data", status.Repo{Branch: "main", Upstream: "origin/main"}),
	}}
	for _, c := range []struct {
		cwd            string
		root, selected string
	}{
		{"/projects/blog/posts", "~/projects", "blog"},
		{"/projects", "~/projects", "site"},
		{"/work", "~/work", ". (root)"},
		{"/work/sandbox/experiments/data/raw", "~/work/sandbox", "experiments/data"},
		{"/work/sandbox", "~/work/sandbox", "experiments"},
		{"/elsewhere/api", "~/work", "repos/acme/api"},
		{"", "~/work", "repos/acme/api"},
	} {
		m := railModel(t, c.cwd, work(), projects, sandbox)
		if got := tui.View(m, 50, 18)[1].Text(); strings.TrimSpace(got) != c.root {
			t.Errorf("opened in %q, the rail shows root %q, want %q", c.cwd, got, c.root)
		}
		if got := selected(m, 50, 18); !strings.HasPrefix(got, c.selected+" ") {
			t.Errorf("opened in %q, the rail starts on %q, want %q", c.cwd, got, c.selected)
		}
	}
}

// With no roots configured, the rail says where to list them. It keeps
// refreshing, so it picks the roots up once they are listed.
func TestRailExplainsHowToConfigureRootsWhenThereAreNone(t *testing.T) {
	m := railModel(t, "/work/repos/acme/api")
	assertView(t, m, 30, 14, "rail-no-roots")

	m, _ = tui.Update(m, tui.Loaded{At: t0, Refresh: true, Problems: []string{"roots:2: work: not an absolute path"}})
	if got, want := tui.View(m, 30, 14)[11].Text(), "⚠ roots:2: work: not an absol…"; got != want {
		t.Errorf("with a bad roots line, the line above the keys shows %q, want %q", got, want)
	}

	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(tui.RefreshEvery)})
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(31 * time.Second), Roots: []tui.Root{work()}, Refresh: true})
	if got, want := selected(m, 50, 18), "repos/acme/api"+strings.Repeat(" ", 31)+"●3 ⇡2"; got != want {
		t.Errorf("once roots are listed, the rail starts on %q, want %q", got, want)
	}
}

// header returns the first line of the rail's view at 50×18.
func header(m tui.Model) string { return tui.View(m, 50, 18)[0].Text() }

// assertCmds fails unless got are want.
func assertCmds(t *testing.T, when string, got []tui.Cmd, want ...tui.Cmd) {
	t.Helper()
	if !reflect.DeepEqual(got, want) && (len(got) > 0 || len(want) > 0) {
		t.Errorf("%s: commands %#v, want %#v", when, got, want)
	}
}

// The rail first reads the repos from disk, which is quick, then runs the
// refresh `githerd herdr sync` runs. It refreshes again 30 seconds after
// each refresh ends, never two at once, and the header counts down.
func TestRailRefreshesEveryThirtySecondsAfterTheLastRefresh(t *testing.T) {
	m, cmds := tui.New(tui.Options{Now: t0, Cwd: "/work"})
	assertCmds(t, "on open", cmds, tui.Load{})
	if got, want := strings.TrimSpace(header(m)), "loading…"; got != want {
		t.Errorf("before the first load, the header shows %q, want %q", got, want)
	}

	m, cmds = tui.Update(m, tui.Loaded{At: t0, Roots: []tui.Root{work()}})
	assertCmds(t, "after the first load", cmds, tui.Refresh{})
	if got, want := header(m), "6/10 need attention · 1 stale"+strings.Repeat(" ", 13)+"syncing…"; got != want {
		t.Errorf("during the refresh, the header shows %q, want %q", got, want)
	}

	m, cmds = tui.Update(m, tui.Loaded{At: t0.Add(2 * time.Second), Roots: []tui.Root{work()}, Refresh: true})
	assertCmds(t, "after the refresh", cmds)
	for _, tick := range []struct {
		after time.Duration
		shows string
		cmds  []tui.Cmd
	}{
		{3 * time.Second, "↻ 29s", nil},
		{31 * time.Second, "↻ 1s", nil},
		{32 * time.Second, "syncing…", []tui.Cmd{tui.Refresh{}}},
		{40 * time.Second, "syncing…", nil},
	} {
		m, cmds = tui.Update(m, tui.Tick{Now: t0.Add(tick.after)})
		assertCmds(t, fmt.Sprintf("at %v", tick.after), cmds, tick.cmds...)
		if got := header(m); !strings.HasSuffix(got, " "+tick.shows) {
			t.Errorf("at %v, the header shows %q, want it to end in %q", tick.after, got, tick.shows)
		}
	}

	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(41 * time.Second), Roots: []tui.Root{work()}, Refresh: true})
	m, cmds = tui.Update(m, tui.Tick{Now: t0.Add(50 * time.Second)})
	assertCmds(t, "9s after the second refresh", cmds)
	if got := header(m); !strings.HasSuffix(got, " ↻ 21s") {
		t.Errorf("9s after the second refresh, the header shows %q, want it to end in ↻ 21s", got)
	}
}

// dirs returns the Dir of each repo of root.
func dirs(root tui.Root) []string {
	var out []string
	for _, r := range root.Repos {
		out = append(out, r.Dir)
	}
	return out
}

// f refreshes at once and fetches every repo of the root shown, whatever
// the TTL. Pressed while a refresh or the first load runs, it waits for
// that to end, and pressing it again meanwhile adds nothing.
func TestRailForcesAFetchWithF(t *testing.T) {
	projects := tui.Root{Name: "~/projects", Dir: "/projects", Repos: []tui.Repo{
		repo("/projects/site", status.Repo{Branch: "main", Upstream: "origin/main"}),
	}}
	forced := tui.Refresh{Force: dirs(work())}

	m := railModel(t, "/work", work(), projects)
	m, cmds := press(m, "f")
	assertCmds(t, "f", cmds, forced)
	if got := header(m); !strings.HasSuffix(got, " fetching…") {
		t.Errorf("during a forced refresh, the header shows %q, want it to end in fetching…", got)
	}
	m, cmds = press(m, "f")
	assertCmds(t, "f during the forced refresh", cmds)
	m, cmds = tui.Update(m, tui.Loaded{At: t0.Add(5 * time.Second), Roots: []tui.Root{work(), projects}, Refresh: true})
	assertCmds(t, "after the forced refresh", cmds)

	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(35 * time.Second)})
	m, cmds = press(m, "f", "f")
	assertCmds(t, "f during an ordinary refresh", cmds)
	m, cmds = tui.Update(m, tui.Loaded{At: t0.Add(36 * time.Second), Roots: []tui.Root{work(), projects}, Refresh: true})
	assertCmds(t, "after the ordinary refresh", cmds, forced)
	m, cmds = tui.Update(m, tui.Loaded{At: t0.Add(40 * time.Second), Roots: []tui.Root{work(), projects}, Refresh: true})
	assertCmds(t, "after the queued forced refresh", cmds)

	m, _ = tui.New(tui.Options{Now: t0, Cwd: "/projects"})
	m, cmds = press(m, "f")
	assertCmds(t, "f before the first load", cmds)
	_, cmds = tui.Update(m, tui.Loaded{At: t0, Roots: []tui.Root{work(), projects}})
	assertCmds(t, "after the first load", cmds, tui.Refresh{Force: dirs(projects)})
}

// A refresh redraws the rail with the new state, stale marks included. The
// cursor stays on its repo when the repo moves to the other group. What went
// wrong shows above the keys until a refresh goes well.
func TestRailRedrawsAfterARefreshAndKeepsTheCursorOnItsRepo(t *testing.T) {
	m := railModel(t, "/work/repos/acme/api", work())

	after := work()
	after.Repos[1].Uncommitted, after.Repos[1].Ahead = 0, 0 // api is committed and pushed
	after.Repos = append(after.Repos[:9], repo("/work/sandbox/scratch", status.Repo{Branch: "main", Uncommitted: 1}))
	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(30 * time.Second)})
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(31 * time.Second), Roots: []tui.Root{after}, Refresh: true,
		Problems: []string{"herdr api snapshot: timed out after 10s", "/work/sandbox/old: git status: not a git repository"}})
	assertView(t, m, 50, 18, "rail-refreshed")

	after.Repos = slices.Clone(after.Repos)
	after.Repos[5].Stale = "" // web fetched at last
	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(61 * time.Second)})
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(62 * time.Second), Roots: []tui.Root{after}, Refresh: true})
	view := tui.View(m, 50, 18)
	if got, want := view[0].Text(), "6/10 need attention"+strings.Repeat(" ", 26)+"↻ 30s"; got != want {
		t.Errorf("once web was fetched, the header shows %q, want %q", got, want)
	}
	if got, want := view[len(view)-3].Text(), strings.Repeat(" ", 50); got != want {
		t.Errorf("after a refresh that went well, the line above the keys shows %q, want it blank", got)
	}
}

// q or Ctrl-C closes the rail. While a filter is typed, q is a letter of it.
func TestRailQuitsWithQOrCtrlC(t *testing.T) {
	m := railModel(t, "/work", work())

	_, cmds := press(m, "q")
	assertCmds(t, "q", cmds, tui.Quit{})
	_, cmds = press(m, tui.KeyCtrlC)
	assertCmds(t, "Ctrl-C", cmds, tui.Quit{})
	m, cmds = press(m, "/", "q")
	assertCmds(t, "q in the filter", cmds)
	_, cmds = press(m, tui.KeyCtrlC)
	assertCmds(t, "Ctrl-C in the filter", cmds, tui.Quit{})
}

// View fills exactly width × height at any size, a tiny pane included.
func TestRailViewFillsItsSizeExactly(t *testing.T) {
	m := railModel(t, "/work", work())
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: []tui.Root{work()}, Refresh: true, Problems: []string{"a problem"}})
	noRoots := railModel(t, "/work")
	loading, _ := tui.New(tui.Options{Now: t0})
	for name, m := range map[string]tui.Model{"rail": m, "no roots": noRoots, "loading": loading} {
		for w := 0; w <= 12; w++ {
			for h := 0; h <= 8; h++ {
				screen := tui.View(m, w, h)
				if len(screen) != h {
					t.Fatalf("%s at %d×%d: %d lines", name, w, h, len(screen))
				}
				for i, l := range screen {
					if got := utf8.RuneCountInString(l.Text()); got != w {
						t.Fatalf("%s at %d×%d: line %d is %d wide: %q", name, w, h, i, got, l.Text())
					}
				}
			}
		}
	}
}

// selected returns the text of the selected line of the view, without the
// blanks around it; "" when no line is selected.
func selected(m tui.Model, width, height int) string {
	for _, l := range tui.View(m, width, height) {
		if l.Selected {
			return strings.TrimSpace(l.Text())
		}
	}
	return ""
}

// listed returns the repo lines of the rail's view at 50×18, without the
// blanks around them.
func listed(m tui.Model) []string {
	screen := tui.View(m, 50, 18)
	var out []string
	for _, l := range screen[3 : len(screen)-2] {
		if text := strings.TrimSpace(l.Text()); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// / starts a filter: what is typed then keeps the repos whose path holds its
// letters in order, whatever their case. Enter keeps the filter and gives
// the keys back to the list; Esc clears it. The cursor stays on its repo.
func TestRailFiltersReposWithSlash(t *testing.T) {
	m := railModel(t, "/work/repos/acme/api", work())

	billing := "repos/acme/billing" + strings.Repeat(" ", 30) + "⇡2"
	cli := "repos/tools/cli" + strings.Repeat(" ", 31) + "∅ ⇡3"

	m, _ = press(m, "/", "a", "c", "m", "E")
	assertView(t, m, 50, 18, "rail-filter")

	m, _ = press(m, tui.KeyEnter, "j")
	if got := selected(m, 50, 18); got != billing {
		t.Errorf("j after Enter selected %q, want %q", got, billing)
	}
	if got, want := tui.View(m, 50, 18)[1].Text(), "/acmE"+strings.Repeat(" ", 36)+"esc clear"; got != want {
		t.Errorf("with the filter kept, the header shows %q, want %q", got, want)
	}

	m, _ = press(m, tui.KeyEsc)
	if got := len(listed(m)); got != 10 {
		t.Errorf("after Esc, %d repos listed, want all 10", got)
	}
	if got := selected(m, 50, 18); got != billing {
		t.Errorf("after Esc, selected %q, want %q", got, billing)
	}

	m, _ = press(m, "/", "r", "t", "c")
	if got, want := listed(m), []string{cli}; !slices.Equal(got, want) {
		t.Errorf("/rtc lists %q, want %q", got, want)
	}
	if got, want := selected(m, 50, 18), cli; got != want {
		t.Errorf("with its repo filtered out, the cursor went to %q, want the first match %q", got, want)
	}
	m, _ = press(m, "x")
	if got, want := listed(m), []string{"no repo matches"}; !slices.Equal(got, want) {
		t.Errorf("/rtcx lists %q, want %q", got, want)
	}
	m, _ = press(m, tui.KeyBackspace, tui.KeyBackspace, tui.KeyBackspace, tui.KeyBackspace, tui.KeyEsc)
	if got := len(listed(m)); got != 10 {
		t.Errorf("after Backspace and Esc, %d repos listed, want all 10", got)
	}
}

// j and k, or the arrows, move the cursor. The list scrolls only as far as
// keeps the cursor in view, and the cursor stops at either end.
func TestRailMovesTheCursorWithJKAndTheArrows(t *testing.T) {
	m := railModel(t, "/work/sandbox/experiments/data/raw", work())
	m, _ = tui.Update(m, tui.Resize{Width: 24, Height: 10})

	m, _ = press(m, "k", "k", "k", tui.KeyUp, "k", tui.KeyDown)
	assertView(t, m, 24, 10, "rail-narrow-moved")

	m, _ = press(m, "j", "j", "j", "j", "j", "j")
	if got, want := selected(m, 24, 10), "…/experiments/data     ✓"; got != want {
		t.Errorf("after j past the end, selected %q, want %q", got, want)
	}
	for range 12 {
		m, _ = press(m, "k")
	}
	if got, want := selected(m, 24, 10), "repos/acme/api     ●3 ⇡2"; got != want {
		t.Errorf("after k past the start, selected %q, want %q", got, want)
	}
}
