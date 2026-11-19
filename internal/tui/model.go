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

// Msg is what Update takes: a Key, a Tick, a Resize or a Loaded.
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
	// Synced is set when the refresh ran the sync, which fetched the repos
	// due. Otherwise fetch failures are unknown, so the stale marks the rail
	// had are kept.
	Synced bool
}

// Root is a configured root and its repos.
type Root struct {
	Name  string // as the rail shows it, such as ~/work
	Dir   string // absolute, with symlinks resolved
	Repos []Repo // in discovery order: the root first when it is a repo
}

// Repo is the state of one repo under a root.
type Repo struct {
	status.Repo        // Dir is absolute, with symlinks resolved
	Stale       string // why its last fetch failed; empty when it did not
}

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

func (Load) cmd()    {}
func (Refresh) cmd() {}
func (Quit) cmd()    {}

// Options are what the rail knows when it opens.
type Options struct {
	Now time.Time
	// Cwd is the cwd of the pane the rail was opened from, absolute with
	// symlinks resolved. It picks the root to show and the repo to start on.
	Cwd       string
	RootsFile string // where the roots file is, as the no-roots guidance shows it
}

// Model is the state of the UI. The zero Model is not usable; use New.
type Model struct {
	opts   Options
	now    time.Time
	height int

	loaded   bool   // a Loaded has arrived
	roots    []Root // as last loaded
	root     int    // index in roots of the root shown
	problems []string

	selected string // Dir of the repo on the cursor's line
	placed   bool   // the cursor was put on the start repo
	top      int    // index of the first list line in view

	filter    string // keeps the lines whose path holds its letters in order
	filtering bool   // keys go to the filter

	refreshing bool      // a Refresh is running
	next       time.Time // when the next refresh is due
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
		m.height = msg.Height
	case Tick:
		m.now = msg.Now
	case Loaded:
		m, cmds = loaded(m, msg)
	case Key:
		m, cmds = railKey(m, msg)
	}
	return m.settle(), cmds
}

// View draws the model width columns wide and height rows high.
func View(m Model, width, height int) Screen {
	return railView(m, width, height)
}

// loaded takes in the state in msg and starts a refresh when it answered
// the first load.
func loaded(m Model, msg Loaded) (Model, []Cmd) {
	m.loaded, m.roots, m.problems, m.now = true, msg.Roots, msg.Problems, msg.At
	m.root = pickRoot(m.roots, m.opts.Cwd)
	if !m.placed {
		if r := m.shown(); r != nil && len(r.Repos) > 0 {
			m.selected, m.placed = startRepo(*r, m.opts.Cwd), true
		}
	}
	if msg.Refresh {
		m.refreshing, m.next = false, msg.At.Add(RefreshEvery)
		return m, nil
	}
	m.refreshing = true
	return m, []Cmd{Refresh{}}
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
