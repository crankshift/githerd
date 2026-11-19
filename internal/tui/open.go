package tui

import (
	"path/filepath"
	"slices"
)

// act carries out o, s or L on the selected repo: o focuses the workspace
// that maps to it or creates one, s opens a shell in it beside the rail, and
// L opens lazygit there.
func act(m Model, k Key) (Model, []Cmd) {
	repo := m.selectedRepo()
	if repo == nil {
		return m, nil
	}
	switch k {
	case "o":
		return m.open(repo.Dir)
	case "s":
		return m, []Cmd{Split{Pane: m.opts.Pane, Dir: repo.Dir}}
	case "L":
		if !m.opts.Lazygit {
			m.note = "lazygit is not on PATH"
			return m, nil
		}
		return m, []Cmd{Split{Pane: m.opts.Pane, Dir: repo.Dir, Run: "lazygit"}}
	}
	return m, nil
}

// selectedRepo returns the repo the actions act on: the repo page's while
// it is open, else the one on the rail's cursor line, or nil when no repo
// is listed.
func (m Model) selectedRepo() *Repo {
	if m.page.open() {
		repo := m.page.repo
		return &repo
	}
	lines := m.lines()
	if len(lines) == 0 {
		return nil
	}
	repo := lines[m.cursor(lines)].repo
	return &repo
}

// created is a workspace the rail asked Herdr to create, which the syncs
// may not have seen yet.
type created struct {
	Workspace        // its ID is empty until Herdr answers
	dir       string // where it was created
	answered  int    // how many refreshes had started when Herdr answered
}

// open focuses the workspace that maps to the repo at dir, or creates one
// there and knows it from then on. It waits while Herdr creates one, and
// until a sync has read the workspaces, so as never to create a duplicate.
func (m Model) open(dir string) (Model, []Cmd) {
	if !m.workspacesRead {
		m.note = "workspaces not known until the sync ends"
		return m, nil
	}
	if w := m.workspaceOf(dir); w != nil {
		if w.ID == "" {
			return m, nil
		}
		return m, []Cmd{FocusWorkspace{ID: w.ID}}
	}
	w := Workspace{Label: filepath.Base(dir), Repo: dir}
	if root := m.shown(); root != nil {
		w.Root = root.Dir
		if root.Dir == dir {
			w.Repo = "" // a root workspace
		}
	}
	m.created = append(slices.Clone(m.created), created{Workspace: w, dir: dir})
	return m, []Cmd{CreateWorkspace{Dir: dir, Label: w.Label}}
}

// answered takes in Herdr's answer to a CreateWorkspace: the new
// workspace's id, or why there is none.
func (m Model) answered(msg Created) Model {
	m.created = slices.Clone(m.created)
	for i, c := range m.created {
		if c.ID != "" || c.dir != msg.Dir {
			continue
		}
		if msg.Err != "" {
			m.note = "⚠ " + msg.Err
			m.created = slices.Delete(m.created, i, i+1)
		} else {
			m.created[i].ID, m.created[i].answered = msg.ID, m.refreshes
		}
		break
	}
	return m
}

// unseen returns the workspaces the rail created that the sync just in
// can't have seen: those Herdr has not answered for yet, and those it
// answered for after that sync started, unless the sync saw them anyway.
func (m Model) unseen() []created {
	var out []created
	for _, c := range m.created {
		seen := slices.ContainsFunc(m.workspaces, func(w Workspace) bool { return w.ID == c.ID })
		if c.ID == "" || (c.answered >= m.refreshes && !seen) {
			out = append(out, c)
		}
	}
	return out
}

// known returns every workspace the rail knows, in sidebar order: those
// of the last sync, then those it created since.
func (m Model) known() []Workspace {
	out := slices.Clone(m.workspaces)
	for _, c := range m.created {
		out = append(out, c.Workspace)
	}
	return out
}

// mappedTo returns the mapped workspaces of the repo at dir, in sidebar
// order.
func (m Model) mappedTo(dir string) []Workspace {
	var out []Workspace
	for _, w := range m.known() {
		if w.Repo == dir {
			out = append(out, w)
		}
	}
	return out
}

// workspaceOf returns the first workspace, in sidebar order, that maps to
// the repo at dir: a mapped workspace of that repo, or a root workspace of a
// root at dir. It returns nil when none does.
func (m Model) workspaceOf(dir string) *Workspace {
	for _, w := range m.known() {
		if w.Repo == dir || (w.Repo == "" && w.Root == dir) {
			return &w
		}
	}
	return nil
}
