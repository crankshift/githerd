package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/crankshift/githerd/internal/discovery"
	"github.com/crankshift/githerd/internal/fetch"
	"github.com/crankshift/githerd/internal/herdr"
	"github.com/crankshift/githerd/internal/repopage"
	"github.com/crankshift/githerd/internal/status"
	"github.com/crankshift/githerd/internal/term"
	"github.com/crankshift/githerd/internal/tui"
)

// runRail runs the rail in the terminal of the pane Herdr opened for it.
func runRail(ctx context.Context, args []string, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: githerd rail")
		return 2
	}
	env, err := herdr.EnvFrom(os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "githerd rail:", err)
		return 2
	}
	t, err := term.Open(os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(stderr, "githerd rail:", err)
		return 1
	}
	// Closing the pane hangs up its terminal.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGHUP)
	defer stop()
	r := rail{env: env, term: t, msgs: make(chan tui.Msg)}
	r.run(ctx, os.Getenv("HERDR_PANE_ID"), herdr.OpenedFrom(os.Getenv("HERDR_PLUGIN_CONTEXT_JSON")))
	if err := t.Close(); err != nil {
		fmt.Fprintln(stderr, "githerd rail:", err)
		return 1
	}
	return 0
}

// rail is the terminal layer of the rail: it feeds keys, ticks and sizes to
// the model, carries out the model's commands, and draws its view.
type rail struct {
	env  herdr.Env
	term *term.Terminal
	msgs chan tui.Msg // keys, and the answers of commands run in the background
	work sync.WaitGroup
	turn chan struct{} // closed when the last Herdr request asked for has run
}

// run runs the rail until the model asks to quit or ctx ends. pane is the
// rail's own pane; cwd is the cwd of the pane it was opened from.
func (r *rail) run(ctx context.Context, pane, cwd string) {
	ctx, cancel := context.WithCancel(ctx)
	defer r.wait()
	defer cancel()

	var problems []string // shown with the first load
	if pane != "" {
		if err := herdr.MarkRail(ctx, r.env, pane); err != nil {
			problems = append(problems, err.Error())
		}
	}
	go r.readKeys(ctx)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)

	w, h, _ := r.term.Size()
	m, cmds := tui.New(tui.Options{Now: time.Now(), Cwd: canonical(cwd), Pane: pane, RootsFile: tildePath(herdr.RootsFile(r.env.ConfigDir))})
	m, _ = tui.Update(m, tui.Resize{Width: w, Height: h})
	for {
		for _, c := range cmds {
			switch c := c.(type) {
			case tui.Quit:
				return
			case tui.Load:
				first := problems
				problems = nil
				r.background(ctx, func() tui.Msg {
					l := load(ctx, r.env)
					l.Problems = append(first, l.Problems...)
					return l
				})
			case tui.Refresh:
				r.background(ctx, func() tui.Msg { return refresh(ctx, r.env, c.Force) })
			case tui.LoadPage:
				r.background(ctx, func() tui.Msg {
					p, err := repopage.Read(ctx, c.Dir)
					return tui.PageLoaded{Dir: c.Dir, Page: p, Err: errText(err)}
				})
			case tui.LoadDetail:
				r.background(ctx, func() tui.Msg {
					d, err := repopage.Show(ctx, c.Dir, c.Item)
					return tui.DetailLoaded{Dir: c.Dir, Item: c.Item, Detail: d, Err: errText(err)}
				})
			case tui.Zoom:
				r.inTurn(ctx, func() tui.Msg {
					if err := herdr.Zoom(ctx, r.env, c.Pane, string(c.Mode)); err != nil {
						return tui.Failed{Err: err.Error()}
					}
					return nil
				})
			}
		}
		r.term.Draw(tui.View(m, w, h).ANSI())
		select {
		case <-ctx.Done():
			return
		case msg := <-r.msgs:
			m, cmds = tui.Update(m, msg)
		case now := <-tick.C:
			m, cmds = tui.Update(m, tui.Tick{Now: now})
		case <-winch:
			w, h, _ = r.term.Size()
			m, cmds = tui.Update(m, tui.Resize{Width: w, Height: h})
		}
	}
}

