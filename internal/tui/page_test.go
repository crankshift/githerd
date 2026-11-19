package tui_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/crankshift/githerd/internal/repopage"
	"github.com/crankshift/githerd/internal/tui"
)

// paneRail is a rail in its pane railPane, opened from cwd, that has loaded
// roots and finished its first refresh, at t0, 100×24.
func paneRail(t *testing.T, cwd string, roots ...tui.Root) tui.Model {
	t.Helper()
	m, _ := tui.New(tui.Options{Now: t0, Cwd: cwd, Pane: railPane, RootsFile: "~/.config/herdr/plugins/config/githerd/roots"})
	m, _ = tui.Update(m, tui.Resize{Width: 100, Height: 24})
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: roots})
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: roots, Refresh: true})
	return m
}

// Enter, l or → on a repo opens its repo page: it asks Herdr to zoom the
// rail's own pane, named, since Herdr would otherwise zoom whichever pane
// has focus, and asks for what the page lists. Esc, h or ← go back to the
// rail with the cursor where it was, and unzoom.
func TestEnterOpensTheRepoPageZoomedAndEscGoesBack(t *testing.T) {
	billing := "repos/acme/billing" + strings.Repeat(" ", 30) + "⇡2"
	for _, open := range []tui.Key{tui.KeyEnter, "l", tui.KeyRight} {
		for _, back := range []tui.Key{tui.KeyEsc, "h", tui.KeyLeft} {
			m := paneRail(t, "/work/repos/acme/api", work())
			m, _ = tui.Update(m, tui.Resize{Width: 50, Height: 18})
			m, _ = press(m, "j")

			m, cmds := press(m, open)
			assertCmds(t, string(open), cmds, tui.Zoom{Pane: railPane, Mode: tui.ZoomOn}, tui.LoadPage{Dir: "/work/repos/acme/billing"})
			if got := tui.View(m, 50, 18)[0].Text(); !strings.HasPrefix(got, "repos/acme/billing  main → origin/main  ⇡2") {
				t.Errorf("after %s, the page's header shows %q, want the repo, its branch and its state", open, got)
			}

			m, cmds = press(m, back)
			assertCmds(t, string(back), cmds, tui.Zoom{Pane: railPane, Mode: tui.ZoomOff})
			if got := selected(m, 50, 18); got != billing {
				t.Errorf("after %s, the rail is on %q, want %q", back, got, billing)
			}
		}
	}
}

// z toggles the zoom of the rail's own pane, from the rail or the repo
// page. In the filter, z is a letter of it.
func TestZTogglesTheZoomOfTheRailsPane(t *testing.T) {
	m := paneRail(t, "/work/repos/acme/api", work())
	toggle := tui.Zoom{Pane: railPane, Mode: tui.ZoomToggle}

	_, cmds := press(m, "z")
	assertCmds(t, "z on the rail", cmds, toggle)
	m, _ = press(m, tui.KeyEnter)
	_, cmds = press(m, "z")
	assertCmds(t, "z on the repo page", cmds, toggle)
	m, _ = press(m, tui.KeyEsc, "/")
	_, cmds = press(m, "z")
	assertCmds(t, "z in the filter", cmds)
}

// Without its own pane, from HERDR_PANE_ID, the rail asks Herdr for no
// zoom: Herdr would zoom whichever pane has focus.
func TestTheRailAsksForNoZoomWithoutItsPane(t *testing.T) {
	m := railModel(t, "/work/repos/acme/api", work())

	m, cmds := press(m, tui.KeyEnter)
	assertCmds(t, "Enter", cmds, tui.LoadPage{Dir: "/work/repos/acme/api"})
	m, cmds = press(m, "z")
	assertCmds(t, "z", cmds)
	_, cmds = press(m, tui.KeyEsc)
	assertCmds(t, "Esc", cmds)
}

