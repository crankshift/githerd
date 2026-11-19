package repopage

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Limits on what Show reads, so a huge file or commit can't stall the page
// or fill memory. Past them, the diff ends in a Meta line that says so.
const (
	maxLines     = 5000 // lines of one diff
	maxLineBytes = 2000 // bytes of one line; a longer line is cut
)

// Item is a line of the repo page that has something to show beside it: a
// change, a commit or a branch. Exactly one of its fields is set.
type Item struct {
	Change Change // a change
	Commit string // a commit: its full hash
	Branch string // a branch: its full ref, such as refs/heads/main
}

// Detail is what the repo page shows beside an item.
type Detail struct {
	Message string     // a commit: its whole message
	Files   []FileDiff // a change: its diff; a commit: the diff of every file it changed
	Commits []Commit   // a branch: its recent commits, newest first
}

// branchLength is how many recent commits Show lists for a branch.
const branchLength = 10

// Show returns what the repo page shows beside it for the repo in dir.
func Show(ctx context.Context, dir string, it Item) (Detail, error) {
	switch {
	case it.Branch != "":
		cs, err := commits(ctx, dir, branchLength, it.Branch)
		return Detail{Commits: cs}, err
	case it.Commit != "":
		return commitDiff(ctx, dir, it.Commit)
	}
	return changeDiff(ctx, dir, it.Change)
}

// commitDiff returns the message of the commit hash and the diff of every
// file it changed; for a merge, against its first parent.
func commitDiff(ctx context.Context, dir, hash string) (Detail, error) {
	show := []string{"show", "--no-color", "--no-ext-diff", "--no-show-signature", "--diff-merges=first-parent", "-M"}
	msg, err := git(ctx, dir, append(show, "--no-patch", "--format=%B", hash, "--")...)
	if err != nil {
		return Detail{}, err
	}
	raw, err := git(ctx, dir, append(show, "--format=", "--raw", "-z", hash, "--")...)
	if err != nil {
		return Detail{}, err
	}
	names, err := rawNames(raw)
	if err != nil {
		return Detail{}, fmt.Errorf("%s: git show: %v", dir, err)
	}
	lines, cut, err := gitLines(ctx, dir, false, append(show, "--format=", "--patch", "--src-prefix=a/", "--dst-prefix=b/", hash, "--")...)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Message: strings.TrimRight(string(msg), "\n")}
	// The patch has a section for each file of the raw diff, in its order.
	for i, f := range parseFiles(lines) {
		if i >= len(names) {
			break
		}
		names[i].Lines = f
		d.Files = append(d.Files, names[i])
	}
	if cut && len(d.Files) > 0 {
		last := &d.Files[len(d.Files)-1]
		last.Lines = append(last.Lines, cutLine)
	}
	return d, nil
}

// rawNames returns the files of a raw diff printed with -z, in order.
func rawNames(raw []byte) ([]FileDiff, error) {
	var files []FileDiff
	fields := strings.Split(strings.Trim(string(raw), "\n\x00"), "\x00")
	for i := 0; i < len(fields); i++ {
		meta := strings.Fields(fields[i])
		if len(meta) == 0 {
			continue
		}
		if len(meta) != 5 || !strings.HasPrefix(meta[0], ":") || i+1 >= len(fields) {
			return nil, fmt.Errorf("parsing raw diff entry %q", fields[i])
		}
		i++
		fd := FileDiff{Path: fields[i]}
		if s := meta[4]; s[0] == 'R' || s[0] == 'C' {
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("parsing raw diff entry %q: no new path", fields[i-1])
			}
			i++
			fd.Orig, fd.Path = fd.Path, fields[i]
		}
		files = append(files, fd)
	}
	return files, nil
}

// changeDiff returns the diff of a change on its side alone: the index
// against HEAD when staged, the working tree against the index when
// unstaged or unmerged, and the whole file as added when untracked.
func changeDiff(ctx context.Context, dir string, c Change) (Detail, error) {
	if c.Area == Untracked && strings.HasSuffix(c.Path, "/") {
		// git lists a repo nested in this one as an untracked dir, and
		// githerd lists it as a repo of its own.
		return Detail{Files: []FileDiff{{Path: c.Path, Lines: []DiffLine{{Kind: Meta, Text: "a repo of its own"}}}}}, nil
	}
	args := []string{"diff", "--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/"}
	exit1OK := false
	switch c.Area {
	case Staged:
		args = append(args, "--cached", "-M", "--", c.Path)
		if c.Orig != "" {
			args = append(args, c.Orig)
		}
	case Untracked:
		args, exit1OK = append(args, "--no-index", "--", "/dev/null", c.Path), true // git diff --no-index exits 1 when the files differ
	default:
		args = append(args, "--", c.Path)
	}
	lines, cut, err := gitLines(ctx, dir, exit1OK, args...)
	if err != nil {
		return Detail{}, err
	}
	fd := FileDiff{Path: c.Path, Orig: c.Orig}
	for _, f := range parseFiles(lines) {
		fd.Lines = append(fd.Lines, f...)
	}
	if cut {
		fd.Lines = append(fd.Lines, cutLine)
	}
	return Detail{Files: []FileDiff{fd}}, nil
}

// cutLine ends a diff that was longer than Show reads.
var cutLine = DiffLine{Kind: Meta, Text: fmt.Sprintf("… cut after %d lines", maxLines)}

// gitLines runs git in dir as git does, and returns the lines it prints, at
// most maxLines of them, each cut to maxLineBytes, and whether it printed
// more. It stops git once it has enough. With exit1OK, exit status 1 is not
// an error, as `git diff --no-index` exits 1 when the files differ.
func gitLines(ctx context.Context, dir string, exit1OK bool, args ...string) (lines []string, cut bool, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, false, fmt.Errorf("%s: git %s: %v", dir, args[0], err)
	}
	r := bufio.NewReader(stdout)
	for {
		line, err := readLine(r)
		if err != nil && line == "" {
			break
		}
		if len(lines) == maxLines {
			cut = true
			cancel()
			break
		}
		lines = append(lines, line)
		if err != nil {
			break
		}
	}
	err = cmd.Wait()
	var exit *exec.ExitError
	switch {
	case cut, err == nil, exit1OK && errors.As(err, &exit) && exit.ExitCode() == 1:
		return lines, cut, nil
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = err.Error()
	}
	return nil, false, fmt.Errorf("%s: git %s: %s", dir, args[0], msg)
}

// readLine reads one line from r without its line end, keeping at most
// maxLineBytes of it, with any broken UTF-8 replaced.
func readLine(r *bufio.Reader) (string, error) {
	var b []byte
	for {
		chunk, err := r.ReadSlice('\n')
		b = append(b, chunk[:min(len(chunk), max(maxLineBytes-len(b), 0))]...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		line := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
		return strings.ToValidUTF8(line, "�"), err
	}
}
