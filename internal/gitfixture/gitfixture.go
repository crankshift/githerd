// Package gitfixture builds real git repos in temp dirs for black-box tests:
// bare "remotes" plus clones arranged into every repo state githerd reports.
//
// Everything runs in a hermetic environment: no user or system git config, a
// temp HOME, and fixed author and committer identities and dates, so commit
// hashes are the same on every run.
package gitfixture

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Fixture is a temp dir holding bare remotes, scratch clones and a root.
type Fixture struct {
	t      testing.TB
	dir    string
	home   string
	env    []string
	pushes int // scratch clones made by PushElsewhere
}

// New creates an empty fixture in a fresh temp dir.
func New(t testing.TB) *Fixture {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	return &Fixture{t: t, dir: dir, home: home, env: Env(home)}
}

// Env returns a hermetic environment for git and githerd with home as HOME.
// Only PATH is taken from the calling process, so git can be found.
func Env(home string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Ada Example",
		"GIT_AUTHOR_EMAIL=ada@example.com",
		"GIT_AUTHOR_DATE=2026-01-02T03:04:05Z",
		"GIT_COMMITTER_NAME=Ada Example",
		"GIT_COMMITTER_EMAIL=ada@example.com",
		"GIT_COMMITTER_DATE=2026-01-02T03:04:05Z",
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
	}
}

// Env returns the fixture's hermetic environment.
func (f *Fixture) Env() []string { return append([]string(nil), f.env...) }

// Path returns an absolute path inside the fixture dir.
func (f *Fixture) Path(rel string) string { return filepath.Join(f.dir, filepath.FromSlash(rel)) }

// Git runs git in dir and returns its trimmed stdout. It fails the test on error.
func (f *Fixture) Git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = f.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		f.t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

// Remote creates a bare repo named name whose main branch holds one commit,
// and returns its path.
func (f *Fixture) Remote(name string) string {
	f.t.Helper()
	bare := f.Path("remotes/" + name + ".git")
	f.mkdir(bare)
	f.Git(bare, "init", "--quiet", "--bare", "--initial-branch=main")
	seed := f.Path("scratch/" + name)
	f.Git(f.dir, "clone", "--quiet", bare, seed)
	f.Git(seed, "checkout", "--quiet", "-B", "main")
	f.Commit(seed, "README.md", "# "+name+"\n", "Initial commit")
	f.Git(seed, "push", "--quiet", "origin", "main")
	return bare
}

// Init creates a repo with no remote and no commits at path, given relative
// to the fixture dir, and returns its absolute path.
func (f *Fixture) Init(path string) string {
	f.t.Helper()
	dst := f.Path(path)
	f.mkdir(dst)
	f.Git(dst, "init", "--quiet", "--initial-branch=main")
	return dst
}

// Clone clones remote into path, given relative to the fixture dir, and
// returns the clone's absolute path.
func (f *Fixture) Clone(remote, path string) string {
	f.t.Helper()
	dst := f.Path(path)
	f.mkdir(filepath.Dir(dst))
	f.Git(f.dir, "clone", "--quiet", remote, dst)
	return dst
}

// PushElsewhere adds n commits to branch of remote from a scratch clone, as
// a teammate would, so existing clones fall behind once they fetch.
func (f *Fixture) PushElsewhere(remote, branch string, n int) {
	f.t.Helper()
	f.pushes++
	other := f.Path(fmt.Sprintf("scratch/teammate-%d", f.pushes))
	f.Git(f.dir, "clone", "--quiet", "--branch", branch, remote, other)
	for i := 1; i <= n; i++ {
		f.Commit(other, fmt.Sprintf("teammate-%d.txt", i), "change\n", fmt.Sprintf("Teammate change %d", i))
	}
	f.Git(other, "push", "--quiet", "origin", branch)
}

// Write writes content to file inside repo, creating parent dirs.
func (f *Fixture) Write(repo, file, content string) {
	f.t.Helper()
	p := filepath.Join(repo, filepath.FromSlash(file))
	f.mkdir(filepath.Dir(p))
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// Commit writes file in repo and commits it with msg.
func (f *Fixture) Commit(repo, file, content, msg string) {
	f.t.Helper()
	f.Write(repo, file, content)
	f.Git(repo, "add", "--", file)
	f.Git(repo, "commit", "--quiet", "-m", msg)
}

func (f *Fixture) mkdir(dir string) {
	f.t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
}