// commit is a commit by Ada made ago before t0, with a made-up full hash
// that starts with short.
func commit(short, subject string, ago time.Duration) repopage.Commit {
	return repopage.Commit{Hash: short + strings.Repeat("0", 40-len(short)), Short: short, Author: "Ada Example", When: t0.Add(-ago), Subject: subject}
}

// apiDir is the busy repo of these tests: uncommitted and ahead.
const apiDir = "/work/repos/acme/api"

// fetchedWork is work() with api last fetched 4 minutes before t0.
func fetchedWork() tui.Root {
	r := work()
	r.Repos[1].FetchedAt = t0.Add(-4 * time.Minute)
	return r
}

// apiPage is what the repo page lists for api: a change on each side, two
// commits to push, branches in every state, and the log.
func apiPage() repopage.Page {
	return repopage.Page{
		Changes: []repopage.Change{
			{Area: repopage.Staged, Code: "M ", Path: "src/routes.go"},
			{Area: repopage.Unstaged, Code: " M", Path: "src/auth.go"},
			{Area: repopage.Untracked, Code: "??", Path: "src/auth_test.go"},
		},
		Upstream: "origin/feature/login",
		Ahead:    2,
		ToPush: []repopage.Commit{
			commit("b81e0d4", "Validate login payload", 2*time.Hour),
			commit("3f2a9c1", "Add session refresh endpoint", 5*time.Hour),
		},
		Branches: []repopage.Branch{
			{Name: "feature/login", Ref: "refs/heads/feature/login", Current: true, Upstream: "origin/feature/login", Ahead: 2},
			{Name: "fix/token-expiry", Ref: "refs/heads/fix/token-expiry"},
			{Name: "main", Ref: "refs/heads/main", Upstream: "origin/main", Behind: 3},
			{Name: "old/feature", Ref: "refs/heads/old/feature", Upstream: "origin/old/feature", Gone: true},
			{Name: "origin/release/1.9", Ref: "refs/remotes/origin/release/1.9", Remote: true},
		},
		Log: []repopage.Commit{
			commit("9c0d1e2", "Add login form", 26*time.Hour),
			commit("77a1f3b", "Set up the router", 3*24*time.Hour),
			commit("e4b2c90", "Initial commit", 40*24*time.Hour),
		},
	}
}

// routesDiff is the staged diff of api's src/routes.go.
func routesDiff() repopage.Detail {
	return repopage.Detail{Files: []repopage.FileDiff{{Path: "src/routes.go", Lines: []repopage.DiffLine{
		{Kind: repopage.Hunk, Text: "@@ -1,7 +1,8 @@"},
		{Kind: repopage.Context, Text: "package routes", Old: 1, New: 1},
		{Kind: repopage.Context, Text: "", Old: 2, New: 2},
		{Kind: repopage.Context, Text: `import "example.com/api/auth"`, Old: 3, New: 3},
		{Kind: repopage.Context, Text: "", Old: 4, New: 4},
		{Kind: repopage.Context, Text: "func Register(r *Router) {", Old: 5, New: 5},
		{Kind: repopage.Context, Text: "\tr.Post(\"/login\", auth.Login)", Old: 6, New: 6},
		{Kind: repopage.Added, Text: "\tr.Post(\"/session/refresh\", auth.Refresh)", New: 7},
		{Kind: repopage.Context, Text: "}", Old: 7, New: 8},
		{Kind: repopage.Hunk, Text: "@@ -40,3 +41,4 @@ func Register(r *Router) {"},
		{Kind: repopage.Context, Text: "func health(w Writer) {", Old: 40, New: 41},
		{Kind: repopage.Removed, Text: "\tw.Write(\"ok\")", Old: 41},
		{Kind: repopage.Added, Text: "\tw.WriteHeader(200)", New: 42},
		{Kind: repopage.Added, Text: "\tw.Write(\"ok\\n\")", New: 43},
		{Kind: repopage.Context, Text: "}", Old: 42, New: 44},
	}}}}
}

