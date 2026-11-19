package repopage

import (
	"strconv"
	"strings"
)

// FileDiff is the diff of one file.
type FileDiff struct {
	Path  string // slash-separated, from the repo's top
	Orig  string // renamed or copied: the path it came from
	Lines []DiffLine
}

// DiffLine is one line of a diff.
type DiffLine struct {
	Kind LineKind
	Text string // without the leading +, - or blank; a hunk or meta line whole
	Old  int    // its line number in the old file; 0 when it is not there
	New  int    // its line number in the new file; 0 when it is not there
}

// LineKind is what a DiffLine is.
type LineKind uint8

// The kinds of diff lines.
const (
	Hunk    LineKind = iota + 1 // a hunk's header, such as "@@ -1,3 +1,4 @@ func main() {"
	Context                     // in both files
	Added                       // only in the new file
	Removed                     // only in the old file
	// Meta says something about the file or the line before it, such as
	// "new file mode 100644", "Binary files a/x and b/x differ" or
	// "\ No newline at end of file".
	Meta
)

// parseFiles splits the lines of a patch, as git diff prints it, into one
// diff per file, in order. It drops the lines that only repeat the file's
// names and blobs: "diff --git", "index", "---" and "+++".
func parseFiles(lines []string) [][]DiffLine {
	var files [][]DiffLine
	var cur []DiffLine
	inHunk := false
	old, new := 0, 0
	prefix := 1 // columns of +, - or blank before a hunk line's text: one per parent
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "diff --cc ") || strings.HasPrefix(line, "diff --combined ") {
			if cur != nil {
				files = append(files, cur)
			}
			cur, inHunk = []DiffLine{}, false
			continue
		}
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			at := strings.TrimLeft(line, "@")
			prefix = len(line) - len(at) - 1
			old, new = hunkStart(line, prefix)
			cur = append(cur, DiffLine{Kind: Hunk, Text: line})
			continue
		}
		if !inHunk {
			if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
				continue
			}
			cur = append(cur, DiffLine{Kind: Meta, Text: line})
			continue
		}
		if strings.HasPrefix(line, `\`) {
			cur = append(cur, DiffLine{Kind: Meta, Text: line})
			continue
		}
		marks, text := line[:min(prefix, len(line))], line[min(prefix, len(line)):]
		switch {
		case strings.Contains(marks, "+"):
			cur = append(cur, DiffLine{Kind: Added, Text: text, New: new})
			new++
		case strings.Contains(marks, "-"):
			if prefix > 1 {
				cur = append(cur, DiffLine{Kind: Removed, Text: text}) // from one of several parents
				continue
			}
			cur = append(cur, DiffLine{Kind: Removed, Text: text, Old: old})
			old++
		default:
			l := DiffLine{Kind: Context, Text: text, New: new}
			if prefix == 1 {
				l.Old = old
				old++
			}
			cur = append(cur, l)
			new++
		}
	}
	if cur != nil {
		files = append(files, cur)
	}
	return files
}

// hunkStart returns the first old and new line numbers of the hunk whose
// header is line. A combined diff's hunk has an old range per parent, so it
// has no one old number.
func hunkStart(line string, prefix int) (old, new int) {
	for _, f := range strings.Fields(line) {
		start, _, _ := strings.Cut(f[min(1, len(f)):], ",")
		n, err := strconv.Atoi(start)
		if err != nil {
			continue
		}
		switch f[0] {
		case '-':
			if prefix == 1 {
				old = n
			}
		case '+':
			new = n
			return old, new
		}
	}
	return old, new
}
