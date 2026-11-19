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

	"github.com/crankshift/githerd/internal/discovery"
	"github.com/crankshift/githerd/internal/fetch"
	"github.com/crankshift/githerd/internal/paths"
	"github.com/crankshift/githerd/internal/status"
)

// source is the --source of every report githerd makes.
const source = "githerd"

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

// Result is what a sync found and reported, or what Scan or Look found.
type Result struct {
	// Ran is false when the sync did not run: the event called for none,
	// or another sync was running and runs once more in its place. It is
	// false for Scan and Look, which report nothing.
	Ran        bool
	Roots      []Root      // the configured roots, in roots-file order
	Workspaces []Workspace // every workspace of the session, in sidebar order
	// Errs are the problems that did not stop the sync, such as a repo
	// that could not be read or a report Herdr refused.
	Errs []error
}

// Root is a configured root and the state of its repos.
type Root struct {
	Path string // as the roots file lists it, with ~ expanded
	// Dir is Path with symlinks resolved, so it matches pane cwds however
	// they are spelled. The repos are found under it, so their dirs have
	// symlinks resolved too: discovery follows none below the root.
	Dir   string
	Depth int           // how many directory levels below the root its repos are found
	Repos []status.Repo // in path order, the root first when it is a repo; Stale is set
}

// Workspace is a Herdr workspace and where its first pane's cwd lies.
type Workspace struct {
	ID    string
	Label string
	Root  string // Dir of the root containing the cwd; empty when outside every root
	Repo  string // Dir of the deepest nested repo containing the cwd; empty for a root workspace
}

// Scan reads the repos under the configured roots from disk, as a sync
// does before its first round of reports: their state, and whether their
// last fetch failed. It fetches nothing and asks Herdr nothing, so it is
// quick. What went wrong is in the Result's Errs.
func Scan(ctx context.Context, env Env) Result {
	var res Result
	res.Roots, _, res.Errs = scanRoots(ctx, env.ConfigDir)
	return res
}

// Look is Scan with the workspaces of the session, mapped through the
// snapshot as Sync maps them. It reports nothing and takes no lock, so it
// can run while a sync does. It fails when it can't read the snapshot.
func Look(ctx context.Context, env Env) (Result, error) {
	res := Scan(ctx, env)
	snap, err := cli{bin: env.Bin}.snapshot(ctx)
	if err != nil {
		return res, err
	}
	res.Workspaces, _ = mapWorkspaces(res.Roots, snap)
	return res, nil
}