// apiPageModel is the repo page of api, zoomed to 100×24, with what it
// lists loaded.
func apiPageModel(t *testing.T) tui.Model {
	t.Helper()
	m := paneRail(t, apiDir, fetchedWork())
	m, _ = press(m, tui.KeyEnter)
	m, _ = tui.Update(m, tui.PageLoaded{Dir: apiDir, Page: apiPage()})
	return m
}

// The repo page lists the repo's changes, staged and unstaged told apart,
// its commits to push and to pull, its branches and its log on the left. It
// starts on the first line, and asks for its diff, which shows on the right
// with the old and new number of each line.
func TestRepoPageListsTheReposSectionsBesideTheDiffOfTheSelectedLine(t *testing.T) {
	m := paneRail(t, apiDir, fetchedWork())
	m, _ = press(m, tui.KeyEnter)
	m, cmds := tui.Update(m, tui.PageLoaded{Dir: apiDir, Page: apiPage()})
	routes := repopage.Item{Change: apiPage().Changes[0]}
	assertCmds(t, "once the page is loaded", cmds, tui.LoadDetail{Dir: apiDir, Item: routes})

	m, cmds = tui.Update(m, tui.DetailLoaded{Dir: apiDir, Item: routes, Detail: routesDiff()})
	assertCmds(t, "once the diff is loaded", cmds)
	assertView(t, m, 100, 24, "page-busy")
}

// j and k, or the arrows, move through the page's lines, past the
// headings, and stop at either end. Each line's detail is asked for when
// the cursor first lands on it, and only then. Beside a commit the page
// shows its message and the diff of every file it changed.
func TestRepoPageMovesThroughItsLinesAndShowsACommit(t *testing.T) {
	m := apiPageModel(t)
	validate := repopage.Item{Commit: apiPage().ToPush[0].Hash}

	m, cmds := press(m, "j", tui.KeyDown, "j")
	assertCmds(t, "on the first commit to push", cmds, tui.LoadDetail{Dir: apiDir, Item: validate})
	m, _ = tui.Update(m, tui.DetailLoaded{Dir: apiDir, Item: validate, Detail: repopage.Detail{
		Message: "Validate login payload\n\nReject an empty password before hashing it.",
		Files: []repopage.FileDiff{
			{Path: "src/auth.go", Lines: []repopage.DiffLine{
				{Kind: repopage.Hunk, Text: "@@ -12,3 +12,6 @@ func Login(user, password string) error {"},
				{Kind: repopage.Context, Text: "\tif user == \"\" {", Old: 12, New: 12},
				{Kind: repopage.Context, Text: "\t\treturn ErrNoUser", Old: 13, New: 13},
				{Kind: repopage.Context, Text: "\t}", Old: 14, New: 14},
				{Kind: repopage.Added, Text: "\tif password == \"\" {", New: 15},
				{Kind: repopage.Added, Text: "\t\treturn ErrNoPassword", New: 16},
				{Kind: repopage.Added, Text: "\t}", New: 17},
			}},
			{Path: "src/errors.go", Orig: "src/err.go", Lines: []repopage.DiffLine{
				{Kind: repopage.Meta, Text: "similarity index 100%"},
				{Kind: repopage.Meta, Text: "rename from src/err.go"},
				{Kind: repopage.Meta, Text: "rename to src/errors.go"},
			}},
		},
	}})
	assertView(t, m, 100, 24, "page-commit")

	m, cmds = press(m, tui.KeyUp)
	assertCmds(t, "back on the untracked file, asked for on the way", cmds)
	m, cmds = press(m, "k", "j", "j")
	assertCmds(t, "on the commit again", cmds)
	m, cmds = press(m, "j")
	assertCmds(t, "on the second commit to push", cmds, tui.LoadDetail{Dir: apiDir, Item: repopage.Item{Commit: apiPage().ToPush[1].Hash}})

	for range 20 {
		m, _ = press(m, "j")
	}
	if got := pageSelected(m); !strings.HasPrefix(got, "e4b2c90 Initial commit") {
		t.Errorf("after j past the end, selected %q, want the last commit of the log", got)
	}
	for range 20 {
		m, _ = press(m, "k")
	}
	if got := pageSelected(m); !strings.HasPrefix(got, "M  src/routes.go") {
		t.Errorf("after k past the start, selected %q, want the first change", got)
	}
}

