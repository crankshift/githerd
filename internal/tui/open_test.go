package tui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/tui"
)

// railPane is the rail's own pane in these tests.
const railPane = "w1:p9"

// session is the workspaces of a Herdr session around work(), in sidebar
// order, as a sync maps them.
func session() []tui.Workspace {
	return []tui.Workspace{
		{ID: "w1", Label: "work", Root: "/work"},
		{ID: "w2", Label: "api", Root: "/work", Repo: "/work/repos/acme/api"},
		{ID: "w3", Label: "api-review", Root: "/work", Repo: "/work/repos/acme/api"},
		{ID: "w4", Label: "web", Root: "/work", Repo: "/work/repos/acme/web"},
		{ID: "w5", Label: "notes"},
		{ID: "w6", Label: "data", Root: "/work", Repo: "/work/sandbox/experiments/data"},
	}
}

// synced is a rail opened from cwd with opts, on work(), that has finished
// its first refresh, whose sync mapped the workspaces of the session.
func synced(t *testing.T, cwd string, opts tui.Options, workspaces []tui.Workspace) tui.Model {
	t.Helper()
	opts.Now, opts.Cwd = t0, cwd
	m, _ := tui.New(opts)
	m, _ = tui.Update(m, tui.Resize{Width: 50, Height: 18})
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: []tui.Root{work()}})
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: []tui.Root{work()}, Refresh: true, Workspaces: workspaces, WorkspacesRead: true})
	return m
}

// Each rail line names the workspaces mapped to its repo after the state
// glyphs, in sidebar order. A root workspace maps to no repo line, and a
// workspace outside every root shows nowhere. Where the names don't fit
// beside the repo's own name, they are left out.
func TestRailLinesNameTheWorkspacesMappedToTheirRepo(t *testing.T) {
	m := synced(t, "/work/repos/acme/api", tui.Options{Pane: railPane}, session())
	assertView(t, m, 50, 18, "rail-workspaces")

	m, _ = tui.Update(m, tui.Resize{Width: 24, Height: 16})
	assertView(t, m, 24, 16, "rail-workspaces-narrow")
}

// Each sync's snapshot renames the lines: workspaces created, closed or
// moved in Herdr show up at the next refresh. A refresh that could not read
// the workspaces, as when the snapshot failed, keeps those it had.
func TestRailLinesFollowTheWorkspacesOfEachSync(t *testing.T) {
	m := synced(t, "/work/repos/acme/api", tui.Options{Pane: railPane}, session())
	api := "repos/acme/api" + strings.Repeat(" ", 26) + "●3 ⇡2 ⧉api"
	billing := "repos/acme/billing" + strings.Repeat(" ", 21) + "⇡2 ⧉billing"

	after := []tui.Workspace{
		{ID: "w2", Label: "api", Root: "/work", Repo: "/work/repos/acme/api"},
		{ID: "w7", Label: "billing", Root: "/work", Repo: "/work/repos/acme/billing"},
	}
	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(30 * time.Second)})
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(31 * time.Second), Roots: []tui.Root{work()}, Refresh: true, Workspaces: after, WorkspacesRead: true})
	if got := listed(m); got[0] != api || got[1] != billing {
		t.Errorf("after a sync, the rail lists %q, want %q then %q", got, api, billing)
	}

	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(61 * time.Second)})
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(62 * time.Second), Roots: []tui.Root{work()}, Refresh: true})
	if got := listed(m); got[0] != api || got[1] != billing {
		t.Errorf("after a refresh without the workspaces, the rail lists %q, want %q then %q", got, api, billing)
	}
}

// o on a repo focuses the workspace that maps to it, the first in sidebar
// order when several do. On the root's own repo, that is a root workspace.
func TestOFocusesTheWorkspaceThatMapsToTheRepo(t *testing.T) {
	for _, c := range []struct {
		cwd, workspace string
	}{
		{"/work/repos/acme/api", "w2"},
		{"/work/repos/acme/web/src", "w4"},
		{"/work/sandbox/experiments/data", "w6"},
		{"/work", "w1"},
	} {
		m := synced(t, c.cwd, tui.Options{Pane: railPane}, session())
		_, cmds := press(m, "o")
		assertCmds(t, "o on the repo of "+c.cwd, cmds, tui.FocusWorkspace{ID: c.workspace})
	}
}

