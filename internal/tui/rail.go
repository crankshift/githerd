package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// railKey handles a key on the rail.
func railKey(m Model, k Key) (Model, []Cmd) {
	if m.filtering {
		return filterKey(m, k), nil
	}
	switch k {
	case "j", KeyDown:
		m = m.move(1)
	case "k", KeyUp:
		m = m.move(-1)
	case "/":
		m.filtering = true
	case KeyEsc:
		m.filter = ""
	case "f":
		return m.forceFetch()
	case "q":
		return m, []Cmd{Quit{}}
	}
	return m, nil
}

// filterKey handles a key while the filter is being typed: a character
// extends it, Backspace shortens it, Enter keeps it and Esc clears it. The
// arrows still move.
func filterKey(m Model, k Key) Model {
	switch k {
	case KeyEnter:
		m.filtering = false
	case KeyEsc:
		m.filtering, m.filter = false, ""
	case KeyBackspace:
		if runes := []rune(m.filter); len(runes) > 0 {
			m.filter = string(runes[:len(runes)-1])
		}
	case KeyUp:
		m = m.move(-1)
	case KeyDown:
		m = m.move(1)
	default:
		if r := []rune(k); len(r) == 1 && r[0] >= ' ' {
			m.filter += string(k)
		}
	}
	return m
}

// matches reports whether label holds the letters of filter in order,
// whatever their case.
func matches(label, filter string) bool {
	rest := []rune(strings.ToLower(filter))
	for _, r := range strings.ToLower(label) {
		if len(rest) > 0 && r == rest[0] {
			rest = rest[1:]
		}
	}
	return len(rest) == 0
}

// move moves the cursor by n lines, stopping at either end.
func (m Model) move(n int) Model {
	lines := m.lines()
	if len(lines) == 0 {
		return m
	}
	i := min(max(m.cursor(lines)+n, 0), len(lines)-1)
	m.selected = lines[i].repo.Dir
	return m
}

// railHeader is how many rows the rail's header takes: counts, root or
// filter, rule.
const railHeader = 3

// footerRows is how many rows the rail's footer takes: what went wrong, if
// anything did, a rule and the keys.
func (m Model) footerRows() int {
	if len(m.problems) > 0 {
		return 3
	}
	return 2
}

// settle moves the cursor to the first line when its repo is not listed,
// as when the filter hides it, and scrolls the list as little as keeps the
// cursor in view at the height of the terminal.
func (m Model) settle() Model {
	lines := m.lines()
	if len(lines) > 0 && !slices.ContainsFunc(lines, func(l line) bool { return l.repo.Dir == m.selected }) {
		m.selected = lines[0].repo.Dir
	}
	m.top = scroll(m.top, m.cursor(lines), len(lines), m.height-railHeader-m.footerRows())
	return m
}

// line is one repo line of the rail.
type line struct {
	repo  Repo
	label string // the repo's path under the root; ". (root)" for the root
}

// lines returns the rail's repo lines that match the filter: repos that
// need attention first, then clean ones, each group in discovery order.
func (m Model) lines() []line {
	r := m.shown()
	if r == nil {
		return nil
	}
	var out []line
	for _, repo := range r.Repos {
		label, err := filepath.Rel(r.Dir, repo.Dir)
		switch {
		case err != nil:
			label = repo.Dir
		case label == ".":
			label = ". (root)"
		default:
			label = filepath.ToSlash(label)
		}
		if matches(label, m.filter) {
			out = append(out, line{repo: repo, label: label})
		}
	}
	slices.SortStableFunc(out, func(a, b line) int {
		return boolOrder(b.repo.NeedsAttention()) - boolOrder(a.repo.NeedsAttention())
	})
	return out
}

func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cursor returns the index of the selected line in lines, or 0 when the
// selected repo is not among them.
func (m Model) cursor(lines []line) int {
	for i, l := range lines {
		if l.repo.Dir == m.selected {
			return i
		}
	}
	return 0
}

