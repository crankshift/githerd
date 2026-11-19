// Package tui is githerd's terminal UI as a pure model: Update takes the
// model and a message, such as a key, and returns the next model and the
// commands for the terminal layer to run; View draws the model at a size.
// It does no I/O, so a test can drive it with keys and read the screen.
package tui

import (
	"path/filepath"
	"time"

	"github.com/crankshift/githerd/internal/status"
)

// RefreshEvery is how long the rail waits between refreshes.
const RefreshEvery = 30 * time.Second

// Msg is what Update takes: a Key, a Tick, a Resize, or the answer to a
// command: a Loaded, a PageLoaded, a DetailLoaded or a Failed.
type Msg any

// Key is a key the user pressed: the character it types, such as "j" or
// "/", or the name of a key that types none, such as KeyUp.
type Key string

// Keys that type no character.
const (
	KeyUp        Key = "up"
	KeyDown      Key = "down"
	KeyLeft      Key = "left"
	KeyRight     Key = "right"
	KeyEnter     Key = "enter"
	KeyEsc       Key = "esc"
	KeyBackspace Key = "backspace"
	KeyCtrlC     Key = "ctrl+c"
)

// Tick tells the model the time, about once a second.
type Tick struct{ Now time.Time }

// Resize tells the model the size of the terminal.
type Resize struct{ Width, Height int }

// Loaded is the state of the repos under every configured root, read at At.
// It answers a Load, or a Refresh when Refresh is set.
type Loaded struct {
	At       time.Time
	Roots    []Root   // in roots-file order
	Problems []string // what went wrong while reading, such as a repo git could not read
	Refresh  bool     // it answers a Refresh
	// Workspaces are the workspaces of the Herdr session, in sidebar order,
	// as the sync mapped them. WorkspacesRead is set when the sync read
	// them; without it, the rail keeps the workspaces it knew.
	Workspaces     []Workspace
	WorkspacesRead bool
}

// Created answers a CreateWorkspace: the id of the workspace Herdr created
// at Dir, or why it could not create one.
type Created struct{ Dir, ID, Err string }

// Failed answers a FocusWorkspace, a Split or a Zoom that failed: why it
// did.
type Failed struct{ Err string }

// Workspace is a Herdr workspace and where its first pane's cwd lies.
type Workspace struct {
	ID    string
	Label string
	Root  string // Dir of the root holding the cwd; empty outside every root
	Repo  string // Dir of the deepest nested repo holding it; empty for a root workspace
}

// Root is a configured root and its repos.
type Root struct {
	Name  string // as the rail shows it, such as ~/work
	Dir   string // absolute, with symlinks resolved
	Repos []Repo // in discovery order: the root first when it is a repo
}

// Repo is the state of one repo under a root. Its Dir is absolute, with
// symlinks resolved.
type Repo = status.Repo

// Cmd is a request the terminal layer carries out for the model.
type Cmd interface{ cmd() }

// Load asks for the state of the repos as it is on disk, without fetching
// or syncing; the answer is a Loaded.
type Load struct{}

// Refresh asks for the refresh `githerd herdr sync` runs: fetch the repos
// last fetched longer ago than the TTL, and those in Force whatever the TTL,
// then report the tokens. The answer is a Loaded with Refresh set.
type Refresh struct{ Force []string }

// Quit asks the terminal layer to restore the terminal and exit.
type Quit struct{}

// FocusWorkspace asks Herdr to focus workspace ID: `herdr workspace focus
// ID`.
type FocusWorkspace struct{ ID string }

// CreateWorkspace asks Herdr to create a workspace at Dir, labelled Label,
// and focus it: `herdr workspace create --cwd Dir --label Label --focus`.
type CreateWorkspace struct{ Dir, Label string }

// Split asks Herdr to split Pane, the rail's own, with a shell in Dir below
// it, and focus the shell: `herdr pane split --pane Pane --direction down
// --cwd Dir --focus`. With Run set, Run is then typed into the shell:
// `herdr pane run <new pane> Run`.
type Split struct{ Pane, Dir, Run string }

// Zoom asks Herdr to zoom Pane, the rail's own pane, in or out, or to
// toggle it: `herdr pane zoom --pane <Pane> --on|--off|--toggle`. It always
// names the pane, since Herdr would otherwise zoom whichever pane has focus.
type Zoom struct {
	Pane string
	Mode ZoomMode
}

// ZoomMode is what a Zoom does.
type ZoomMode string

// The zoom modes.
const (
	ZoomOn     ZoomMode = "on"
	ZoomOff    ZoomMode = "off"
	ZoomToggle ZoomMode = "toggle"
)

// LoadPage asks for what the repo page of the repo in Dir lists; the
// answer is a PageLoaded.
type LoadPage struct{ Dir string }

func (Load) cmd()            {}
func (Refresh) cmd()         {}
func (Quit) cmd()            {}
func (FocusWorkspace) cmd()  {}
func (CreateWorkspace) cmd() {}
func (Split) cmd()           {}
func (Zoom) cmd()            {}
func (LoadPage) cmd()        {}

// Options are what the rail knows when it opens.
type Options struct {
	Now time.Time
	// Cwd is the cwd of the pane the rail was opened from, absolute with
	// symlinks resolved. It picks the root to show and the repo to start on.
	Cwd       string
	RootsFile string // where the roots file is, as the no-roots guidance shows it
	// Pane is the rail's own pane, from HERDR_PANE_ID, which the requests
	// to Herdr name. Without it, the rail asks Herdr for no zoom.
	Pane    string
	Lazygit bool // lazygit is on PATH
}