// o on a repo that no workspace maps to creates one at the repo, labelled
// with the repo's name, and focuses it. A workspace outside every root, or
// one mapped to a repo nested in this one, doesn't count.
func TestOCreatesAWorkspaceAtTheRepoWhenNoneMapsToIt(t *testing.T) {
	for _, c := range []struct {
		cwd, label string
	}{
		{"/work/repos/acme/billing", "billing"},
		{"/work/sandbox/experiments", "experiments"},
		{"/work", "work"},
	} {
		workspaces := session()[1:] // without the root workspace
		m := synced(t, c.cwd, tui.Options{Pane: railPane}, workspaces)
		_, cmds := press(m, "o")
		assertCmds(t, "o on the repo of "+c.cwd, cmds, tui.CreateWorkspace{Dir: c.cwd, Label: c.label})
	}
}

// The rail knows a workspace it created from the moment it asks for it, so
// o never creates a second one: o waits while Herdr creates it, then
// focuses it. A sync that started before Herdr created it doesn't make the
// rail forget it; the next sync's snapshot is the truth again.
func TestOFocusesTheWorkspaceItCreatedRatherThanCreatingASecond(t *testing.T) {
	const billing = "/work/repos/acme/billing"
	line := "repos/acme/billing" + strings.Repeat(" ", 21) + "⇡2 ⧉billing"
	m := synced(t, billing, tui.Options{Pane: railPane}, session())

	m, cmds := press(m, "o")
	assertCmds(t, "o", cmds, tui.CreateWorkspace{Dir: billing, Label: "billing"})
	if got := listed(m)[1]; got != line {
		t.Errorf("while Herdr creates the workspace, the rail shows %q, want %q", got, line)
	}
	m, cmds = press(m, "o")
	assertCmds(t, "o while Herdr creates the workspace", cmds)

	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(30 * time.Second)}) // a sync starts
	m, _ = tui.Update(m, tui.Created{Dir: billing, ID: "w7"})
	m, cmds = press(m, "o")
	assertCmds(t, "o once Herdr created the workspace", cmds, tui.FocusWorkspace{ID: "w7"})
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(31 * time.Second), Roots: []tui.Root{work()}, Refresh: true, Workspaces: session(), WorkspacesRead: true})
	m, cmds = press(m, "o")
	assertCmds(t, "o after a sync that started before Herdr created the workspace", cmds, tui.FocusWorkspace{ID: "w7"})

	seen := append(session(), tui.Workspace{ID: "w7", Label: "billing", Root: "/work", Repo: billing})
	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(61 * time.Second)})
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(62 * time.Second), Roots: []tui.Root{work()}, Refresh: true, Workspaces: seen, WorkspacesRead: true})
	if got := listed(m)[1]; got != line {
		t.Errorf("once a sync saw the workspace, the rail shows %q, want %q", got, line)
	}
	m, cmds = press(m, "o")
	assertCmds(t, "o once a sync saw the workspace", cmds, tui.FocusWorkspace{ID: "w7"})

	m, _ = tui.Update(m, tui.Tick{Now: t0.Add(92 * time.Second)}) // the workspace was closed meanwhile
	m, _ = tui.Update(m, tui.Loaded{At: t0.Add(93 * time.Second), Roots: []tui.Root{work()}, Refresh: true, Workspaces: session(), WorkspacesRead: true})
	_, cmds = press(m, "o")
	assertCmds(t, "o once the workspace was closed", cmds, tui.CreateWorkspace{Dir: billing, Label: "billing"})
}

// When Herdr can't carry out o, s or L, the line above the keys says why.
// o then creates the workspace again.
func TestTheRailSaysWhyHerdrCouldNotOpenTheRepo(t *testing.T) {
	const billing = "/work/repos/acme/billing"
	m := synced(t, billing, tui.Options{Pane: railPane}, session())

	m, _ = press(m, "o")
	m, _ = tui.Update(m, tui.Created{Dir: billing, Err: "herdr workspace create: no such directory"})
	if got, want := noteLine(m), "⚠ herdr workspace create: no such directory"; got != want {
		t.Errorf("after a failed create, the line above the keys shows %q, want %q", got, want)
	}
	if got, want := listed(m)[1], "repos/acme/billing"+strings.Repeat(" ", 30)+"⇡2"; got != want {
		t.Errorf("after a failed create, the rail shows %q, want %q", got, want)
	}
	m, cmds := press(m, "o")
	assertCmds(t, "o after a failed create", cmds, tui.CreateWorkspace{Dir: billing, Label: "billing"})

	m, _ = tui.Update(m, tui.Failed{Err: "herdr pane split: pane w1:p9 not found"})
	if got, want := noteLine(m), "⚠ herdr pane split: pane w1:p9 not found"; got != want {
		t.Errorf("after a failed split, the line above the keys shows %q, want %q", got, want)
	}
}

