// Package fetch updates the remote state of repos with git fetch.
package fetch

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
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

// errLastFailed is why a repo is stale when its last fetch failed and was
// not retried: the reason it failed is not kept.
var errLastFailed = errors.New("last fetch failed")

// Options says which repos Run fetches and how long each may take.
type Options struct {
	TTL     time.Duration // skip a repo fetched, or whose fetch failed, less than TTL ago
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
// The clock is the mtime of FETCH_HEAD. A failed fetch leaves FETCH_HEAD
// empty, so a repo whose fetch failed less than opts.TTL ago is skipped too,
// and stays stale: its Err says the last fetch failed.
func Run(ctx context.Context, dirs []string, opts Options) []Outcome {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	return each(dirs, func(dir string) Outcome { return one(ctx, dir, opts) })
}

// Last returns, without fetching anything, the outcome Run would give each
// repo in dirs if none were due: Err is set for a repo whose last fetch
// failed. It shows which repos are stale before Run has had its say.
func Last(ctx context.Context, dirs []string) []Outcome {
	return each(dirs, func(dir string) Outcome {
		o, _, err := lastFetch(ctx, dir)
		if err != nil {
			o.Err = err
		}
		return o
	})
}

// each runs do for every dir, a few at a time, and returns the outcomes in
// the order of dirs.
func each(dirs []string, do func(dir string) Outcome) []Outcome {
	outcomes := make([]Outcome, len(dirs))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, dir := range dirs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			outcomes[i] = do(dir)
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

// lastFetch returns the outcome of the last fetch of the repo in dir, and
// when it happened, without fetching. The outcome's Remote is empty when
// the repo has nothing to fetch, and its Err is set when the last fetch
// failed. err is set when git can't tell.
func lastFetch(ctx context.Context, dir string) (o Outcome, at time.Time, err error) {
	o = Outcome{Dir: dir}
	if o.Remote, err = upstreamRemote(ctx, dir); err != nil || o.Remote == "" {
		return o, time.Time{}, err
	}
	at, failed, err := status.LastFetch(ctx, dir)
	if failed {
		o.Err = errLastFailed
	}
	return o, at, err
}

func fetchOne(ctx context.Context, dir string, opts Options) Outcome {
	o, last, err := lastFetch(ctx, dir)
	switch {
	case err != nil:
		o.Err = err
		return o
	case o.Remote == "", !opts.Force && time.Since(last) < opts.TTL:
		return o
	}
	var env []string
	if env, o.Err = fetchEnv(ctx, dir); o.Err != nil {
		return o
	}
	var head string
	if head, o.Err = status.FetchHead(ctx, dir); o.Err != nil {
		return o
	}
	_, o.Err = gitEnv(ctx, dir, env, "fetch", "--quiet", o.Remote)
	o.Fetched = o.Err == nil
	if o.Err != nil {
		markFailed(head)
	}
	return o
}

// markFailed leaves the FETCH_HEAD at path empty, with now as its mtime, so
// the TTL counts from the failure. Git empties FETCH_HEAD when a fetch
// starts, but a fetch that hangs until it is killed leaves the start time,
// and one that fails late can leave some of what it fetched. If the file
// can't be written, the next run simply fetches again.
func markFailed(path string) {
	now := time.Now()
	if os.WriteFile(path, nil, 0o644) == nil {
		os.Chtimes(path, now, now)
	}
}

// batchSSH is the SSH command for a fetch when the user has set none: in
// batch mode, ssh asks for no password, passphrase or host key confirmation.
const batchSSH = "ssh -o BatchMode=yes"

// fetchEnv returns the environment for git fetch in dir, so that it never
// prompts: terminal prompts are off, and ssh runs in batch mode unless the
// user has set their own SSH command, which is left as it is.
func fetchEnv(ctx context.Context, dir string) ([]string, error) {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" {
		return env, nil
	}
	own, err := git(ctx, dir, "config", "--default=", "--get", "core.sshCommand")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(own)) == "" {
		env = append(env, "GIT_SSH_COMMAND="+batchSSH)
	}
	return env, nil
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

// git runs a git command that only reads the repo in dir, with the
// caller's environment. It takes no optional locks, so it never competes
// with git commands the user or an agent runs there.
func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return gitEnv(ctx, dir, nil, append([]string{"--no-optional-locks"}, args...)...)
}

// gitEnv runs git in dir with env, or the caller's environment when env is
// nil. Its error is the first line git printed on stderr: git puts the cause
// there and generic advice after it.
//
// Git reads nothing from stdin and runs in a session of its own, with no
// controlling terminal to prompt on. When ctx ends, the whole session is
// killed: ssh and other helpers git started die with it instead of holding
// its output open.
func gitEnv(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = env
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
