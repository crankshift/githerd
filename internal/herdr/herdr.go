// Package herdr keeps Herdr's sidebar in step with the repos under the
// configured roots. Sync refreshes every repo, maps each Herdr workspace to a
// root or a repo through the session snapshot, and reports the tokens: the
// root's summary to a root workspace, the repo's uncommitted count to a
// mapped workspace.
package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/crankshift/githerd/internal/discovery"
	"github.com/crankshift/githerd/internal/fetch"
	"github.com/crankshift/githerd/internal/status"
)

// Source is the --source of every report githerd makes.
const Source = "githerd"

// Env is the Herdr plugin environment a sync runs in.
type Env struct {
	Bin       string // HERDR_BIN_PATH: the herdr CLI
	ConfigDir string // HERDR_PLUGIN_CONFIG_DIR: holds the roots file
	StateDir  string // HERDR_PLUGIN_STATE_DIR: holds the sync lock and the last --seq
	// Event is HERDR_PLUGIN_EVENT_JSON, the event a hook runs for; empty
	// when the sync was not triggered by an event.
	Event string
}

// EnvFrom reads the Env from Herdr's plugin variables through getenv, such
// as os.Getenv. It fails when one of the paths is missing: outside Herdr
// there is no session to report to.
func EnvFrom(getenv func(string) string) (Env, error) {
	env := Env{
		Bin:       getenv("HERDR_BIN_PATH"),
		ConfigDir: getenv("HERDR_PLUGIN_CONFIG_DIR"),
		StateDir:  getenv("HERDR_PLUGIN_STATE_DIR"),
		Event:     getenv("HERDR_PLUGIN_EVENT_JSON"),
	}
	for _, v := range []struct{ name, value string }{
		{"HERDR_BIN_PATH", env.Bin},
		{"HERDR_PLUGIN_CONFIG_DIR", env.ConfigDir},
		{"HERDR_PLUGIN_STATE_DIR", env.StateDir},
	} {
		if v.value == "" {
			return Env{}, fmt.Errorf("%s is not set; run this from Herdr", v.name)
		}
	}
	return env, nil
}

// Result is what a sync found and reported.
type Result struct {
	// Ran is false when the sync did not run: the event called for none,
	// or another sync was running and runs once more in its place.
	Ran        bool
	Roots      []Root      // the configured roots, in roots-file order
	Workspaces []Workspace // every workspace of the session, in sidebar order
	// Errs are the problems that did not stop the sync, such as a repo
	// that could not be read or a report Herdr refused.
	Errs []error
}

// Root is a configured root and the state of its repos.
type Root struct {
	Path  string
	Repos []Repo // in path order; the root first when it is a repo
	canon string // Path with symlinks resolved, for matching pane cwds
}

// Repo is the state of one repo under a root.
type Repo struct {
	status.Repo
	Stale error  // why its fetch failed, so behind may be out of date; nil when it didn't fail
	canon string // Dir with symlinks resolved
}

// Workspace is a Herdr workspace and where its first pane's cwd lies.
type Workspace struct {
	ID    string
	Label string
	Root  string // path of the root containing the cwd; empty when outside every root
	Repo  string // dir of the deepest nested repo containing the cwd; empty for a root workspace
}

// Sync reports the state of every repo under the configured roots to the
// workspaces of the Herdr session. For an agent status change it does
// nothing unless the agent turned done or idle.
//
// Hooks fire in bursts, so one sync runs at a time, under a lock in the
// state dir. A sync that finds the lock held leaves a mark and returns at
// once; the running sync finds the mark when it finishes and runs once more,
// so a burst costs two syncs and its last trigger is not lost.
func Sync(ctx context.Context, env Env) (Result, error) {
	if !worthSyncing(env.Event) {
		return Result{}, nil
	}
	if err := os.MkdirAll(env.StateDir, 0o755); err != nil {
		return Result{}, err
	}
	for {
		l, err := lock(env.StateDir)
		if err != nil || l == nil {
			return Result{}, err
		}
		res, err := syncOnce(ctx, env)
		res.Ran = true
		rerun := l.unlock()
		if err != nil || !rerun {
			return res, err
		}
	}
}

