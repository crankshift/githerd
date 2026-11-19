package tui

import (
	"fmt"
	"time"

	"github.com/crankshift/githerd/internal/repopage"
)

// entry is a line of the repo page with something to show beside it: a
// change, a commit or a branch.
type entry interface {
	// item is what the page asks repopage to show beside the entry.
	item() repopage.Item
	// row draws the entry's line of the list, w wide: what goes on the
	// left, and what goes flush right.
	row(w int, now time.Time) (left, right []Span)
	// head draws the lines above what was read beside the entry, w wide:
	// d, when ok says it was read.
	head(d detail, ok bool, w int, now time.Time) [][]Span
	// titled reports whether each file of what was read beside the entry
	// shows under a title naming it.
	titled() bool
}

// changeEntry is a changed file, on one side of the repo.
type changeEntry struct{ c repopage.Change }

func (e changeEntry) item() repopage.Item { return repopage.Item{Change: e.c} }

func (e changeEntry) row(w int, _ time.Time) (left, right []Span) {
	path := e.c.Path
	if e.c.Orig != "" {
		path = e.c.Orig + " → " + e.c.Path
	}
	return []Span{{Text: "  "}, {Text: e.c.Code, Style: Style{Color: areaColor[e.c.Area]}}, {Text: " " + cutPath(path, w-5)}}, nil
}

// head names the file and the side it changed on, and counts the lines
// its diff adds and removes.
func (e changeEntry) head(d detail, ok bool, w int, _ time.Time) [][]Span {
	var added, removed int
	if ok && len(d.read.Files) > 0 {
		for _, l := range d.read.Files[0].Lines {
			switch l.Kind {
			case repopage.Added:
				added++
			case repopage.Removed:
				removed++
			}
		}
	}
	side := areaName[e.c.Area]
	if e.c.Orig != "" {
		side += " · renamed from " + e.c.Orig
	}
	return [][]Span{
		leftRight([]Span{{Text: e.c.Path, Style: Style{Bold: true}}},
			[]Span{{Text: fmt.Sprintf("+%d", added), Style: Style{Color: Green}}, {Text: " "}, {Text: fmt.Sprintf("−%d", removed), Style: Style{Color: Red}}}, w),
		fit([]Span{{Text: side, Style: Style{Dim: true}}}, w),
	}
}

// titled is false: the head names the change's one file.
func (changeEntry) titled() bool { return false }

// areaColor is the color of a change's code: green when staged, yellow
// when unstaged, red when untracked or unmerged.
var areaColor = map[repopage.Area]Color{repopage.Staged: Green, repopage.Unstaged: Yellow, repopage.Untracked: Red, repopage.Unmerged: Red}

// areaName names the side of the repo a change is on.
var areaName = map[repopage.Area]string{repopage.Staged: "staged", repopage.Unstaged: "unstaged", repopage.Untracked: "untracked", repopage.Unmerged: "unmerged"}

// commitEntry is a commit, listed in a section of the page.
type commitEntry struct {
	c  repopage.Commit
	in section
}

// section is a section of the page that lists commits.
type section struct {
	color Color  // of its commits' short hashes, and of state
	state string // what the page says of each of its commits beside it, such as "not pushed"
}

// The sections that list commits.
var (
	toPush = section{color: Cyan, state: "not pushed"}
	toPull = section{color: Magenta, state: "not pulled"}
	inLog  = section{color: Yellow}
)

func (e commitEntry) item() repopage.Item { return repopage.Item{Commit: e.c.Hash} }

func (e commitEntry) row(_ int, now time.Time) (left, right []Span) {
	return []Span{{Text: "  "}, {Text: e.c.Short, Style: Style{Color: e.in.color}}, {Text: " " + e.c.Subject}},
		[]Span{{Text: age(now.Sub(e.c.When)), Style: Style{Dim: true}}}
}

