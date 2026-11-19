package tui

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/crankshift/githerd/internal/repopage"
)

// PageLoaded is what the repo page of the repo in Dir lists, or why it
// could not be read. It answers a LoadPage.
type PageLoaded struct {
	Dir  string
	Page repopage.Page
	Err  string
}

// LoadDetail asks for what the repo page of the repo in Dir shows beside
// Item: a diff, or a branch's recent commits. The answer is a DetailLoaded.
type LoadDetail struct {
	Dir  string
	Item repopage.Item
}

// DetailLoaded is what the repo page shows beside Item, or why it could not
// be read. It answers a LoadDetail.
type DetailLoaded struct {
	Dir    string
	Item   repopage.Item
	Detail repopage.Detail
	Err    string
}

func (LoadDetail) cmd() {}

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

// detail is what was read beside an item, during gen.
type detail struct {
	gen int
	d   repopage.Detail
	err string
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

// pageKey handles a key on the repo page.
func pageKey(m Model, k Key) (Model, []Cmd) {
	switch k {
	case KeyEsc, "h", KeyLeft:
		m.page = page{}
		return m, m.zoom(ZoomOff)
	case "j", KeyDown:
		m.page = m.page.move(1)
	case "k", KeyUp:
		m.page = m.page.move(-1)
	case "J":
		m = m.scrollDetail(1)
	case "K":
		m = m.scrollDetail(-1)
	case "z":
		return m, m.zoom(ZoomToggle)
	case "f":
		return m.forceFetch()
	case "q":
		return m, []Cmd{Quit{}}
	}
	return m, nil
}

// move moves the cursor by n entries, stopping at either end, and shows
// the new entry's detail from its top.
func (p page) move(n int) page {
	entries := p.entries()
	if len(entries) == 0 {
		return p
	}
	p.cursor = min(max(p.cursor+n, 0), len(entries)-1)
	p.sel, p.scroll = entries[p.cursor].item, 0
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
		if e.item == p.sel {
			p.cursor = i
		}
	}
	if len(entries) > 0 {
		p.sel = entries[p.cursor].item
	}
	m.page = p
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
	p.details[msg.Item] = detail{gen: p.asked[msg.Item], d: msg.Detail, err: msg.Err}
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
	it := entries[p.cursor].item
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
// its rule, what went wrong if anything did, a rule and the keys.
func (m Model) pageRows() int {
	if len(m.problems) > 0 {
		return 5
	}
	return 4
}

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

// entryKind is the section an entry is listed in.
type entryKind uint8

const (
	changeEntry entryKind = iota + 1
	pushEntry
	pullEntry
	branchEntry
	logEntry
)

// entry is a line of the page with something to show beside it.
type entry struct {
	kind   entryKind
	item   repopage.Item
	change repopage.Change
	commit repopage.Commit
	branch repopage.Branch
}

// row is one line of the page's list: a section's heading, a note such as
// "nothing to push", or an entry.
type row struct {
	text  []Span
	entry *entry
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
	add := func(e entry) { rows = append(rows, row{entry: &e}) }
	d := p.data

	heading(fmt.Sprintf("Changes %d", changedFiles(d.Changes)), Yellow)
	for _, c := range d.Changes {
		add(entry{kind: changeEntry, item: repopage.Item{Change: c}, change: c})
	}
	if len(d.Changes) == 0 {
		note("nothing to commit")
	}

	commits := func(kind entryKind, cs []repopage.Commit, total int, none string) {
		for _, c := range cs {
			add(entry{kind: kind, item: repopage.Item{Commit: c.Hash}, commit: c})
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
	commits(pushEntry, d.ToPush, d.Ahead, "nothing to push")
	heading(fmt.Sprintf("To pull %d", d.Behind), Magenta)
	commits(pullEntry, d.ToPull, d.Behind, "nothing to pull")

	heading(fmt.Sprintf("Branches %d", len(d.Branches)), Blue)
	for _, b := range d.Branches {
		add(entry{kind: branchEntry, item: repopage.Item{Branch: b.Ref}, branch: b})
	}
	if len(d.Branches) == 0 {
		note("no branches")
	}

	heading("Log", Default)
	commits(logEntry, d.Log, len(d.Log), "no commits yet")
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
			out = append(out, *r.entry)
		}
	}
	return out
}

// pageView draws the repo page: a header with the repo's state, the list
// of its sections beside what the selected line shows, and the keys.
func pageView(m Model, w, h int) Screen {
	footer := Screen{rule(w), keys(w, "j/k", "move", "J/K", "scroll", "esc", "back", "z", "zoom", "f", "fetch", "q", "quit")}
	if len(m.problems) > 0 {
		footer = append(Screen{m.problemLine(w)}, footer...)
	}
	screen := Screen{m.pageHeader(w), rule(w)}
	body := max(h-len(screen)-len(footer), 0)
	lw, rw := columns(w)
	left := m.page.listLines(lw, body, m.now)
	right := m.page.detailLines(rw, m.now)
	scroll := min(m.page.scroll, max(len(right)-body, 0))
	for i := 0; i < body; i++ {
		spans := left[i]
		if rw > 0 {
			spans = append(append(spans, Span{Text: "│", Style: Style{Dim: true}}), cell(right, scroll+i, rw)...)
		}
		screen = append(screen, Line{Spans: spans})
	}
	screen = append(screen, footer...)
	return screen[:min(len(screen), h)]
}

// columns returns the widths of the page's list and of what shows beside
// it, which a one-column rule separates, in w columns.
func columns(w int) (list, detail int) {
	list = max(w*38/100, min(34, w/2))
	if w-list-1 < 1 {
		return w, 0
	}
	return list, w - list - 1
}

// cell returns line i of lines, or blanks w wide past their end.
func cell(lines [][]Span, i, w int) []Span {
	if i < len(lines) {
		return lines[i]
	}
	return fit(nil, w)
}

// listLines draws the rows of the page's list in view, height of them,
// each w wide.
func (p page) listLines(w, height int, now time.Time) [][]Span {
	var out [][]Span
	switch {
	case p.err != "":
		out = append(out, fit([]Span{{Text: "⚠ " + p.err, Style: Style{Color: Red}}}, w))
	case !p.loaded:
		out = append(out, fit([]Span{{Text: "loading…", Style: Style{Dim: true}}}, w))
	default:
		rows := p.rows()
		cur := p.cursorRow(rows)
		for i := p.top; i < len(rows) && len(out) < height; i++ {
			out = append(out, rowSpans(rows[i], i == cur, w, now))
		}
	}
	for len(out) < height {
		out = append(out, fit(nil, w))
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

// rowSpans draws one row of the list w wide: the cursor's in reverse video.
func rowSpans(r row, selected bool, w int, now time.Time) []Span {
	if r.entry == nil {
		return fit(r.text, w)
	}
	var left, right []Span
	e := r.entry
	switch e.kind {
	case changeEntry:
		c := e.change
		path := c.Path
		if c.Orig != "" {
			path = c.Orig + " → " + c.Path
		}
		left = []Span{{Text: "  "}, {Text: c.Code, Style: Style{Color: areaColor[c.Area]}}, {Text: " " + cutPath(path, w-5)}}
	case branchEntry:
		b := e.branch
		mark, name := Span{Text: "    "}, Style{}
		switch {
		case b.Current:
			mark, name = Span{Text: "  * ", Style: Style{Color: Green}}, Style{Color: Green, Bold: true}
		case b.Remote:
			name = Style{Dim: true}
		}
		left, right = []Span{mark, {Text: b.Name, Style: name}}, tracking(b)
	default:
		c := e.commit
		left = []Span{{Text: "  "}, {Text: c.Short, Style: Style{Color: commitColor[e.kind]}}, {Text: " " + c.Subject}}
		right = []Span{{Text: age(now.Sub(c.When)), Style: Style{Dim: true}}}
	}
	spans := leftRight(left, right, w)
	if selected {
		return []Span{{Text: spansText(spans), Style: Style{Reverse: true}}}
	}
	return spans
}

// areaColor is the color of a change's code: green when staged, yellow
// when unstaged, red when untracked or unmerged.
var areaColor = map[repopage.Area]Color{repopage.Staged: Green, repopage.Unstaged: Yellow, repopage.Untracked: Red, repopage.Unmerged: Red}

// commitColor is the color of a commit's hash: cyan to push, magenta to
// pull, yellow in the log.
var commitColor = map[entryKind]Color{pushEntry: Cyan, pullEntry: Magenta, logEntry: Yellow}

// tracking is a branch's state against its upstream: ⇡N ahead and ⇣N
// behind, = level with it, or why it has none to compare with.
func tracking(b repopage.Branch) []Span {
	switch {
	case b.Remote:
		return []Span{{Text: "remote", Style: Style{Dim: true}}}
	case b.Gone:
		return []Span{{Text: "gone", Style: Style{Color: Red}}}
	case b.Upstream == "":
		return []Span{{Text: "∅ no upstream", Style: Style{Color: Red}}}
	case b.Ahead == 0 && b.Behind == 0:
		return []Span{{Text: "=", Style: Style{Color: Green}}}
	}
	var out []Span
	if b.Ahead > 0 {
		out = append(out, Span{Text: fmt.Sprintf("⇡%d", b.Ahead), Style: Style{Color: Cyan}})
	}
	if b.Behind > 0 {
		if len(out) > 0 {
			out = append(out, Span{Text: " "})
		}
		out = append(out, Span{Text: fmt.Sprintf("⇣%d", b.Behind), Style: Style{Color: Magenta}})
	}
	return out
}

// ago says how long ago d was: "just now", or such as "4m ago".
func ago(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	return age(d) + " ago"
}

// age says how long ago d was, in its largest whole unit: 40m, 5h, 3d, 2w,
// 4mo, 2y, or now under a minute.
func age(d time.Duration) string {
	day := 24 * time.Hour
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", d/time.Minute)
	case d < day:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d < 7*day:
		return fmt.Sprintf("%dd", d/day)
	case d < 30*day:
		return fmt.Sprintf("%dw", d/(7*day))
	case d < 365*day:
		return fmt.Sprintf("%dmo", d/(30*day))
	}
	return fmt.Sprintf("%dy", d/(365*day))
}

// detailLines draws what the page shows beside the selected entry, every
// line of it, each w wide.
func (p page) detailLines(w int, now time.Time) [][]Span {
	entries := p.entries()
	if w <= 0 || len(entries) == 0 {
		return nil
	}
	e := entries[p.cursor]
	d, ok := p.details[e.item]
	var out [][]Span
	line := func(spans ...Span) { out = append(out, fit(spans, w)) }

	switch e.kind {
	case changeEntry:
		c := e.change
		var added, removed int
		if ok && len(d.d.Files) > 0 {
			for _, l := range d.d.Files[0].Lines {
				switch l.Kind {
				case repopage.Added:
					added++
				case repopage.Removed:
					removed++
				}
			}
		}
		out = append(out, leftRight([]Span{{Text: c.Path, Style: Style{Bold: true}}},
			[]Span{{Text: fmt.Sprintf("+%d", added), Style: Style{Color: Green}}, {Text: " "}, {Text: fmt.Sprintf("−%d", removed), Style: Style{Color: Red}}}, w))
		side := areaName[c.Area]
		if c.Orig != "" {
			side += " · renamed from " + c.Orig
		}
		line(Span{Text: side, Style: Style{Dim: true}})
	case branchEntry:
		b := e.branch
		name := Style{Bold: true}
		if b.Current {
			name.Color = Green
		}
		line(Span{Text: "branch ", Style: Style{Dim: true}}, Span{Text: b.Name, Style: name})
		switch {
		case b.Remote:
			line(Span{Text: "remote branch: no local branch tracks it", Style: Style{Dim: true}})
		case b.Gone:
			line(Span{Text: "tracks " + b.Upstream + ", which is gone", Style: Style{Color: Red}})
		case b.Upstream == "":
			line(Span{Text: "no upstream", Style: Style{Color: Red}})
		default:
			state := tracking(b)
			if b.Ahead == 0 && b.Behind == 0 {
				state = []Span{{Text: "up to date", Style: Style{Color: Green}}}
			}
			line(append([]Span{{Text: "tracks ", Style: Style{Dim: true}}, {Text: b.Upstream}, {Text: "  "}}, state...)...)
		}
		line()
		line(Span{Text: "Recent commits", Style: Style{Dim: true, Bold: true}})
	default:
		c := e.commit
		line(Span{Text: "commit ", Style: Style{Dim: true}}, Span{Text: c.Hash, Style: Style{Color: Yellow}})
		when := []Span{{Text: c.Author + " · " + ago(now.Sub(c.When)), Style: Style{Dim: true}}}
		switch e.kind {
		case pushEntry:
			when = append(when, Span{Text: " · "}, Span{Text: "not pushed", Style: Style{Color: Cyan}})
		case pullEntry:
			when = append(when, Span{Text: " · "}, Span{Text: "not pulled", Style: Style{Color: Magenta}})
		}
		line(when...)
		line()
		if !ok || d.err != "" {
			line(Span{Text: "    " + c.Subject})
			line()
		}
	}

	switch {
	case !ok:
		line(Span{Text: "loading…", Style: Style{Dim: true}})
	case d.err != "":
		line(Span{Text: "⚠ " + d.err, Style: Style{Color: Red}})
	default:
		if d.d.Message != "" {
			for _, text := range strings.Split(d.d.Message, "\n") {
				line(Span{Text: "    " + text})
			}
			line()
		}
		for _, c := range d.d.Commits {
			out = append(out, leftRight(
				[]Span{{Text: "  "}, {Text: c.Short, Style: Style{Color: Yellow}}, {Text: " " + c.Subject}},
				[]Span{{Text: c.Author + " " + age(now.Sub(c.When)), Style: Style{Dim: true}}}, w))
		}
		for _, f := range d.d.Files {
			if e.kind != changeEntry {
				name := f.Path
				if f.Orig != "" {
					name = f.Orig + " → " + f.Path
				}
				title := "── " + clean(name) + " "
				line(Span{Text: title + strings.Repeat("─", max(w-width(title), 0)), Style: Style{Color: Blue}})
			}
			out = append(out, diffLines(f, w)...)
		}
	}
	return out
}

// areaName names the side of the repo a change is on.
var areaName = map[repopage.Area]string{repopage.Staged: "staged", repopage.Unstaged: "unstaged", repopage.Untracked: "untracked", repopage.Unmerged: "unmerged"}

// gutterWidth is how wide the view beside the list must be for diff lines
// to show their old and new line numbers.
const gutterWidth = 48

// diffLines draws the lines of a file's diff w wide, with old and new line
// numbers when w allows.
func diffLines(f repopage.FileDiff, w int) [][]Span {
	digits := 0
	if w >= gutterWidth {
		digits = 4
		for _, l := range f.Lines {
			digits = max(digits, len(strconv.Itoa(max(l.Old, l.New))))
		}
	}
	num := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", digits)
		}
		return fmt.Sprintf("%*d", digits, n)
	}
	var out [][]Span
	for _, l := range f.Lines {
		var spans []Span
		switch l.Kind {
		case repopage.Hunk:
			spans = []Span{{Text: l.Text, Style: Style{Color: Cyan}}}
		case repopage.Meta:
			spans = []Span{{Text: l.Text, Style: Style{Dim: true}}}
		default:
			if digits > 0 {
				spans = []Span{{Text: num(l.Old) + " " + num(l.New) + " ", Style: Style{Dim: true}}}
			}
			mark, color := " ", Default
			switch l.Kind {
			case repopage.Added:
				mark, color = "+", Green
			case repopage.Removed:
				mark, color = "-", Red
			}
			spans = append(spans, Span{Text: mark + expandTabs(l.Text), Style: Style{Color: color}})
		}
		out = append(out, fit(spans, w))
	}
	return out
}

