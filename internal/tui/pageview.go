package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/crankshift/githerd/internal/repopage"
)

// pageView draws the repo page: a header with the repo's state, the list
// of its sections beside what the selected line shows, and the keys.
func pageView(m Model, w, h int) Screen {
	footer := m.pageFooter(w)
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

// rowSpans draws one row of the list w wide: the cursor's in reverse video.
func rowSpans(r row, selected bool, w int, now time.Time) []Span {
	if r.entry == nil {
		return fit(r.text, w)
	}
	left, right := r.entry.row(w, now)
	spans := leftRight(left, right, w)
	if selected {
		return []Span{{Text: spansText(spans), Style: Style{Reverse: true}}}
	}
	return spans
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
	d, ok := p.details[e.item()]
	out := e.head(d, ok, w, now)
	line := func(spans ...Span) { out = append(out, fit(spans, w)) }

	switch {
	case !ok:
		line(Span{Text: "loading…", Style: Style{Dim: true}})
	case d.err != "":
		line(Span{Text: "⚠ " + d.err, Style: Style{Color: Red}})
	default:
		if d.read.Message != "" {
			for _, text := range strings.Split(d.read.Message, "\n") {
				line(Span{Text: "    " + text})
			}
			line()
		}
		for _, c := range d.read.Commits {
			out = append(out, leftRight(
				[]Span{{Text: "  "}, {Text: c.Short, Style: Style{Color: Yellow}}, {Text: " " + c.Subject}},
				[]Span{{Text: c.Author + " " + age(now.Sub(c.When)), Style: Style{Dim: true}}}, w))
		}
		for _, f := range d.read.Files {
			if e.titled() {
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
	if width(spansText(left))+1+width(spansText(right)) > w {
		right = nil // the repo's state matters more
	}
	return Line{Spans: leftRight(left, right, w)}
}
