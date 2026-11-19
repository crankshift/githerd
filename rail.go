package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/crankshift/githerd/internal/herdr"
	"github.com/crankshift/githerd/internal/repopage"
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
	// The rail runs with the Herdr server's PATH, as the shells Herdr opens do.
	_, lazygitErr := exec.LookPath("lazygit")
	m, cmds := tui.New(tui.Options{
		Now:       time.Now(),
		Cwd:       cwd,
		RootsFile: tildePath(herdr.RootsFile(r.env.ConfigDir)),
		Pane:      pane,
		Lazygit:   lazygitErr == nil,
	})
	m, _ = tui.Update(m, tui.Resize{Width: w, Height: h})
	for {
		for _, c := range cmds {
			switch c := c.(type) {
			case tui.Quit:
				return
			case tui.FocusWorkspace:
				r.inTurn(ctx, func() tui.Msg { return failed(herdr.FocusWorkspace(ctx, r.env, c.ID)) })
			case tui.CreateWorkspace:
				r.inTurn(ctx, func() tui.Msg {
					id, err := herdr.CreateWorkspace(ctx, r.env, c.Dir, c.Label)
					msg := tui.Created{Dir: c.Dir, ID: id}
					if err != nil {
						msg.Err = err.Error()
					}
					return msg
				})
			case tui.Split:
				r.inTurn(ctx, func() tui.Msg { return failed(herdr.OpenShell(ctx, r.env, c.Pane, c.Dir, c.Run)) })
			case tui.Load:
				first := problems
				problems = nil
				r.background(ctx, func() tui.Msg {
					l := loaded(herdr.Scan(ctx, r.env))
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
				r.inTurn(ctx, func() tui.Msg { return failed(herdr.Zoom(ctx, r.env, c.Pane, zoomModes[c.Mode])) })
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

// zoomModes are the Herdr zoom modes of the model's.
var zoomModes = map[tui.ZoomMode]herdr.ZoomMode{tui.ZoomOn: herdr.ZoomOn, tui.ZoomOff: herdr.ZoomOff, tui.ZoomToggle: herdr.ZoomToggle}

// failed returns the answer to a request that err tells failed, or nil
// when the request did not fail.
func failed(err error) tui.Msg {
	if err == nil {
		return nil
	}
	return tui.Failed{Err: err.Error()}
}

// background runs f on its own and hands its answer to the model, if it
// has one.
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

// refresh runs the refresh `githerd herdr sync` runs, fetching the repos in
// force whatever the TTL. When another sync holds the lock, that sync runs
// once more in this one's place, and the rail reads the repos from disk and
// the workspaces from the snapshot instead, which take no lock. A refresh
// with repos to force waits for the lock instead.
func refresh(ctx context.Context, env herdr.Env, force []string) tui.Loaded {
	res, err := herdr.Sync(ctx, env, force...)
	var lookErr error
	if !res.Ran {
		res, lookErr = herdr.Look(ctx, env)
	}
	l := loaded(res, err, lookErr)
	l.Refresh = true
	return l
}

// loaded is what the rail shows of res, read just now: the roots named
// with ~ for the home dir, and the workspaces when res holds them. errs
// are other problems to show.
func loaded(res herdr.Result, errs ...error) tui.Loaded {
	l := tui.Loaded{At: time.Now(), Problems: messages(res.Errs)}
	for _, err := range errs {
		if err != nil {
			l.Problems = append(l.Problems, err.Error())
		}
	}
	for _, root := range res.Roots {
		l.Roots = append(l.Roots, tui.Root{Name: tildePath(root.Path), Dir: root.Dir, Repos: root.Repos})
	}
	// A session always has a workspace, the rail's own; with none, the
	// snapshot could not be read.
	l.WorkspacesRead = len(res.Workspaces) > 0
	for _, w := range res.Workspaces {
		l.Workspaces = append(l.Workspaces, tui.Workspace{ID: w.ID, Label: w.Label, Root: w.Root, Repo: w.Repo})
	}
	return l
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