// pageSelected returns the text of the selected line of the page's list at
// 100×24, without the blanks around it; "" when no line is selected.
func pageSelected(m tui.Model) string {
	for _, l := range tui.View(m, 100, 24) {
		for _, sp := range l.Spans {
			if sp.Style.Reverse {
				return strings.TrimSpace(sp.Text)
			}
		}
	}
	return ""
}

// Beside a branch the page shows its upstream, how far ahead and behind it
// is, and its recent commits.
func TestRepoPageShowsABranchAndItsRecentCommits(t *testing.T) {
	m := apiPageModel(t)
	main := repopage.Item{Branch: "refs/heads/main"}

	m, cmds := press(m, "j", "j", "j", "j", "j", "j", "j")
	assertCmds(t, "on main", cmds, tui.LoadDetail{Dir: apiDir, Item: main})
	m, _ = tui.Update(m, tui.DetailLoaded{Dir: apiDir, Item: main, Detail: repopage.Detail{Commits: []repopage.Commit{
		commit("1a2b3c4", "Bump the Go version", 3*time.Hour),
		commit("9c0d1e2", "Add login form", 26*time.Hour),
		commit("77a1f3b", "Set up the router", 3*24*time.Hour),
	}}})
	assertView(t, m, 100, 24, "page-branch")

	for _, c := range []struct {
		key  tui.Key
		says string
	}{
		{"k", "no upstream"},
		{"k", "tracks origin/feature/login  ⇡2"},
		{"j", "no upstream"},
		{"j", "tracks origin/main  ⇣3"},
		{"j", "tracks origin/old/feature, which is gone"},
		{"j", "remote branch: no local branch tracks it"},
	} {
		m, _ = press(m, c.key)
		if got := strings.TrimSpace(strings.SplitN(tui.View(m, 100, 24)[3].Text(), "│", 2)[1]); got != c.says {
			t.Errorf("beside %s, the page says %q, want %q", pageSelected(m), got, c.says)
		}
	}
}

// beside returns what the page at 100×24 shows on row i beside its list,
// without the blanks around it.
func beside(m tui.Model, i int) string {
	return strings.TrimSpace(strings.SplitN(tui.View(m, 100, 24)[i].Text(), "│", 2)[1])
}

// J and K scroll the diff by half its height, the list keeping its place,
// and stop at either end of the diff. Moving to another line shows its
// diff from the top.
func TestRepoPageScrollsTheDiffWithShiftJK(t *testing.T) {
	m := apiPageModel(t)
	long := repopage.FileDiff{Path: "src/routes.go", Lines: []repopage.DiffLine{{Kind: repopage.Hunk, Text: "@@ -1,59 +1,59 @@"}}}
	for i := 1; i <= 59; i++ {
		long.Lines = append(long.Lines, repopage.DiffLine{Kind: repopage.Context, Text: fmt.Sprintf("line %d", i), Old: i, New: i})
	}
	routes := repopage.Item{Change: apiPage().Changes[0]}
	m, _ = tui.Update(m, tui.DetailLoaded{Dir: apiDir, Item: routes, Detail: repopage.Detail{Files: []repopage.FileDiff{long}}})
	// The diff's 62 lines: the file, its side, the hunk and 59 lines; 20 show.

	for _, c := range []struct {
		keys  []tui.Key
		first string
	}{
		{[]tui.Key{"J"}, "8    8  line 8"},
		{[]tui.Key{"J", "J"}, "28   28  line 28"},
		{[]tui.Key{"J", "J"}, "40   40  line 40"},
		{[]tui.Key{"K"}, "30   30  line 30"},
		{[]tui.Key{"K", "K", "K", "K"}, "src/routes.go                                           +0 −0"},
		{[]tui.Key{"J", "j", "k"}, "src/routes.go                                           +0 −0"},
	} {
		m, _ = press(m, c.keys...)
		if got := beside(m, 2); got != c.first {
			t.Errorf("after %q, the diff starts with %q, want %q", c.keys, got, c.first)
		}
		if got := pageSelected(m); !strings.HasPrefix(got, "M  src/routes.go") {
			t.Errorf("after %q, the list is on %q, want it still on src/routes.go", c.keys, got)
		}
	}
}