// railView draws the rail: a header with the counts and the root, the repo
// lines, and the keys.
func railView(m Model, w, h int) Screen {
	if m.loaded && len(m.roots) == 0 {
		return noRootsView(m, w, h)
	}
	screen := Screen{m.counts(w), m.subhead(w), rule(w)}
	footer := Screen{rule(w), keys(w, "/", "filter", "f", "fetch", "q", "quit")}
	if len(m.problems) > 0 {
		footer = append(Screen{m.problemLine(w)}, footer...)
	}

	lines := m.lines()
	cur := m.cursor(lines)
	body := max(h-railHeader-len(footer), 0)
	top := scroll(m.top, cur, len(lines), body)
	for i := top; i < len(lines) && i < top+body; i++ {
		screen = append(screen, repoLine(lines[i], i == cur, w))
	}
	if len(lines) == 0 && m.filter != "" {
		screen = append(screen, Line{Spans: fit([]Span{{Text: "no repo matches", Style: Style{Dim: true}}}, w)})
	}
	for len(screen) < h-len(footer) {
		screen = append(screen, blank(w))
	}
	screen = append(screen, footer...)
	return screen[:min(len(screen), h)]
}

// problemLine names the first thing that went wrong in the last load or
// refresh, and how many more did.
func (m Model) problemLine(w int) Line {
	var more []Span
	if n := len(m.problems) - 1; n > 0 {
		more = []Span{{Text: fmt.Sprintf("+%d more", n), Style: Style{Dim: true}}}
	}
	return Line{Spans: leftRight([]Span{{Text: "⚠ " + m.problems[0], Style: Style{Color: Red}}}, more, w)}
}

// noRootsView tells where to list the roots when there are none.
func noRootsView(m Model, w, h int) Screen {
	screen := Screen{{Spans: fit([]Span{{Text: "No roots configured.", Style: Style{Color: Yellow, Bold: true}}}, w)}, blank(w)}
	texts := wrap("List the dirs githerd scans, one per line, such as ~/work, in", w)
	texts = append(texts, chunks(m.opts.RootsFile, w)...) // whole, to be found
	texts = append(texts, "")
	texts = append(texts, wrap(fmt.Sprintf("The rail reads it again every %d seconds.", RefreshEvery/time.Second), w)...)
	for _, text := range texts {
		screen = append(screen, Line{Spans: fit([]Span{{Text: text}}, w)})
	}
	footer := Screen{rule(w), keys(w, "q", "quit")}
	if len(m.problems) > 0 {
		footer = append(Screen{m.problemLine(w)}, footer...)
	}
	for len(screen) < h-len(footer) {
		screen = append(screen, blank(w))
	}
	screen = append(screen[:min(len(screen), max(h-len(footer), 0))], footer...)
	return screen[:min(len(screen), h)]
}