// worthSyncing reports whether the hook event in eventJSON can have changed
// what githerd reports. An agent that turned done or idle may have left
// changes behind; one that turned working or blocked has not finished yet.
func worthSyncing(eventJSON string) bool {
	var event struct {
		Event string `json:"event"`
		Data  struct {
			AgentStatus string `json:"agent_status"`
		} `json:"data"`
	}
	if eventJSON == "" || json.Unmarshal([]byte(eventJSON), &event) != nil {
		return true // a spare sync costs less than a missed one
	}
	if event.Event != "pane_agent_status_changed" {
		return true
	}
	return event.Data.AgentStatus == "done" || event.Data.AgentStatus == "idle"
}

// syncOnce runs one sync in two rounds of reports. The first reports what
// the repos hold locally, so a slow or hanging remote can't hold up the
// sidebar. Then the repos due for a fetch are fetched, and the second round
// reports again to the workspaces whose tokens that changed.
func syncOnce(ctx context.Context, env Env) (Result, error) {
	var res Result
	paths, errs := readRoots(filepath.Join(env.ConfigDir, rootsFile))
	res.Errs = append(res.Errs, errs...)
	var dirs []string
	res.Roots, dirs, errs = scan(ctx, paths)
	res.Errs = append(res.Errs, errs...)
	herdr := cli{bin: env.Bin}
	snap, err := herdr.snapshot(ctx)
	if err != nil {
		return res, err
	}

	var targets []target
	for _, sw := range snap.Workspaces {
		w := Workspace{ID: sw.ID, Label: sw.Label}
		t := target{id: sw.ID}
		t.root, t.repo = locate(res.Roots, snap.firstPaneCwd(sw.ID))
		if t.root != nil {
			w.Root = t.root.Path
		}
		if t.repo != nil {
			w.Repo = t.repo.Dir
		}
		res.Workspaces = append(res.Workspaces, w)
		// Outside every root, a workspace gets a report only to clear what
		// githerd reported to it earlier.
		if t.root != nil || hasAny(sw.Tokens, tokenNames) {
			targets = append(targets, t)
		}
	}

	sent, errs, err := report(ctx, herdr, env.StateDir, targets, nil)
	res.Errs = append(res.Errs, errs...)
	if err != nil {
		return res, err
	}
	res.Errs = append(res.Errs, fetchDue(ctx, res.Roots, dirs)...)
	_, errs, err = report(ctx, herdr, env.StateDir, targets, sent)
	res.Errs = append(res.Errs, errs...)
	return res, err
}

// target is a workspace githerd reports to: a mapped workspace with its
// repo, a root workspace with its root, or, with neither, a workspace
// outside every root whose tokens are cleared.
type target struct {
	id   string
	root *Root
	repo *Repo
}

func (t target) tokens() map[string]string {
	switch {
	case t.repo != nil:
		return repoTokens(*t.repo)
	case t.root != nil:
		return rootTokens(*t.root)
	}
	return map[string]string{}
}

// report sends each target its tokens, all under one new --seq, and
// returns the tokens of every target. With sent, the tokens of an earlier
// round, it skips the targets whose tokens are unchanged, and takes no
// --seq when none changed. A report Herdr refuses is among the errs.
func report(ctx context.Context, herdr cli, stateDir string, targets []target, sent map[string]map[string]string) (now map[string]map[string]string, errs []error, err error) {
	now = map[string]map[string]string{}
	var changed []target
	for _, t := range targets {
		now[t.id] = t.tokens()
		if old, ok := sent[t.id]; !ok || !maps.Equal(old, now[t.id]) {
			changed = append(changed, t)
		}
	}
	if len(changed) == 0 {
		return now, nil, nil
	}
	seq, err := nextSeq(stateDir)
	if err != nil {
		return now, nil, err
	}
	for _, t := range changed {
		if err := herdr.report(ctx, t.id, seq, now[t.id]); err != nil {
			errs = append(errs, err)
		}
	}
	return now, errs, nil
}