// head names the commit in full, its author, how long ago it was made and
// its state, and its subject until its whole message is read.
func (e commitEntry) head(d detail, ok bool, w int, now time.Time) [][]Span {
	when := []Span{{Text: e.c.Author + " · " + ago(now.Sub(e.c.When)), Style: Style{Dim: true}}}
	if e.in.state != "" {
		when = append(when, Span{Text: " · "}, Span{Text: e.in.state, Style: Style{Color: e.in.color}})
	}
	out := [][]Span{
		fit([]Span{{Text: "commit ", Style: Style{Dim: true}}, {Text: e.c.Hash, Style: Style{Color: Yellow}}}, w),
		fit(when, w),
		fit(nil, w),
	}
	if !ok || d.err != "" {
		out = append(out, fit([]Span{{Text: "    " + e.c.Subject}}, w), fit(nil, w))
	}
	return out
}

// titled is true: a commit changes any number of files.
func (commitEntry) titled() bool { return true }

// branchEntry is a local branch, or a remote branch no local branch tracks.
type branchEntry struct{ b repopage.Branch }

func (e branchEntry) item() repopage.Item { return repopage.Item{Branch: e.b.Ref} }

func (e branchEntry) row(int, time.Time) (left, right []Span) {
	mark, name := Span{Text: "    "}, Style{}
	switch {
	case e.b.Current:
		mark, name = Span{Text: "  * ", Style: Style{Color: Green}}, Style{Color: Green, Bold: true}
	case e.b.Remote:
		name = Style{Dim: true}
	}
	short, _ := tracking(e.b)
	return []Span{mark, {Text: e.b.Name, Style: name}}, short
}

// head names the branch and how it stands against its upstream, above its
// recent commits.
func (e branchEntry) head(_ detail, _ bool, w int, _ time.Time) [][]Span {
	name := Style{Bold: true}
	if e.b.Current {
		name.Color = Green
	}
	_, long := tracking(e.b)
	return [][]Span{
		fit([]Span{{Text: "branch ", Style: Style{Dim: true}}, {Text: e.b.Name, Style: name}}, w),
		fit(long, w),
		fit(nil, w),
		fit([]Span{{Text: "Recent commits", Style: Style{Dim: true, Bold: true}}}, w),
	}
}

// titled is true, as for a commit, though a branch lists no files.
func (branchEntry) titled() bool { return true }

// tracking is how branch b stands against its upstream, short for its row
// and long for beside it: ⇡N ahead and ⇣N behind, level with it, or why it
// has none to compare with.
func tracking(b repopage.Branch) (short, long []Span) {
	red, dim := Style{Color: Red}, Style{Dim: true}
	tracks := func(state ...Span) []Span {
		return append([]Span{{Text: "tracks ", Style: dim}, {Text: b.Upstream}, {Text: "  "}}, state...)
	}
	switch {
	case b.Remote:
		return []Span{{Text: "remote", Style: dim}}, []Span{{Text: "remote branch: no local branch tracks it", Style: dim}}
	case b.Gone:
		return []Span{{Text: "gone", Style: red}}, []Span{{Text: "tracks " + b.Upstream + ", which is gone", Style: red}}
	case b.Upstream == "":
		return []Span{{Text: "∅ no upstream", Style: red}}, []Span{{Text: "no upstream", Style: red}}
	case b.Ahead == 0 && b.Behind == 0:
		return []Span{{Text: "=", Style: Style{Color: Green}}}, tracks(Span{Text: "up to date", Style: Style{Color: Green}})
	}
	var counts []Span
	if b.Ahead > 0 {
		counts = append(counts, Span{Text: fmt.Sprintf("⇡%d", b.Ahead), Style: Style{Color: Cyan}})
	}
	if b.Behind > 0 {
		if len(counts) > 0 {
			counts = append(counts, Span{Text: " "})
		}
		counts = append(counts, Span{Text: fmt.Sprintf("⇣%d", b.Behind), Style: Style{Color: Magenta}})
	}
	return counts, tracks(counts...)
}