// The keys line names Enter, o and s. While a filter is typed, o, s and L
// are letters of it.
func TestTheRailShowsItsOpenKeysAndTheFilterTakesTheirLetters(t *testing.T) {
	m := synced(t, "/work/repos/acme/api", tui.Options{Pane: railPane, Lazygit: true}, session())
	if got, want := tui.View(m, 50, 18)[17].Text(), "enter page / filter o ws s shell f fetch q quit"+strings.Repeat(" ", 3); got != want {
		t.Errorf("the keys line shows %q, want %q", got, want)
	}

	m, cmds := press(m, "/", "o", "s", "L")
	assertCmds(t, "o, s and L in the filter", cmds)
	if got, want := strings.TrimSpace(tui.View(m, 50, 18)[1].Text()), "/osL▏ enter keep · esc clear"; !strings.HasPrefix(got, "/osL▏") {
		t.Errorf("after typing o, s and L in the filter, the header shows %q, want %q", got, want)
	}
}

// Until a sync has read the workspaces, o can't tell whether one maps to
// the repo, so it opens nothing rather than risk a duplicate, and says so.
func TestOWaitsForTheFirstSyncToKnowTheWorkspaces(t *testing.T) {
	m, _ := tui.New(tui.Options{Now: t0, Cwd: "/work/repos/acme/billing", Pane: railPane})
	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: []tui.Root{work()}})

	m, cmds := press(m, "o")
	assertCmds(t, "o before the first sync", cmds)
	if got, want := noteLine(m), "workspaces not known until the sync ends"; got != want {
		t.Errorf("after o before the first sync, the line above the keys shows %q, want %q", got, want)
	}

	m, _ = tui.Update(m, tui.Loaded{At: t0, Roots: []tui.Root{work()}, Refresh: true, Workspaces: session(), WorkspacesRead: true})
	_, cmds = press(m, "o")
	assertCmds(t, "o after the first sync", cmds, tui.CreateWorkspace{Dir: "/work/repos/acme/billing", Label: "billing"})
}

// s opens a shell in the selected repo, in a split of the rail's own pane.
func TestSOpensAShellInTheRepoBesideTheRail(t *testing.T) {
	m := synced(t, "/work/repos/acme/api", tui.Options{Pane: railPane}, session())

	_, cmds := press(m, "s")
	assertCmds(t, "s on api", cmds, tui.Split{Pane: railPane, Dir: "/work/repos/acme/api"})
	_, cmds = press(m, "j", "s")
	assertCmds(t, "s on billing", cmds, tui.Split{Pane: railPane, Dir: "/work/repos/acme/billing"})
}

// noteLine returns the line above the rail's keys at 50×18, without the
// blanks around it.
func noteLine(m tui.Model) string {
	return strings.TrimSpace(tui.View(m, 50, 18)[15].Text())
}

// L opens lazygit in the selected repo, in a shell split beside the rail,
// when lazygit is on PATH. Without it, L opens nothing, and the line above
// the keys says why until the next key.
func TestLOpensLazygitInTheRepoWhenItIsOnPath(t *testing.T) {
	m := synced(t, "/work/repos/acme/api", tui.Options{Pane: railPane, Lazygit: true}, session())
	_, cmds := press(m, "L")
	assertCmds(t, "L with lazygit", cmds, tui.Split{Pane: railPane, Dir: "/work/repos/acme/api", Run: "lazygit"})

	m = synced(t, "/work/repos/acme/api", tui.Options{Pane: railPane}, session())
	m, cmds = press(m, "L")
	assertCmds(t, "L without lazygit", cmds)
	if got, want := noteLine(m), "lazygit is not on PATH"; got != want {
		t.Errorf("after L without lazygit, the line above the keys shows %q, want %q", got, want)
	}
	m, _ = press(m, "j")
	if got := noteLine(m); got != "" {
		t.Errorf("after the next key, the line above the keys shows %q, want it blank", got)
	}
}