// fetchDue fetches the repos in dirs due for it and rereads those it
// fetched, updating the repos of roots in place: their state, and whether
// they are stale.
func fetchDue(ctx context.Context, roots []Root, dirs []string) []error {
	outcomes := fetch.Run(ctx, dirs, fetch.Options{TTL: fetch.DefaultTTL})
	stale := staleByDir(outcomes)
	var fetched []string
	for _, o := range outcomes {
		if o.Fetched {
			fetched = append(fetched, o.Dir)
		}
	}
	repos, errs := status.ReadAll(ctx, fetched)
	byDir := map[string]status.Repo{}
	for _, r := range repos {
		byDir[r.Dir] = r
	}
	for i := range roots {
		for j := range roots[i].Repos {
			r := &roots[i].Repos[j]
			if read, ok := byDir[r.Dir]; ok { // a repo that can't be reread keeps its state
				r.Repo = read
			}
			r.Stale = stale[r.Dir]
		}
	}
	return errs
}

// scan finds the repos under the roots at paths and reads their state, and
// returns the roots and the dirs of all their repos. Roots that nest share
// their repos, which are read once and listed once in dirs.
func scan(ctx context.Context, paths []string) ([]Root, []string, []error) {
	var errs []error
	found := make([][]string, len(paths)) // repo dirs of each root
	var dirs []string                     // repo dirs of every root, once each
	seen := map[string]bool{}
	for i, path := range paths {
		var err error
		if found[i], err = discovery.Repos(path, discovery.DefaultDepth); err != nil {
			errs = append(errs, fmt.Errorf("root %s: %w", path, err))
		}
		for _, dir := range found[i] {
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}

	stale := staleByDir(fetch.Last(ctx, dirs))
	repos, readErrs := status.ReadAll(ctx, dirs)
	errs = append(errs, readErrs...)
	byDir := map[string]status.Repo{}
	for _, r := range repos {
		byDir[r.Dir] = r
	}

	roots := make([]Root, len(paths))
	for i, path := range paths {
		roots[i] = Root{Path: path, canon: canonical(path)}
		for _, dir := range found[i] {
			if r, ok := byDir[dir]; ok { // an unreadable repo is left out
				roots[i].Repos = append(roots[i].Repos, Repo{Repo: r, Stale: stale[dir], canon: underCanon(roots[i], dir)})
			}
		}
	}
	return roots, dirs, errs
}

// staleByDir returns why each stale repo among outcomes is stale, by dir.
func staleByDir(outcomes []fetch.Outcome) map[string]error {
	stale := map[string]error{}
	for _, o := range outcomes {
		if o.Err != nil {
			stale[o.Dir] = o.Err
		}
	}
	return stale
}

// underCanon returns dir, which lies under root, with symlinks resolved.
// Discovery follows no symlinks below the root, so only the root's own
// path can hold any.
func underCanon(root Root, dir string) string {
	rel, err := filepath.Rel(root.Path, dir)
	if err != nil {
		return canonical(dir)
	}
	return filepath.Join(root.canon, rel)
}

// canonical returns path cleaned and with symlinks resolved, or only
// cleaned when it cannot be resolved, such as a cwd that was removed.
func canonical(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// locate returns the root that contains cwd, the deepest one when roots
// nest, and the deepest repo under it, other than the root itself, that
// contains cwd. Either is nil when nothing contains cwd, or cwd is unknown.
func locate(roots []Root, cwd string) (*Root, *Repo) {
	if !filepath.IsAbs(cwd) {
		return nil, nil
	}
	cwd = canonical(cwd)
	var root *Root
	for i := range roots {
		if within(cwd, roots[i].canon) && (root == nil || len(roots[i].canon) > len(root.canon)) {
			root = &roots[i]
		}
	}
	if root == nil {
		return nil, nil
	}
	var repo *Repo
	for i := range root.Repos {
		r := &root.Repos[i]
		if r.canon != root.canon && within(cwd, r.canon) && (repo == nil || len(r.canon) > len(repo.canon)) {
			repo = r
		}
	}
	return root, repo
}

// within reports whether path is dir or lies under it. Both are clean.
func within(path, dir string) bool {
	if path == dir {
		return true
	}
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return strings.HasPrefix(path, dir)
}