// Sync reports the state of every repo under the configured roots to the
// workspaces of the Herdr session. For an agent status change it does
// nothing unless the agent turned done or idle.
//
// It reports what the repos hold locally first, then fetches the repos due
// for it with the 15-minute TTL, and reports again to the workspaces whose
// tokens that changed. The Result holds the state after the fetches.
//
// Hooks fire in bursts, so one sync runs at a time, under a lock in the
// state dir. A sync that finds the lock held leaves a mark and returns at
// once; the running sync finds the mark when it finishes and runs once more,
// so a burst costs two syncs and its last trigger is not lost. When another
// sync takes the lock first, that one runs once more instead, and Sync
// returns the run it did.
//
// The repos whose dirs are in force are fetched whatever the TTL, as the
// rail's f asks. A sync with any waits for the lock instead of leaving the
// mark, so that its fetches run: under the lock, they never race another
// sync's fetch of the same repo. The rerun fetches with the TTL alone.
func Sync(ctx context.Context, env Env, force ...string) (Result, error) {
	if !worthSyncing(env.Event) {
		return Result{}, nil
	}
	if err := os.MkdirAll(env.StateDir, 0o755); err != nil {
		return Result{}, err
	}
	var l *syncLock
	var err error
	if len(force) > 0 {
		l, err = waitLock(ctx, env.StateDir)
	} else {
		l, err = lock(env.StateDir)
	}
	if err != nil || l == nil {
		return Result{}, err
	}
	for {
		res, err := syncOnce(ctx, env, force)
		res.Ran = true
		rerun := l.unlock()
		if err != nil || !rerun {
			return res, err
		}
		force = nil
		if l, err = lock(env.StateDir); err != nil || l == nil {
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
func syncOnce(ctx context.Context, env Env, force []string) (Result, error) {
	var res Result
	var dirs []string
	res.Roots, dirs, res.Errs = scanRoots(ctx, env.ConfigDir)
	herdr := cli{bin: env.Bin}
	snap, err := herdr.snapshot(ctx)
	if err != nil {
		return res, err
	}
	var targets []target
	res.Workspaces, targets = mapWorkspaces(res.Roots, snap)

	sent, errs, err := reportRound(ctx, herdr, env.StateDir, targets, nil)
	res.Errs = append(res.Errs, errs...)
	if err != nil {
		return res, err
	}
	res.Errs = append(res.Errs, fetchDue(ctx, res.Roots, dirs, force)...)
	_, errs, err = reportRound(ctx, herdr, env.StateDir, targets, sent)
	res.Errs = append(res.Errs, errs...)
	return res, err
}

// mapWorkspaces maps each workspace of snap through its first pane's cwd
// to the root and the repo of roots it lies in. It returns them with the
// targets of a sync's reports: the workspaces inside a root, and those
// outside every root that carry tokens githerd reported earlier, to clear.
// The targets point into roots, so they report what roots hold when they
// do.
func mapWorkspaces(roots []Root, snap snapshot) ([]Workspace, []target) {
	var workspaces []Workspace
	var targets []target
	for _, sw := range snap.Workspaces {
		w := Workspace{ID: sw.ID, Label: sw.Label}
		t := target{id: sw.ID}
		t.root, t.repo = locate(roots, snap.firstPaneCwd(sw.ID))
		if t.root != nil {
			w.Root = t.root.Dir
		}
		if t.repo != nil {
			w.Repo = t.repo.Dir
		}
		workspaces = append(workspaces, w)
		if t.root != nil || hasAny(sw.Tokens, tokenNames) {
			targets = append(targets, t)
		}
	}
	return workspaces, targets
}

// target is a workspace githerd reports to: a mapped workspace with its
// repo, a root workspace with its root, or, with neither, a workspace
// outside every root whose tokens are cleared.
type target struct {
	id   string
	root *Root
	repo *status.Repo
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

// reportRound sends each target its tokens, all under one new --seq, and
// returns the tokens of every target. With sent, the tokens of an earlier
// round, it skips the targets whose tokens are unchanged, and takes no
// --seq when none changed. A report Herdr refuses is among the errs.
func reportRound(ctx context.Context, herdr cli, stateDir string, targets []target, sent map[string]map[string]string) (now map[string]map[string]string, errs []error, err error) {
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

// fetchDue fetches the repos in dirs due for it, and those in force
// whatever the TTL, and rereads those it fetched, updating the repos of
// roots in place: their state, and whether they are stale.
func fetchDue(ctx context.Context, roots []Root, dirs, force []string) []error {
	forced := map[string]bool{}
	for _, dir := range force {
		forced[dir] = true
	}
	var now, due []string
	for _, dir := range dirs {
		if forced[dir] {
			now = append(now, dir)
		} else {
			due = append(due, dir)
		}
	}
	outcomes := append(fetch.Run(ctx, now, fetch.Options{Force: true}), fetch.Run(ctx, due, fetch.Options{TTL: fetch.DefaultTTL})...)
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
		for j, r := range roots[i].Repos {
			if read, ok := byDir[r.Dir]; ok { // a repo that can't be reread keeps its state
				roots[i].Repos[j] = read
			}
		}
		fetch.MarkStale(roots[i].Repos, outcomes)
	}
	return errs
}

// scanRoots reads the roots file in configDir, finds the repos under each
// root and reads their state. It returns the roots, the dirs of all their
// repos, and what went wrong. Roots that nest share their repos, which are
// read once and listed once in dirs.
func scanRoots(ctx context.Context, configDir string) ([]Root, []string, []error) {
	roots, errs := readRoots(RootsFile(configDir))
	found := make([][]string, len(roots)) // repo dirs of each root
	var dirs []string                     // repo dirs of every root, once each
	seen := map[string]bool{}
	for i := range roots {
		roots[i].Dir = canonical(roots[i].Path)
		var err error
		if found[i], err = discovery.Repos(roots[i].Dir, roots[i].Depth); err != nil {
			errs = append(errs, fmt.Errorf("root %s: %w", roots[i].Path, err))
		}
		for _, dir := range found[i] {
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}

	repos, readErrs := status.ReadAll(ctx, dirs)
	errs = append(errs, readErrs...)
	fetch.MarkStale(repos, fetch.Last(ctx, dirs))
	byDir := map[string]status.Repo{}
	for _, r := range repos {
		byDir[r.Dir] = r
	}

	for i := range roots {
		for _, dir := range found[i] {
			if r, ok := byDir[dir]; ok { // an unreadable repo is left out
				roots[i].Repos = append(roots[i].Repos, r)
			}
		}
	}
	return roots, dirs, errs
}

// canonical returns path cleaned and with symlinks resolved, or only
// cleaned when it cannot be resolved, such as a cwd that was removed. An
// empty path stays empty.
func canonical(path string) string {
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// locate returns the root that contains cwd, the deepest one when roots
// nest, and the deepest repo under it, other than the root itself, that
// contains cwd. Either is nil when nothing contains cwd, or cwd is unknown.
func locate(roots []Root, cwd string) (*Root, *status.Repo) {
	if !filepath.IsAbs(cwd) {
		return nil, nil
	}
	cwd = canonical(cwd)
	i := paths.Deepest(cwd, roots, func(r Root) string { return r.Dir })
	if i < 0 {
		return nil, nil
	}
	root := &roots[i]
	// The deepest repo is the root's own only when no nested repo holds cwd.
	j := paths.Deepest(cwd, root.Repos, func(r status.Repo) string { return r.Dir })
	if j < 0 || root.Repos[j].Dir == root.Dir {
		return root, nil
	}
	return root, &root.Repos[j]
}
