// Package status reads the state of one repo with git, without writing to it.
package status

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Repo is the state of one repo.
type Repo struct {
	Dir         string // absolute path of the repo
	Branch      string // current branch
	Upstream    string // upstream of the current branch, such as origin/main; empty when there is none
	Uncommitted int    // porcelain entries in the working tree and index, untracked files included
	Ahead       int    // commits the current branch has that its upstream lacks
	Behind      int    // commits the upstream has that the current branch lacks, as of the last fetch
}

// NeedsAttention reports whether the repo is uncommitted, ahead, behind or
// no upstream.
func (r Repo) NeedsAttention() bool {
	return r.Uncommitted > 0 || r.Ahead > 0 || r.Behind > 0 || r.Upstream == ""
}

// Read returns the state of the repo in dir.
func Read(ctx context.Context, dir string) (Repo, error) {
	out, err := git(ctx, dir, "status", "--porcelain=v2", "--branch", "--untracked-files=all")
	if err != nil {
		return Repo{}, err
	}
	r := Repo{Dir: dir}
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
		case "branch.head":
			r.Branch = value
		case "branch.upstream":
			r.Upstream = value
		case "branch.ab":
			if _, err := fmt.Sscanf(value, "+%d -%d", &r.Ahead, &r.Behind); err != nil {
				return Repo{}, fmt.Errorf("%s: parsing %q: %v", dir, line, err)
			}
		}
	}
	return r, sc.Err()
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

