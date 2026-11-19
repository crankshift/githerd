package tui

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Screen is what View draws: one Line per row, each exactly as wide as the
// view.
type Screen []Line

// Line is one row of a Screen.
type Line struct {
	Spans    []Span
	Selected bool // the cursor's line, drawn in reverse video
	Dim      bool // drawn faint, as a clean repo is
}

// Span is a run of text in one style.
type Span struct {
	Text  string
	Style Style
}

// Style is how a Span is drawn.
type Style struct {
	Color   Color
	Bold    bool
	Dim     bool
	Reverse bool // in reverse video, as the cursor's part of a line is
}

// Color is a terminal foreground color.
type Color uint8

// The colors a Span can have.
const (
	Default Color = iota
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
)

// Text returns the line's text.
func (l Line) Text() string {
	var b strings.Builder
	for _, s := range l.Spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// String returns the screen as plain text for golden files: each line
// between │ borders, so trailing blanks show, followed by the line's
// selected and dim marks. A line is marked selected when part of it is.
func (s Screen) String() string {
	var b strings.Builder
	for _, l := range s {
		b.WriteString("│" + l.Text() + "│")
		if l.Selected || slices.ContainsFunc(l.Spans, func(sp Span) bool { return sp.Style.Reverse }) {
			b.WriteString(" selected")
		}
		if l.Dim {
			b.WriteString(" dim")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// ANSI returns the escape sequences that draw the screen on a terminal,
// from its top-left corner, each line at its own row.
func (s Screen) ANSI() string {
	var b strings.Builder
	for i, l := range s {
		b.WriteString("\x1b[" + strconv.Itoa(i+1) + ";1H\x1b[0m")
		if l.Selected {
			// Colors would turn into backgrounds in reverse video.
			b.WriteString("\x1b[7m" + l.Text())
		} else {
			for _, sp := range l.Spans {
				b.WriteString(sgr(sp.Style, l.Dim) + sp.Text)
			}
		}
		b.WriteString("\x1b[0m\x1b[K")
	}
	return b.String()
}

// sgr returns the escape sequence that sets style, faint when dim is set.
func sgr(style Style, dim bool) string {
	codes := []string{"0"}
	if style.Bold {
		codes = append(codes, "1")
	}
	if style.Dim || dim {
		codes = append(codes, "2")
	}
	if style.Reverse {
		codes = append(codes, "7")
	}
	if style.Color != Default {
		codes = append(codes, strconv.Itoa(30+int(ansiColor[style.Color])))
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// ansiColor maps a Color to its ANSI color number.
var ansiColor = map[Color]int{Red: 1, Green: 2, Yellow: 3, Blue: 4, Magenta: 5, Cyan: 6}

// width returns how many columns s takes. Every rune githerd draws takes
// one.
func width(s string) int { return utf8.RuneCountInString(s) }

// clean replaces control characters in s, which a repo path or a git
// message could hold, so they can't drive the terminal.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			return '?'
		}
		return r
	}, s)
}

// fit returns spans cut or padded to exactly w columns. A span that doesn't
// fit ends in …
func fit(spans []Span, w int) []Span {
	var out []Span
	n := 0
	for _, sp := range spans {
		text := clean(sp.Text)
		if n+width(text) > w {
			runes := []rune(text)
			if keep := w - n - 1; keep >= 0 {
				text = string(runes[:keep]) + "…"
			} else {
				text = ""
			}
		}
		if text != "" {
			out = append(out, Span{Text: text, Style: sp.Style})
			n += width(text)
		}
		if n >= w {
			break
		}
	}
	if n < w {
		out = append(out, Span{Text: strings.Repeat(" ", w-n)})
	}
	return out
}

// leftRight returns left and right on one line w columns wide, right flush
// with the edge. Left is cut to make room; right is dropped when there is no
// room for it and at least one column of left.
func leftRight(left, right []Span, w int) []Span {
	rw := 0
	for _, sp := range right {
		rw += width(clean(sp.Text))
	}
	if rw == 0 || rw+2 > w {
		return fit(left, w)
	}
	return append(append(fit(left, w-rw-1), Span{Text: " "}), right...)
}

// cutPath cuts the slash-separated path p to at most w columns by dropping
// its start, marked with …, so the end stays visible. It drops whole
// directories while the last name fits.
func cutPath(p string, w int) string {
	p = clean(p)
	if width(p) <= w {
		return p
	}
	for i, r := range p {
		if r == '/' && width(p[i:])+1 <= w {
			return "…" + p[i:]
		}
	}
	runes := []rune(p)
	if w <= 1 {
		return string(runes[len(runes)-max(w, 0):])
	}
	return "…" + string(runes[len(runes)-w+1:])
}

// blank is an empty line w columns wide.
func blank(w int) Line { return Line{Spans: fit(nil, w)} }

// rule is a horizontal line w columns wide.
func rule(w int) Line {
	return Line{Spans: []Span{{Text: strings.Repeat("─", max(w, 0)), Style: Style{Dim: true}}}}
}
