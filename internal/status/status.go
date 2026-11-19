// Package status reads the state of one repo with git, without writing to it.
package status

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo is the state of one repo.
type Repo struct {
	Dir         string // absolute path of the repo
	Branch      string // current branch; empty when detached
	Detached    string // short commit HEAD points at when detached; empty on a branch
	Upstream    string // upstream of the current branch, such as origin/main; empty when there is none
	Uncommitted int    // porcelain entries in the working tree and index, untracked files included
	// Ahead is the number of commits the current branch has that its
	// upstream lacks. With no upstream, it counts the commits that are on no
	// remote branch.
	Ahead     int
	Behind    int       // commits the upstream has that the current branch lacks, as of the last fetch
	NoCommits bool      // the current branch has no commits yet
	FetchedAt time.Time // when the repo was last fetched, from FETCH_HEAD; zero when it never was, or its last fetch failed
	// Stale is why the repo's last fetch failed, so Behind may be out of
	// date; empty when it did not fail. Read leaves it empty: the outcomes
	// of the fetch module set it.
	Stale string
}

// NoUpstream reports whether the current branch has no upstream branch. A
// detached repo is never no upstream, because it has no branch.
func (r Repo) NoUpstream() bool { return r.Detached == "" && r.Upstream == "" }

// NeedsAttention reports whether the repo is uncommitted, ahead, behind or
// no upstream. Detached alone does not count.
func (r Repo) NeedsAttention() bool {
	return r.Uncommitted > 0 || r.Ahead > 0 || r.Behind > 0 || r.NoUpstream()
}

// Tally counts how many of repos need attention, and how many are stale.
func Tally(repos []Repo) (attention, stale int) {
	for _, r := range repos {
		if r.NeedsAttention() {
			attention++
		}
		if r.Stale != "" {
			stale++
		}
	}
	return attention, stale
}

// Entry is a file that `git status --porcelain=v2 -z` lists: its entry as
// git prints it, such as "1 .M N... 100644 100644 100644 <hash> <hash>
// README.md", and for a renamed or copied file the path it came from, which
// git prints apart.
type Entry struct {
	Line string
	Orig string
}

// Read returns the state of the repo in dir.
func Read(ctx context.Context, dir string) (Repo, error) {
	r, _, err := ReadEntries(ctx, dir)
	return r, err
}

// ReadEntries returns the state of the repo in dir and the entries of the
// git status it read that state from: one for each change Uncommitted
// counts, in git's order.
func ReadEntries(ctx context.Context, dir string) (Repo, []Entry, error) {
	out, err := Git(ctx, dir, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return Repo{}, nil, err
	}
	r := Repo{Dir: dir}
	var oid string
	var entries []Entry
	tracked := false // the upstream exists, so ahead and behind are known
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if field == "" {
			continue
		}
		header, ok := strings.CutPrefix(field, "# ")
		if !ok {
			e := Entry{Line: field}
			if field[0] == '2' { // renamed or copied
				if i+1 >= len(fields) {
					return Repo{}, nil, fmt.Errorf("%s: git status: parsing %q: no original path", dir, field)
				}
				i++
				e.Orig = fields[i]
			}
			entries = append(entries, e)
			continue
		}
		key, value, _ := strings.Cut(header, " ")
		switch key {
		case "branch.oid":
			oid = value
		case "branch.head":
			r.Branch = value
		case "branch.upstream":
			r.Upstream = value
		case "branch.ab":
			tracked = true
			if _, err := fmt.Sscanf(value, "+%d -%d", &r.Ahead, &r.Behind); err != nil {
				return Repo{}, nil, fmt.Errorf("%s: parsing %q: %v", dir, field, err)
			}
		}
	}
	r.Uncommitted = len(entries)
	r.NoCommits = oid == "(initial)"

	switch {
	case r.Branch == "(detached)":
		r.Branch = ""
		short, err := Git(ctx, dir, "rev-parse", "--short", oid)
		if err != nil {
			return Repo{}, nil, err
		}
		r.Detached = strings.TrimSpace(string(short))
	case !tracked:
		// No upstream, or one whose remote branch is gone: count what
		// would be lost if this repo disappeared.
		r.Upstream = ""
		if !r.NoCommits {
			if r.Ahead, err = count(ctx, dir, "rev-list", "--count", "HEAD", "--not", "--remotes"); err != nil {
				return Repo{}, nil, err
			}
		}
	}

	if r.FetchedAt, err = fetchedAt(ctx, dir); err != nil {
		return Repo{}, nil, err
	}
	return r, entries, nil
}

// fetchedAt returns when the repo in dir was last fetched: the mtime of its
// FETCH_HEAD. It returns the zero time when the repo was never fetched, and
// also when its last fetch failed; see LastFetch.
func fetchedAt(ctx context.Context, dir string) (time.Time, error) {
	at, failed, err := LastFetch(ctx, dir)
	if failed {
		at = time.Time{}
	}
	return at, err
}

// LastFetch returns when the repo in dir was last fetched, from the mtime of
// its FETCH_HEAD, and whether that fetch failed, which leaves FETCH_HEAD
// empty. The time is zero when the repo was never fetched.
func LastFetch(ctx context.Context, dir string) (at time.Time, failed bool, err error) {
	path, err := FetchHead(ctx, dir)
	if err != nil {
		return time.Time{}, false, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return info.ModTime(), info.Size() == 0, nil
}

// FetchHead returns the path of the FETCH_HEAD of the repo in dir, where
// git records what each fetch brought.
func FetchHead(ctx context.Context, dir string) (string, error) {
	out, err := Git(ctx, dir, "rev-parse", "--git-path", "FETCH_HEAD")
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return path, nil
}

// count runs a git command that prints one number.
func count(ctx context.Context, dir string, args ...string) (int, error) {
	out, err := Git(ctx, dir, args...)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("%s: git %s: %v", dir, args[0], err)
	}
	return n, nil
}

// Git runs a git command that only reads the repo in dir, and returns what
// it prints. It takes no optional locks, so it never writes to the repo,
// not even to refresh the index, and never competes with git commands the
// user or an agent runs there. Its error names dir, the command and what
// git said.
func Git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s: git %s: %s", dir, args[0], msg)
	}
	return out, nil
}