// expandTabs replaces the tabs in s with blanks to the next stop, every
// four columns.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// pageHeader names the repo, its branch and upstream, and its state, and
// when it was last fetched.
func (m Model) pageHeader(w int) Line {
	r := m.page.repo
	left := []Span{{Text: m.page.label, Style: Style{Bold: true}}, {Text: "  "}}
	if r.Detached != "" {
		left = append(left, Span{Text: "detached at " + r.Detached, Style: Style{Color: Magenta}})
	} else {
		left = append(left, Span{Text: r.Branch, Style: Style{Color: Magenta}}, Span{Text: " → ", Style: Style{Dim: true}})
		if r.Upstream != "" {
			left = append(left, Span{Text: r.Upstream, Style: Style{Dim: true}})
		} else {
			left = append(left, Span{Text: "no upstream", Style: Style{Color: Red}})
		}
	}
	left = append(append(left, Span{Text: "  "}), glyphs(r)...)
	var right []Span
	switch {
	case m.forcing:
		right = []Span{{Text: "fetching…", Style: Style{Color: Cyan}}}
	case r.Stale != "":
		right = []Span{{Text: "⚠ fetch failed", Style: Style{Color: Red}}}
	case !r.FetchedAt.IsZero():
		right = []Span{{Text: "fetched " + ago(m.now.Sub(r.FetchedAt)), Style: Style{Dim: true}}}
	}
	return Line{Spans: leftRight(left, right, w)}
}
