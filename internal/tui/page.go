package tui

import (
	"fmt"
	"maps"

	"github.com/crankshift/githerd/internal/repopage"
)

// page is the state of the repo page. Its maps are never written once
// shared: a change copies them, so earlier models keep theirs.
type page struct {
	dir   string // the repo's Dir; empty while the page is closed
	repo  Repo   // its state, as the rail last read it
	label string // its name, as the page's header shows it

	loaded bool // a PageLoaded has arrived
	data   repopage.Page
	err    string // why the page could not be read

	cursor int           // index in entries of the selected one
	sel    repopage.Item // the selected entry's item, which a reload keeps selected
	top    int           // index of the first list row in view
	scroll int           // index of the first detail line in view

	gen     int                      // how many PageLoaded have arrived
	details map[repopage.Item]detail // what was read beside each item
	asked   map[repopage.Item]int    // the gen at which each item's detail was last asked for
}

// detail is what was read beside an item, during gen, or why it could not
// be.
type detail struct {
	gen  int
	read repopage.Detail
	err  string
}

// open reports whether the repo page is showing.
func (p page) open() bool { return p.dir != "" }

// openPage opens the repo page of the repo on the cursor's line, zooms the
// rail's pane to the full tab, and asks for what the page lists.
func (m Model) openPage() (Model, []Cmd) {
	lines := m.lines()
	if len(lines) == 0 {
		return m, nil
	}
	l := lines[m.cursor(lines)]
	m.page = page{dir: l.repo.Dir, repo: l.repo, label: l.label}
	if r := m.shown(); r != nil && l.repo.Dir == r.Dir {
		m.page.label = r.Name
	}
	return m, append(m.zoom(ZoomOn), LoadPage{Dir: l.repo.Dir})
}

// move moves the cursor by n entries, stopping at either end, and shows
// the new entry's detail from its top.
func (p page) move(n int) page {
	entries := p.entries()
	if len(entries) == 0 {
		return p
	}
	p.cursor = min(max(p.cursor+n, 0), len(entries)-1)
	p.sel, p.scroll = entries[p.cursor].item(), 0
	return p
}

// pageLoaded takes in what the page lists, keeping the selected entry
// selected when it is still listed.
func (m Model) pageLoaded(msg PageLoaded) Model {
	p := m.page
	if !p.open() || msg.Dir != p.dir {
		return m
	}
	p.loaded, p.data, p.err, p.gen = true, msg.Page, msg.Err, p.gen+1
	entries := p.entries()
	p.cursor = min(p.cursor, max(len(entries)-1, 0))
	for i, e := range entries {
		if e.item() == p.sel {
			p.cursor = i
		}
	}
	if len(entries) > 0 {
		p.sel = entries[p.cursor].item()
	}
	m.page = p
	return m
}

// reloadPage takes in the repo's state from a refresh of the rail, which
// reads the page again too.
func (m Model) reloadPage() Model {
	if r := m.shown(); r != nil {
		for _, repo := range r.Repos {
			if repo.Dir == m.page.dir {
				m.page.repo = repo
			}
		}
	}
	return m
}

// detailLoaded takes in what was read beside an item.
func (m Model) detailLoaded(msg DetailLoaded) Model {
	p := m.page
	if !p.open() || msg.Dir != p.dir {
		return m
	}
	p.details = maps.Clone(p.details)
	if p.details == nil {
		p.details = map[repopage.Item]detail{}
	}
	p.details[msg.Item] = detail{gen: p.asked[msg.Item], read: msg.Detail, err: msg.Err}
	m.page = p
	return m
}

// askDetail asks for what the page shows beside the selected entry, unless
// it was read, or asked for, since the page last loaded.
func (m Model) askDetail() (Model, []Cmd) {
	p := m.page
	entries := p.entries()
	if !p.open() || len(entries) == 0 {
		return m, nil
	}
	it := entries[p.cursor].item()
	if d, ok := p.details[it]; (ok && d.gen == p.gen) || p.asked[it] == p.gen {
		return m, nil
	}
	p.asked = maps.Clone(p.asked)
	if p.asked == nil {
		p.asked = map[repopage.Item]int{}
	}
	p.asked[it] = p.gen
	m.page = p
	return m, []Cmd{LoadDetail{Dir: p.dir, Item: it}}
}

