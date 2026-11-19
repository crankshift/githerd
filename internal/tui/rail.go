package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"time"
)

// railKey handles a key on the rail.
func railKey(m Model, k Key) (Model, []Cmd) {
	return m, nil
}

// line is one repo line of the rail.
type line struct {
	repo  Repo
	label string // the repo's path under the root; ". (root)" for the root
}

// lines returns the rail's repo lines: repos that need attention first,
// then clean ones, each group in discovery order.
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
		out = append(out, line{repo: repo, label: label})
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
	screen := Screen{m.counts(w), Line{Spans: fit([]Span{{Text: m.rootName()}}, w)}, rule(w)}
	footer := Screen{rule(w), keys(w, "/", "filter", "f", "fetch", "q", "quit")}

	lines := m.lines()
	cur := m.cursor(lines)
	body := max(h-len(screen)-len(footer), 0)
	top := scroll(m.top, cur, len(lines), body)
	for i := top; i < len(lines) && i < top+body; i++ {
		screen = append(screen, repoLine(lines[i], i == cur, w))
	}
	for len(screen) < h-len(footer) {
		screen = append(screen, blank(w))
	}
	screen = append(screen, footer...)
	return screen[:min(len(screen), h)]
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
	var right []Span
	if !m.next.IsZero() {
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

// rootName is the name of the root shown.
func (m Model) rootName() string {
	if r := m.shown(); r != nil {
		return r.Name
	}
	return ""
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