// background runs f on its own and hands its answer, if it has one, to the
// model.
func (r *rail) background(ctx context.Context, f func() tui.Msg) {
	r.work.Add(1)
	go func() {
		defer r.work.Done()
		msg := f()
		if msg == nil {
			return
		}
		select {
		case r.msgs <- msg:
		case <-ctx.Done():
		}
	}()
}

// inTurn runs the Herdr request f on its own once the requests asked for
// before it have run, so they reach Herdr in the order the model made
// them: a zoom in and out in quick succession must not end zoomed in.
func (r *rail) inTurn(ctx context.Context, f func() tui.Msg) {
	prev, done := r.turn, make(chan struct{})
	r.turn = done
	r.background(ctx, func() tui.Msg {
		defer close(done)
		if prev != nil {
			select {
			case <-prev:
			case <-ctx.Done():
				return nil
			}
		}
		return f()
	})
}

// wait gives the commands still running, whose context has ended, a moment
// to stop the git and herdr processes they started.
func (r *rail) wait() {
	done := make(chan struct{})
	go func() { r.work.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// readKeys hands the keys typed on stdin to the model.
func (r *rail) readKeys(ctx context.Context) {
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		for _, k := range term.Keys(buf[:n]) {
			select {
			case r.msgs <- tui.Key(k):
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// load reads the repos under every root from disk, without fetching: their
// state, and whether their last fetch failed.
func load(ctx context.Context, env herdr.Env) tui.Loaded {
	paths, errs := herdr.ReadRoots(env.ConfigDir)
	l := tui.Loaded{Problems: messages(errs)}
	for _, path := range paths {
		dirs, err := discovery.Repos(path, discovery.DefaultDepth)
		if err != nil {
			l.Problems = append(l.Problems, fmt.Sprintf("root %s: %v", path, err))
		}
		stale := map[string]error{}
		for _, o := range fetch.Last(ctx, dirs) {
			if o.Err != nil {
				stale[o.Dir] = o.Err
			}
		}
		repos, errs := status.ReadAll(ctx, dirs)
		l.Problems = append(l.Problems, messages(errs)...)
		var hr []herdr.Repo
		for _, repo := range repos {
			hr = append(hr, herdr.Repo{Repo: repo, Stale: stale[repo.Dir]})
		}
		l.Roots = append(l.Roots, railRoot(path, hr))
	}
	l.At = time.Now()
	return l
}

// refresh runs the refresh `githerd herdr sync` runs, after fetching the
// repos in force whatever the TTL. When another sync holds the lock, that
// sync runs once more in this one's place, and the rail reads the repos from
// disk instead.
func refresh(ctx context.Context, env herdr.Env, force []string) tui.Loaded {
	if len(force) > 0 {
		fetch.Run(ctx, force, fetch.Options{Force: true}) // the sync reports the repos that failed
	}
	res, err := herdr.Sync(ctx, env)
	if !res.Ran {
		l := load(ctx, env)
		if err != nil {
			l.Problems = append(l.Problems, err.Error())
		}
		l.Refresh = true
		return l
	}
	l := tui.Loaded{Problems: messages(res.Errs), Refresh: true}
	if err != nil {
		l.Problems = append(l.Problems, err.Error())
	}
	for _, root := range res.Roots {
		l.Roots = append(l.Roots, railRoot(root.Path, root.Repos))
	}
	l.At = time.Now()
	return l
}

// railRoot returns the root at path with its repos as the rail shows them:
// named with ~ for the home dir, with symlinks resolved so they match the
// cwd the rail was opened from.
func railRoot(path string, repos []herdr.Repo) tui.Root {
	root := tui.Root{Name: tildePath(path), Dir: canonical(path)}
	for _, repo := range repos {
		r := tui.Repo{Repo: repo.Repo}
		if rel, err := filepath.Rel(path, repo.Dir); err == nil {
			r.Dir = filepath.Join(root.Dir, rel) // below the root, discovery follows no symlinks
		}
		if repo.Stale != nil {
			r.Stale = repo.Stale.Error()
		}
		root.Repos = append(root.Repos, r)
	}
	return root
}

// canonical returns path cleaned and with symlinks resolved, or only
// cleaned when it cannot be resolved.
func canonical(path string) string {
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// tildePath returns path with the home dir spelled ~.
func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// errText returns the message of err, or "" when it is nil.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func messages(errs []error) []string {
	var out []string
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}