// The page of a clean repo says there is nothing to commit, push or pull,
// and starts on its branch.
func TestRepoPageOfACleanRepo(t *testing.T) {
	docs := "/work/repos/acme/docs"
	m := paneRail(t, docs, fetchedWork())
	m, _ = press(m, tui.KeyEnter)
	m, cmds := tui.Update(m, tui.PageLoaded{Dir: docs, Page: repopage.Page{
		Upstream: "origin/main",
		Branches: []repopage.Branch{{Name: "main", Ref: "refs/heads/main", Current: true, Upstream: "origin/main"}},
		Log: []repopage.Commit{
			commit("5eed5ed", "Document the login flow", 2*24*time.Hour),
			commit("d15ea5e", "Fix broken links", 9*24*time.Hour),
			commit("c0ffee1", "Initial commit", 400*24*time.Hour),
		},
	}})
	main := repopage.Item{Branch: "refs/heads/main"}
	assertCmds(t, "once the page is loaded", cmds, tui.LoadDetail{Dir: docs, Item: main})
	m, _ = tui.Update(m, tui.DetailLoaded{Dir: docs, Item: main, Detail: repopage.Detail{Commits: []repopage.Commit{
		commit("5eed5ed", "Document the login flow", 2*24*time.Hour),
		commit("d15ea5e", "Fix broken links", 9*24*time.Hour),
		commit("c0ffee1", "Initial commit", 400*24*time.Hour),
	}}})
	assertView(t, m, 100, 24, "page-clean")
}

// In a pane too narrow for them, as when the page shows unzoomed, diff
// lines leave out their line numbers.
func TestRepoPageLeavesOutLineNumbersWhenNarrow(t *testing.T) {
	m := apiPageModel(t)
	m, _ = tui.Update(m, tui.DetailLoaded{Dir: apiDir, Item: repopage.Item{Change: apiPage().Changes[0]}, Detail: routesDiff()})
	m, _ = tui.Update(m, tui.Resize{Width: 60, Height: 16})

	assertView(t, m, 60, 16, "page-narrow")
}

