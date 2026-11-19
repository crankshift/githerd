// Package fetch updates the remote state of repos with git fetch.
package fetch

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Options says which repos Run fetches.
type Options struct{}

// Outcome is what Run did for one repo.
type Outcome struct {
	Dir     string
	Remote  string // remote of the current branch's upstream; empty when there is none, so nothing was fetched
	Fetched bool   // git fetch ran and succeeded
	Err     error  // git fetch failed
}

// Run fetches the remote of each repo's upstream and returns one outcome
// per repo, in the order of dirs.
func Run(ctx context.Context, dirs []string, opts Options) []Outcome {
	outcomes := make([]Outcome, len(dirs))
	for i, dir := range dirs {
		outcomes[i] = one(ctx, dir)
	}
	return outcomes
}

func one(ctx context.Context, dir string) Outcome {
	o := Outcome{Dir: dir}
	o.Remote, o.Err = upstreamRemote(ctx, dir)
	if o.Err != nil || o.Remote == "" {
		return o
	}
	if _, o.Err = git(ctx, dir, "fetch", "--quiet", o.Remote); o.Err == nil {
		o.Fetched = true
	}
	return o
}

// upstreamRemote returns the remote of the current branch's upstream, or ""
// when HEAD is detached, the branch has no upstream, or the upstream is a
// local branch.
func upstreamRemote(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "for-each-ref", "--format=%(HEAD)%(upstream:remotename)", "refs/heads")
	if err != nil {
		return "", err
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if remote, ok := strings.CutPrefix(sc.Text(), "*"); ok && remote != "." {
			return remote, nil
		}
	}
	return "", sc.Err()
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", args[0], msg)
	}
	return out, nil
}
