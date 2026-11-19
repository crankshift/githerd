// Package repopage reads with git what the repo page of one repo shows: its
// changes, its commits to push and to pull, its branches and its log, and
// beside each of them a diff or a summary. It never writes to the repo.
package repopage

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/crankshift/githerd/internal/status"
)

// The most commits Read lists in each of ToPush and ToPull, and in Log.
const (
	maxCommits = 100
	logLength  = 10
)

// Page is what the repo page lists for a repo.
type Page struct {
	Changes []Change // unmerged, then staged, then unstaged, then untracked, each in path order

	Upstream string // the current branch's upstream, such as origin/main; empty when it has none
	Detached bool   // HEAD points at a commit rather than a branch
	// Ahead counts the commits to push: those the upstream lacks or, with
	// no upstream, those on no remote branch. Behind counts the commits to
	// pull, as of the last fetch.
	Ahead, Behind  int
	ToPush, ToPull []Commit // newest first, at most maxCommits of each
	Log            []Commit // the recent commits of HEAD that are not to push, newest first

	// Branches are the current branch, the other local branches in name
	// order, then the remote branches no local branch tracks.
	Branches []Branch
}

// Branch is a local branch, or a remote branch that no local branch tracks.
type Branch struct {
	Name          string // such as main, or origin/release/1.9 for a remote branch
	Ref           string // in full, such as refs/heads/main
	Current       bool   // HEAD is on it
	Remote        bool   // a remote branch that no local branch tracks
	Upstream      string // a local branch's upstream, such as origin/main; empty when it has none
	Gone          bool   // its upstream was set, but the remote branch is gone
	Ahead, Behind int    // the commits it has that its upstream lacks, and those it lacks
}

// Commit is one commit, as the page lists it.
type Commit struct {
	Hash    string // in full
	Short   string // abbreviated as git does, so it stays unique in the repo
	Author  string
	When    time.Time // when it was authored
	Subject string
}

// Change is a file changed on one side of the repo: in the index, in the
// working tree, untracked, or unmerged in both.
type Change struct {
	Area Area
	// Code is the change as `git status --short` shows it, for this side
	// alone: "M " staged, " M" unstaged, "??" untracked, "UU" unmerged.
	Code string
	Path string // slash-separated, from the repo's top
	Orig string // renamed or copied: the path it came from
}

// Area is the side of the repo a change is on.
type Area uint8

// The areas a change can be in.
const (
	Unmerged Area = iota + 1
	Staged
	Unstaged
	Untracked
)

// Read returns what the repo page lists for the repo in dir.
func Read(ctx context.Context, dir string) (Page, error) {
	r, entries, err := status.ReadEntries(ctx, dir)
	if err != nil {
		return Page{}, err
	}
	p := Page{Upstream: r.Upstream, Detached: r.Detached != "", Ahead: r.Ahead, Behind: r.Behind}
	if p.Changes, err = changes(entries); err != nil {
		return Page{}, fmt.Errorf("%s: git status: %v", dir, err)
	}
	if p.Branches, err = branches(ctx, dir); err != nil {
		return Page{}, err
	}
	if r.NoCommits {
		return p, nil
	}

	switch {
	case p.Detached:
	case p.Upstream != "":
		if p.ToPush, err = commits(ctx, dir, maxCommits, "@{upstream}..HEAD"); err != nil {
			return Page{}, err
		}
		if p.ToPull, err = commits(ctx, dir, maxCommits, "HEAD..@{upstream}"); err != nil {
			return Page{}, err
		}
	default:
		// No upstream, or one whose remote branch is gone: list what would
		// be lost if this repo disappeared, which Ahead counts.
		if p.ToPush, err = commits(ctx, dir, maxCommits, "HEAD", "--not", "--remotes"); err != nil {
			return Page{}, err
		}
	}

	log, err := commits(ctx, dir, logLength+len(p.ToPush), "HEAD")
	if err != nil {
		return Page{}, err
	}
	toPush := map[string]bool{}
	for _, c := range p.ToPush {
		toPush[c.Hash] = true
	}
	for _, c := range log {
		if !toPush[c.Hash] && len(p.Log) < logLength {
			p.Log = append(p.Log, c)
		}
	}
	return p, nil
}

