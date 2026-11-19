// Package herdr keeps Herdr's sidebar in step with the repos under the
// configured roots. Sync refreshes every repo, maps each Herdr workspace to a
// root or a repo through the session snapshot, and reports the tokens: the
// root's summary to a root workspace, the repo's uncommitted count to a
// mapped workspace.
package herdr

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
}

// EnvFrom reads the Env from Herdr's plugin variables through getenv, such
// as os.Getenv. It fails when one is missing: outside Herdr there is no
// session to report to.
func EnvFrom(getenv func(string) string) (Env, error) {
	env := Env{
		Bin:       getenv("HERDR_BIN_PATH"),
		ConfigDir: getenv("HERDR_PLUGIN_CONFIG_DIR"),
		StateDir:  getenv("HERDR_PLUGIN_STATE_DIR"),
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
// workspaces of the Herdr session.
func Sync(ctx context.Context, env Env) (Result, error) {
	var res Result
	paths, errs := readRoots(filepath.Join(env.ConfigDir, rootsFile))
	res.Errs = append(res.Errs, errs...)
	herdr := cli{bin: env.Bin}
	snap, err := herdr.snapshot(ctx)
	if err != nil {
		return res, err
	}
	res.Roots, errs = scan(ctx, paths)
	res.Errs = append(res.Errs, errs...)

	seq := uint64(time.Now().UnixMilli())
	for _, sw := range snap.Workspaces {
		w := Workspace{ID: sw.ID, Label: sw.Label}
		root, repo := locate(res.Roots, snap.firstPaneCwd(sw.ID))
		var toks map[string]string
		switch {
		case repo != nil:
			w.Root, w.Repo = root.Path, repo.Dir
			toks = repoTokens(*repo)
		case root != nil:
			w.Root = root.Path
			toks = rootTokens(*root)
		case hasAny(sw.Tokens, tokenNames):
			toks = map[string]string{} // outside every root now: clear what githerd reported
		}
		res.Workspaces = append(res.Workspaces, w)
		if toks == nil {
			continue
		}
		if err := herdr.report(ctx, w.ID, seq, toks); err != nil {
			res.Errs = append(res.Errs, err)
		}
	}
	return res, nil
}

// scan finds the repos under the roots at paths, fetches those last fetched
// longer ago than the TTL, and reads their state. Roots that nest share
// their repos, which are fetched and read once.
func scan(ctx context.Context, paths []string) ([]Root, []error) {
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

	stale := map[string]error{}
	for _, o := range fetch.Run(ctx, dirs, fetch.Options{TTL: fetch.DefaultTTL}) {
		if o.Err != nil {
			stale[o.Dir] = o.Err
		}
	}
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
	return roots, errs
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
// contains cwd. Either is nil when nothing contains cwd.
func locate(roots []Root, cwd string) (*Root, *Repo) {
	if cwd == "" {
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