// wrap breaks text into lines of at most w columns at blanks. A word longer
// than w gets a line of its own, which fit then cuts. Empty text is one
// empty line.
func wrap(text string, w int) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(text) {
		switch {
		case cur == "":
			cur = word
		case width(cur)+1+width(word) <= w:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	return append(lines, cur)
}

// chunks breaks s into pieces of w columns, for a path that must show
// whole.
func chunks(s string, w int) []string {
	runes := []rune(s)
	var out []string
	for w > 0 && len(runes) > w {
		out = append(out, string(runes[:w]))
		runes = runes[w:]
	}
	return append(out, string(runes))
}

// counts is the header's first line: how many repos need attention and how
// many are stale, and when the next refresh comes.
func (m Model) counts(w int) Line {
	attention, stale, total := 0, 0, 0
	if r := m.shown(); r != nil {
		total = len(r.Repos)
		for _, repo := range r.Repos {
			if repo.NeedsAttention() {
				attention++
			}
			if repo.Stale != "" {
				stale++
			}
		}
	}
	tally := Style{Color: Green, Bold: true}
	if attention > 0 {
		tally.Color = Yellow
	}
	if !m.loaded {
		return Line{Spans: fit([]Span{{Text: "loading…", Style: Style{Dim: true}}}, w)}
	}
	var right []Span
	switch {
	case m.forcing:
		right = []Span{{Text: "fetching…", Style: Style{Color: Cyan}}}
	case m.refreshing:
		right = []Span{{Text: "syncing…", Style: Style{Color: Cyan}}}
	case !m.next.IsZero():
		secs := int((m.next.Sub(m.now) + time.Second - 1) / time.Second)
		right = []Span{{Text: fmt.Sprintf("↻ %ds", max(secs, 0)), Style: Style{Dim: true}}}
	}
	// Without room for the words, the bare count is still read as the
	// sidebar's N/T token is.
	left := func(words string) []Span {
		spans := []Span{{Text: fmt.Sprintf("%d/%d", attention, total), Style: tally}, {Text: words}}
		if stale > 0 {
			spans = append(spans, Span{Text: " · "}, Span{Text: fmt.Sprintf("%d stale", stale), Style: Style{Color: Red}})
		}
		return spans
	}
	spans := left(" need attention")
	if width(spansText(spans))+width(spansText(right))+1 > w {
		spans = left("")
	}
	return Line{Spans: leftRight(spans, right, w)}
}

// keys is a line of key hints: each key followed by what it does, two
// blanks apart, or one when two don't fit.
func keys(w int, pairs ...string) Line {
	hint := func(gap string) []Span {
		var spans []Span
		for i := 0; i+1 < len(pairs); i += 2 {
			if i > 0 {
				spans = append(spans, Span{Text: gap})
			}
			spans = append(spans, Span{Text: pairs[i], Style: Style{Color: Blue}}, Span{Text: " " + pairs[i+1]})
		}
		return spans
	}
	spans := hint("  ")
	if width(spansText(spans)) > w {
		spans = hint(" ")
	}
	return Line{Spans: fit(spans, w)}
}

// scroll returns the index of the first of n lines to show in a body of
// height lines, starting from top and moving as little as keeps cur in view.
func scroll(top, cur, n, height int) int {
	if height <= 0 {
		return 0
	}
	top = min(top, cur)
	top = max(top, cur-height+1)
	top = min(top, n-height)
	return max(top, 0)
}

// subhead is the header's second line: the filter while there is one, else
// the root shown.
func (m Model) subhead(w int) Line {
	hint := Style{Dim: true}
	switch {
	case m.filtering:
		return Line{Spans: leftRight(
			[]Span{{Text: "/", Style: Style{Color: Blue}}, {Text: m.filter}, {Text: "▏", Style: Style{Color: Blue}}},
			[]Span{{Text: "enter keep · esc clear", Style: hint}}, w)}
	case m.filter != "":
		return Line{Spans: leftRight(
			[]Span{{Text: "/", Style: Style{Color: Blue}}, {Text: m.filter}},
			[]Span{{Text: "esc clear", Style: hint}}, w)}
	}
	name := ""
	if r := m.shown(); r != nil {
		name = r.Name
	}
	return Line{Spans: fit([]Span{{Text: name}}, w)}
}

// repoLine draws one repo: its path on the left, its state glyphs on the
// right. A clean repo is dimmed.
func repoLine(l line, selected bool, w int) Line {
	right := glyphs(l.repo)
	return Line{
		Spans:    leftRight([]Span{{Text: cutPath(l.label, w-width(spansText(right))-1)}}, right, w),
		Selected: selected,
		Dim:      !l.repo.NeedsAttention(),
	}
}

// glyphs are the state glyphs of a repo, or a check mark when it has none:
// ●N uncommitted, ∅ no upstream, ⇡N ahead, ⇣N behind, ⌀ detached, ⚠ stale.
func glyphs(r Repo) []Span {
	var gs []Span
	if r.Uncommitted > 0 {
		gs = append(gs, Span{Text: fmt.Sprintf("●%d", r.Uncommitted), Style: Style{Color: Yellow}})
	}
	if r.NoUpstream() {
		gs = append(gs, Span{Text: "∅", Style: Style{Color: Red}})
	}
	if r.Ahead > 0 {
		gs = append(gs, Span{Text: fmt.Sprintf("⇡%d", r.Ahead), Style: Style{Color: Cyan}})
	}
	if r.Behind > 0 {
		gs = append(gs, Span{Text: fmt.Sprintf("⇣%d", r.Behind), Style: Style{Color: Magenta}})
	}
	if r.Detached != "" {
		gs = append(gs, Span{Text: "⌀", Style: Style{Dim: true}})
	}
	if r.Stale != "" {
		gs = append(gs, Span{Text: "⚠", Style: Style{Color: Red}})
	}
	if len(gs) == 0 {
		return []Span{{Text: "✓", Style: Style{Color: Green}}}
	}
	var out []Span
	for i, g := range gs {
		if i > 0 {
			out = append(out, Span{Text: " "})
		}
		out = append(out, g)
	}
	return out
}

func spansText(spans []Span) string {
	return Line{Spans: spans}.Text()
}