// commitFormat is the git log format that commits parses.
const commitFormat = "--format=%H%x00%h%x00%an%x00%at%x00%s"

// commits returns at most n commits of the revisions in revs, newest first.
func commits(ctx context.Context, dir string, n int, revs ...string) ([]Commit, error) {
	args := append([]string{"log", "--no-color", "--no-show-signature", commitFormat, "-n", strconv.Itoa(n)}, revs...)
	out, err := status.Git(ctx, dir, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	var cs []Commit
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\x00", 5)
		if len(f) != 5 {
			return nil, fmt.Errorf("%s: git log: parsing %q", dir, line)
		}
		secs, err := strconv.ParseInt(f[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: git log: parsing %q: %v", dir, line, err)
		}
		cs = append(cs, Commit{Hash: f[0], Short: f[1], Author: f[2], When: time.Unix(secs, 0).UTC(), Subject: f[4]})
	}
	return cs, nil
}

// branches returns the repo's branches in the order Page lists them.
func branches(ctx context.Context, dir string) ([]Branch, error) {
	out, err := status.Git(ctx, dir, "for-each-ref",
		"--format=%(refname)%00%(HEAD)%00%(upstream)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(symref)",
		"refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var current, local, remote []Branch
	tracked := map[string]bool{} // the refs local branches track
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) != 6 {
			return nil, fmt.Errorf("%s: git for-each-ref: parsing %q", dir, line)
		}
		ref, head, upstream, short, track, symref := f[0], f[1], f[2], f[3], f[4], f[5]
		if name, ok := strings.CutPrefix(ref, "refs/remotes/"); ok {
			if symref == "" { // not origin/HEAD
				remote = append(remote, Branch{Name: name, Ref: ref, Remote: true})
			}
			continue
		}
		b := Branch{Name: strings.TrimPrefix(ref, "refs/heads/"), Ref: ref, Current: head == "*", Upstream: short}
		if upstream != "" {
			tracked[upstream] = true
		}
		for _, part := range strings.Split(track, ", ") {
			word, n, _ := strings.Cut(part, " ")
			switch word {
			case "gone":
				b.Gone = true
			case "ahead":
				b.Ahead, _ = strconv.Atoi(n)
			case "behind":
				b.Behind, _ = strconv.Atoi(n)
			}
		}
		if b.Current {
			current = append(current, b)
		} else {
			local = append(local, b)
		}
	}
	all := append(current, local...)
	for _, b := range remote {
		if !tracked[b.Ref] {
			all = append(all, b)
		}
	}
	return all, nil
}

// changes returns the changes that the entries of a git status list.
func changes(entries []status.Entry) ([]Change, error) {
	var unmerged, staged, unstaged, untracked []Change
	for _, entry := range entries {
		e := entry.Line
		var fields []string
		switch e[0] {
		case '1':
			fields = strings.SplitN(e, " ", 9)
		case '2':
			fields = strings.SplitN(e, " ", 10)
		case 'u':
			fields = strings.SplitN(e, " ", 11)
		case '?':
			untracked = append(untracked, Change{Area: Untracked, Code: "??", Path: e[2:]})
			continue
		default:
			continue
		}
		if len(fields) < 9 || len(fields[1]) != 2 {
			return nil, fmt.Errorf("parsing %q", e)
		}
		xy, path, orig := fields[1], fields[len(fields)-1], entry.Orig
		if e[0] == 'u' {
			unmerged = append(unmerged, Change{Area: Unmerged, Code: xy, Path: path})
			continue
		}
		side := func(code byte) Change {
			c := Change{Path: path}
			if code == 'R' || code == 'C' {
				c.Orig = orig
			}
			return c
		}
		if x := xy[0]; x != '.' {
			c := side(x)
			c.Area, c.Code = Staged, string(x)+" "
			staged = append(staged, c)
		}
		if y := xy[1]; y != '.' {
			c := side(y)
			c.Area, c.Code = Unstaged, " "+string(y)
			unstaged = append(unstaged, c)
		}
	}
	return append(append(append(unmerged, staged...), unstaged...), untracked...), nil
}