// The page's View fills exactly width × height at any size, a tiny pane
// included, while loading, loaded, and when reading failed.
func TestRepoPageViewFillsItsSizeExactly(t *testing.T) {
	loading := paneRail(t, apiDir, fetchedWork())
	loading, _ = press(loading, tui.KeyEnter)
	loaded := apiPageModel(t)
	loaded, _ = tui.Update(loaded, tui.DetailLoaded{Dir: apiDir, Item: repopage.Item{Change: apiPage().Changes[0]}, Detail: routesDiff()})
	loaded, _ = tui.Update(loaded, tui.Loaded{At: t0, Roots: []tui.Root{fetchedWork()}, Refresh: true, Problems: []string{"a problem"}})
	failed, _ := tui.Update(loading, tui.PageLoaded{Dir: apiDir, Err: "git status: not a git repository"})
	for name, m := range map[string]tui.Model{"loading": loading, "loaded": loaded, "failed": failed} {
		for w := 0; w <= 40; w++ {
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

// While the page is open, each refresh of the rail reads the page again.
// The cursor stays on its line wherever the line moved, or keeps its place
// when the line is gone; what shows beside it stays until it is read
// again. Answers for a page no longer open are dropped.
func TestRepoPageReadsTheRepoAgainWithEachRefresh(t *testing.T) {
	m := apiPageModel(t)
	validate := repopage.Item{Commit: apiPage().ToPush[0].Hash}
	m, _ = press(m, "j", "j", "j")
	m, _ = tui.Update(m, tui.DetailLoaded{Dir: apiDir, Item: validate, Detail: repopage.Detail{Message: "Validate login payload"}})

	after := fetchedWork()
	after.Repos[1].Uncommitted = 1 // two changes were committed
	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(30 * time.Second)})
	m, cmds := tui.Update(m, tui.Loaded{At: t0.Add(31 * time.Second), Roots: []tui.Root{after}, Refresh: true})
	assertCmds(t, "after a refresh", cmds, tui.LoadPage{Dir: apiDir})
	if got := tui.View(m, 100, 24)[0].Text(); !strings.HasPrefix(got, "repos/acme/api  feature/login → origin/feature/login  ●1 ⇡2 ") {
		t.Errorf("after a refresh, the header shows %q, want the new state", got)
	}

	p := apiPage()
	p.Changes = p.Changes[:1]
	m, cmds = tui.Update(m, tui.PageLoaded{Dir: apiDir, Page: p})
	assertCmds(t, "read again", cmds, tui.LoadDetail{Dir: apiDir, Item: validate})
	if got := pageSelected(m); !strings.HasPrefix(got, "b81e0d4 Validate login payload") {
		t.Errorf("read again, the cursor is on %q, want it still on its commit", got)
	}
	if got, want := beside(m, 5), "Validate login payload"; got != want {
		t.Errorf("read again, beside the commit shows %q, want %q until it is read again", got, want)
	}

	p.ToPush, p.Ahead = p.ToPush[1:], 1 // pushed
	m, cmds = tui.Update(m, tui.PageLoaded{Dir: apiDir, Page: p})
	assertCmds(t, "with the commit gone", cmds, tui.LoadDetail{Dir: apiDir, Item: repopage.Item{Commit: p.ToPush[0].Hash}})
	if got := pageSelected(m); !strings.HasPrefix(got, "3f2a9c1 Add session refresh") {
		t.Errorf("with its commit gone, the cursor is on %q, want the line now in its place", got)
	}

	before := tui.View(m, 100, 24).String()
	m, cmds = tui.Update(m, tui.DetailLoaded{Dir: "/work/repos/acme/billing", Item: repopage.Item{Commit: p.ToPush[0].Hash}, Detail: repopage.Detail{Message: "Other"}})
	assertCmds(t, "an answer for another repo", cmds)
	if got := tui.View(m, 100, 24).String(); got != before {
		t.Errorf("an answer for another repo changed the page:\n%s", got)
	}
	m, _ = press(m, tui.KeyEsc)
	before = tui.View(m, 100, 24).String()
	m, cmds = tui.Update(m, tui.PageLoaded{Dir: apiDir, Page: apiPage()})
	assertCmds(t, "a page read after Esc", cmds)
	if got := tui.View(m, 100, 24).String(); got != before {
		t.Errorf("a page read after Esc changed the rail:\n%s", got)
	}
}

// f fetches every repo of the root from the page too, and q closes the
// rail.
func TestRepoPageFetchesWithFAndQuitsWithQ(t *testing.T) {
	m := apiPageModel(t)

	m, cmds := press(m, "f")
	assertCmds(t, "f", cmds, tui.Refresh{Force: dirs(fetchedWork())})
	if got := tui.View(m, 100, 24)[0].Text(); !strings.HasSuffix(got, " fetching…") {
		t.Errorf("during a forced refresh, the page's header shows %q, want it to end in fetching…", got)
	}
	_, cmds = press(m, "q")
	assertCmds(t, "q", cmds, tui.Quit{})
}

// What went wrong shows: reading the page in the list, reading a line's
// detail beside it, and a request to Herdr, such as a zoom, above the keys.
func TestRepoPageShowsWhatWentWrong(t *testing.T) {
	m := paneRail(t, apiDir, fetchedWork())
	m, _ = press(m, tui.KeyEnter)

	failed, _ := tui.Update(m, tui.PageLoaded{Dir: apiDir, Err: "git status: index file corrupt"})
	if got, want := strings.TrimSpace(strings.SplitN(tui.View(failed, 100, 24)[2].Text(), "│", 2)[0]), "⚠ git status: index file corrupt"; got != want {
		t.Errorf("when the page can't be read, the list shows %q, want %q", got, want)
	}

	m, _ = tui.Update(m, tui.PageLoaded{Dir: apiDir, Page: apiPage()})
	m, _ = tui.Update(m, tui.DetailLoaded{Dir: apiDir, Item: repopage.Item{Change: apiPage().Changes[0]}, Err: "git diff: bad object"})
	if got, want := beside(m, 4), "⚠ git diff: bad object"; got != want {
		t.Errorf("when a diff can't be read, beside it shows %q, want %q", got, want)
	}

	m, cmds := tui.Update(m, tui.Failed{Err: "herdr pane zoom: pane not found"})
	assertCmds(t, "a failed request", cmds)
	if got, want := strings.TrimSpace(tui.View(m, 100, 24)[21].Text()), "⚠ herdr pane zoom: pane not found"; got != want {
		t.Errorf("after a failed request, the line above the keys shows %q, want %q", got, want)
	}
}

// From the repo page, which is zoomed, s and L first unzoom the rail's
// pane: zoomed, the tab would show the page alone and hide the new split.
// They do on the rail too, which z may have zoomed.
func TestSAndLUnzoomTheRailsPaneFirst(t *testing.T) {
	m := synced(t, apiDir, tui.Options{Pane: railPane, From: fromPane, Lazygit: true}, session())
	unzoom := tui.Zoom{Pane: railPane, Mode: tui.ZoomOff}
	shell := tui.Split{From: fromPane, Rail: railPane, Dir: apiDir}
	lazygit := tui.Split{From: fromPane, Rail: railPane, Dir: apiDir, Run: "lazygit"}

	page, _ := press(m, tui.KeyEnter)
	_, cmds := press(page, "s")
	assertCmds(t, "s on the page", cmds, unzoom, shell)
	_, cmds = press(page, "L")
	assertCmds(t, "L on the page", cmds, unzoom, lazygit)
	_, cmds = press(m, "z", "s")
	assertCmds(t, "s on the rail zoomed with z", cmds, unzoom, shell)
}

// On the repo page, o, s and L act on the page's repo as they do on the
// rail, even once a refresh no longer lists it.
func TestRepoPageOpensItsRepoWithOSAndL(t *testing.T) {
	m := synced(t, "/work/repos/acme/api", tui.Options{Pane: railPane}, session())
	m, _ = press(m, "j", tui.KeyEnter) // billing's page
	billing := "/work/repos/acme/billing"

	_, cmds := press(m, "s")
	assertCmds(t, "s", cmds, tui.Zoom{Pane: railPane, Mode: tui.ZoomOff}, tui.Split{Rail: railPane, Dir: billing})
	_, cmds = press(m, "o")
	assertCmds(t, "o", cmds, tui.CreateWorkspace{Dir: billing, Label: "billing"})
	l, cmds := press(m, "L")
	assertCmds(t, "L without lazygit", cmds)
	if got, want := strings.TrimSpace(tui.View(l, 50, 18)[15].Text()), "lazygit is not on PATH"; got != want {
		t.Errorf("L without lazygit: the line above the keys shows %q, want %q", got, want)
	}

	gone := work()
	gone.Repos = slices.Delete(gone.Repos, 2, 3) // billing
	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(30 * time.Second)})
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(31 * time.Second), Roots: []tui.Root{gone}, Refresh: true, Workspaces: session(), WorkspacesRead: true})
	_, cmds = press(m, "s")
	assertCmds(t, "s once billing is no longer listed", cmds, tui.Zoom{Pane: railPane, Mode: tui.ZoomOff}, tui.Split{Rail: railPane, Dir: billing})
}
