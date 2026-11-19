// Package status reads the state of one repo with git, without writing to it.
package status

import (
	"bufio"
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
	FetchedAt time.Time // mtime of FETCH_HEAD; zero when the repo was never fetched
}

// NoUpstream reports whether the current branch has no upstream branch. A
// detached repo is never no upstream, because it has no branch.
func (r Repo) NoUpstream() bool { return r.Detached == "" && r.Upstream == "" }

// NeedsAttention reports whether the repo is uncommitted, ahead, behind or
// no upstream. Detached alone does not count.
func (r Repo) NeedsAttention() bool {
	return r.Uncommitted > 0 || r.Ahead > 0 || r.Behind > 0 || r.NoUpstream()
}

// Read returns the state of the repo in dir.
func Read(ctx context.Context, dir string) (Repo, error) {
	out, err := git(ctx, dir, "status", "--porcelain=v2", "--branch", "--untracked-files=all")
	if err != nil {
		return Repo{}, err
	}
	r := Repo{Dir: dir}
	var oid string
	tracked := false // the upstream exists, so ahead and behind are known
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		header, ok := strings.CutPrefix(line, "# ")
		if !ok {
			r.Uncommitted++
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
				return Repo{}, fmt.Errorf("%s: parsing %q: %v", dir, line, err)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Repo{}, err
	}

	switch {
	case r.Branch == "(detached)":
		r.Branch = ""
		short, err := git(ctx, dir, "rev-parse", "--short", oid)
		if err != nil {
			return Repo{}, err
		}
		r.Detached = strings.TrimSpace(string(short))
	case !tracked:
		// No upstream, or one whose remote branch is gone: count what
		// would be lost if this clone disappeared.
		r.Upstream = ""
		if oid != "(initial)" {
			if r.Ahead, err = count(ctx, dir, "rev-list", "--count", "HEAD", "--not", "--remotes"); err != nil {
				return Repo{}, err
			}
		}
	}

	if r.FetchedAt, err = fetchedAt(ctx, dir); err != nil {
		return Repo{}, err
	}
	return r, nil
}

// fetchedAt returns the mtime of the repo's FETCH_HEAD, or the zero time
// when it has none.
func fetchedAt(ctx context.Context, dir string) (time.Time, error) {
	out, err := git(ctx, dir, "rev-parse", "--git-path", "FETCH_HEAD")
	if err != nil {
		return time.Time{}, err
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// count runs a git command that prints one number.
func count(ctx context.Context, dir string, args ...string) (int, error) {
	out, err := git(ctx, dir, args...)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("%s: git %s: %v", dir, args[0], err)
	}
	return n, nil
}

// git runs git in dir without taking optional locks, so it never competes
// with git commands the user or an agent is running in the same repo.
func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
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