// scrollDetail scrolls what shows beside the list by half its height, down
// when dir is 1 and up when it is -1, stopping at either end.
func (m Model) scrollDetail(dir int) Model {
	body := m.height - m.pageRows()
	_, rw := columns(m.width)
	last := max(len(m.page.detailLines(rw, m.now))-body, 0)
	m.page.scroll = min(max(m.page.scroll+dir*max(body/2, 1), 0), last)
	return m
}

// pageRows is how many rows of the page are not its body: the header and
// its rule, and the footer.
func (m Model) pageRows() int { return 2 + len(m.pageFooter(m.width)) }

// pageFooter is the repo page's footer w wide. The page is zoomed, wide
// enough for one line of keys.
func (m Model) pageFooter(w int) Screen { return m.footer(w, keys(w, 1, pageKeys...)) }

// settlePage scrolls the page's list as little as keeps the cursor in view
// at the height of the terminal, with the heading above it when it is the
// first of its section.
func (m Model) settlePage() Model {
	if !m.page.loaded {
		return m
	}
	rows := m.page.rows()
	cur, body := m.page.cursorRow(rows), m.height-m.pageRows()
	top := scroll(m.page.top, cur, len(rows), body)
	if top == cur && cur > 0 && rows[cur-1].entry == nil && body > 1 {
		top--
	}
	m.page.top = top
	return m
}

// row is one line of the page's list: a section's heading, a note such as
// "nothing to push", or an entry.
type row struct {
	text  []Span
	entry entry
}

// rows returns the lines of the page's list: Changes, To push, To pull,
// Branches and Log.
func (p page) rows() []row {
	var rows []row
	heading := func(text string, color Color) {
		rows = append(rows, row{text: []Span{{Text: text, Style: Style{Color: color, Bold: true, Dim: color == Default}}}})
	}
	note := func(text string) {
		rows = append(rows, row{text: []Span{{Text: "  " + text, Style: Style{Dim: true}}}})
	}
	add := func(e entry) { rows = append(rows, row{entry: e}) }
	d := p.data

	heading(fmt.Sprintf("Changes %d", changedFiles(d.Changes)), Yellow)
	for _, c := range d.Changes {
		add(changeEntry{c})
	}
	if len(d.Changes) == 0 {
		note("nothing to commit")
	}

	commits := func(in section, cs []repopage.Commit, total int, none string) {
		for _, c := range cs {
			add(commitEntry{c, in})
		}
		if more := total - len(cs); more > 0 {
			note(fmt.Sprintf("… %d more", more))
		}
		if total == 0 && len(cs) == 0 {
			note(none)
		}
	}
	push := "To push"
	if !d.Detached && d.Upstream == "" {
		push = "Local commits"
	}
	heading(fmt.Sprintf("%s %d", push, d.Ahead), Cyan)
	commits(toPush, d.ToPush, d.Ahead, "nothing to push")
	heading(fmt.Sprintf("To pull %d", d.Behind), Magenta)
	commits(toPull, d.ToPull, d.Behind, "nothing to pull")

	heading(fmt.Sprintf("Branches %d", len(d.Branches)), Blue)
	for _, b := range d.Branches {
		add(branchEntry{b})
	}
	if len(d.Branches) == 0 {
		note("no branches")
	}

	heading("Log", Default)
	commits(inLog, d.Log, len(d.Log), "no commits yet")
	return rows
}

// changedFiles counts the files that changes holds: a file changed both in
// the index and in the working tree counts once, as the rail counts it.
func changedFiles(changes []repopage.Change) int {
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Path] = true
	}
	return len(seen)
}

// entries returns the entries of the page's list, in order.
func (p page) entries() []entry {
	if !p.loaded {
		return nil
	}
	var out []entry
	for _, r := range p.rows() {
		if r.entry != nil {
			out = append(out, r.entry)
		}
	}
	return out
}

// cursorRow returns the index in rows of the selected entry's row.
func (p page) cursorRow(rows []row) int {
	n := 0
	for i, r := range rows {
		if r.entry != nil {
			if n == p.cursor {
				return i
			}
			n++
		}
	}
	return 0
}
