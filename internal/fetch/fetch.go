// Package fetch updates the remote state of repos with git fetch.
package fetch

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/crankshift/githerd/internal/status"
)

// Defaults for Options.
const (
	DefaultTTL     = 15 * time.Minute // how long a fetch stays fresh
	DefaultTimeout = 20 * time.Second // how long one repo's fetch may take
)

// workers bounds how many repos are fetched at once.
const workers = 8

// Options says which repos Run fetches and how long each may take.
type Options struct {
	TTL     time.Duration // skip a repo fetched less than TTL ago
	Force   bool          // fetch every repo, whatever the TTL
	Timeout time.Duration // per repo; zero means DefaultTimeout
}

// Outcome is what Run did for one repo.
type Outcome struct {
	Dir     string
	Remote  string // remote of the current branch's upstream; empty when there is none, so nothing was fetched
	Fetched bool   // git fetch ran and succeeded; false when the repo was skipped
	Err     error  // why the fetch failed, in one line; the repo is stale
}

// Run fetches the remote of each repo's upstream and returns one outcome per
// repo, in the order of dirs. Repos are fetched in parallel, a few at a time,
// and each fetch that outlives opts.Timeout is killed and counts as failed.
//
// It skips a repo fetched less than opts.TTL ago, unless opts.Force is set.
// The clock is the mtime of FETCH_HEAD. Git empties FETCH_HEAD when a fetch
// fails, so a stale repo is fetched again whatever the TTL.
func Run(ctx context.Context, dirs []string, opts Options) []Outcome {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	outcomes := make([]Outcome, len(dirs))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, dir := range dirs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			outcomes[i] = one(ctx, dir, opts)
		}()
	}
	wg.Wait()
	return outcomes
}

func one(ctx context.Context, dir string, opts Options) Outcome {
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	o := fetchOne(ctx, dir, opts)
	if o.Err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		o.Err = fmt.Errorf("timed out after %v", opts.Timeout)
	}
	return o
}

func fetchOne(ctx context.Context, dir string, opts Options) Outcome {
	o := Outcome{Dir: dir}
	o.Remote, o.Err = upstreamRemote(ctx, dir)
	if o.Err != nil || o.Remote == "" {
		return o
	}
	if !opts.Force {
		var last time.Time
		if last, o.Err = status.FetchedAt(ctx, dir); o.Err != nil || time.Since(last) < opts.TTL {
			return o
		}
	}
	_, o.Err = git(ctx, dir, "fetch", "--quiet", o.Remote)
	o.Fetched = o.Err == nil
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

// git runs git in dir. Its error is the first line git printed on stderr:
// git puts the cause there and generic advice after it.
//
// Git runs in a session of its own, so when ctx ends, the whole process
// group is killed: ssh and other helpers git started die with it instead of
// holding its output open.
func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		for _, line := range strings.Split(stderr.String(), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				return nil, errors.New(line)
			}
		}
		return nil, err
	}
	return out, nil
}