// Model is the state of the UI. The zero Model is not usable; use New.
type Model struct {
	opts          Options
	now           time.Time
	width, height int

	page page // the repo page; open when its dir is set

	loaded   bool   // a Loaded has arrived
	roots    []Root // as last loaded
	root     int    // index in roots of the root shown
	problems []string
	note     string // a message for the user, shown above the keys until the next key

	workspaces     []Workspace // the session's, as the last sync that read them mapped them
	workspacesRead bool        // a sync has read them
	created        []created   // workspaces the rail created that the syncs may not have seen

	selected string // Dir of the repo on the cursor's line
	placed   bool   // the cursor was put on the start repo
	top      int    // index of the first list line in view

	filter    string // keeps the lines whose path holds its letters in order
	filtering bool   // keys go to the filter

	refreshes   int       // how many Refreshes have started
	refreshing  bool      // a Refresh is running
	forcing     bool      // the Refresh running fetches whatever the TTL
	forceQueued bool      // a forced Refresh starts when the one running ends
	next        time.Time // when the next refresh is due
}

// New returns the model of a rail that has just opened, and the commands to
// run first.
func New(opts Options) (Model, []Cmd) {
	return Model{opts: opts, now: opts.Now}, []Cmd{Load{}}
}

// Update returns the model after msg, and the commands it calls for.
func Update(m Model, msg Msg) (Model, []Cmd) {
	var cmds []Cmd
	switch msg := msg.(type) {
	case Resize:
		m.width, m.height = msg.Width, msg.Height
	case Tick:
		m.now = msg.Now
		if !m.refreshing && !m.next.IsZero() && !m.now.Before(m.next) {
			m, cmds = m.refresh(false)
		}
	case Loaded:
		m, cmds = loaded(m, msg)
		if msg.Refresh && m.page.open() {
			m, cmds = m.reloadPage(), append(cmds, LoadPage{Dir: m.page.dir})
		}
	case PageLoaded:
		m = m.pageLoaded(msg)
	case DetailLoaded:
		m = m.detailLoaded(msg)
	case Created:
		m = m.answered(msg)
	case Failed:
		m.note = "⚠ " + msg.Err
	case Key:
		if msg == KeyCtrlC {
			return m, []Cmd{Quit{}}
		}
		m.note = ""
		if m.page.open() {
			m, cmds = pageKey(m, msg)
		} else {
			m, cmds = railKey(m, msg)
		}
	}
	m, ask := m.askDetail()
	return m.settle(), append(cmds, ask...)
}

// View draws the model width columns wide and height rows high.
func View(m Model, width, height int) Screen {
	if m.page.open() {
		return pageView(m, width, height)
	}
	return railView(m, width, height)
}

// zoom returns the request to zoom the rail's own pane as mode says, or
// none when the rail doesn't know its pane.
func (m Model) zoom(mode ZoomMode) []Cmd {
	if m.opts.Pane == "" {
		return nil
	}
	return []Cmd{Zoom{Pane: m.opts.Pane, Mode: mode}}
}

// loaded takes in the state in msg and starts a refresh when it answered
// the first load.
func loaded(m Model, msg Loaded) (Model, []Cmd) {
	m.loaded, m.roots, m.problems, m.now = true, msg.Roots, msg.Problems, msg.At
	if msg.WorkspacesRead {
		m.workspaces, m.workspacesRead = msg.Workspaces, true
		m.created = m.unseen()
	}
	m.root = pickRoot(m.roots, m.opts.Cwd)
	if !m.placed {
		if r := m.shown(); r != nil && len(r.Repos) > 0 {
			m.selected, m.placed = startRepo(*r, m.opts.Cwd), true
		}
	}
	if !msg.Refresh {
		return m.refresh(m.forceQueued) // the first load is in: now refresh
	}
	m.refreshing, m.forcing, m.next = false, false, msg.At.Add(RefreshEvery)
	if m.forceQueued {
		return m.refresh(true)
	}
	return m, nil
}

// refresh starts a refresh. A forced one fetches every repo of the root
// shown, whatever the TTL.
func (m Model) refresh(force bool) (Model, []Cmd) {
	m.refreshing, m.forcing, m.forceQueued = true, force, false
	m.refreshes++
	var r Refresh
	if root := m.shown(); force && root != nil {
		for _, repo := range root.Repos {
			r.Force = append(r.Force, repo.Dir)
		}
	}
	return m, []Cmd{r}
}

// forceFetch starts a forced refresh, or has one start when the load or
// refresh running ends. A forced refresh running already is enough.
func (m Model) forceFetch() (Model, []Cmd) {
	if m.loaded && !m.refreshing {
		return m.refresh(true)
	}
	if !m.forcing {
		m.forceQueued = true
	}
	return m, nil
}

// shown returns the root the rail shows, or nil when no roots are
// configured.
func (m Model) shown() *Root {
	if m.root < 0 || m.root >= len(m.roots) {
		return nil
	}
	return &m.roots[m.root]
}

// pickRoot returns the index of the deepest root that contains cwd, or of
// the first root when none does.
func pickRoot(roots []Root, cwd string) int {
	best := 0
	for i, r := range roots {
		if within(cwd, r.Dir) && (!within(cwd, roots[best].Dir) || len(r.Dir) > len(roots[best].Dir)) {
			best = i
		}
	}
	return best
}

// startRepo returns the Dir of the deepest repo of root that contains cwd,
// the root's own repo included, or "" when none does.
func startRepo(root Root, cwd string) string {
	best := ""
	for _, r := range root.Repos {
		if within(cwd, r.Dir) && len(r.Dir) > len(best) {
			best = r.Dir
		}
	}
	return best
}

// within reports whether path is dir or lies under it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}
