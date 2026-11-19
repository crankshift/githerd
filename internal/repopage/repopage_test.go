package repopage_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
	"github.com/crankshift/githerd/internal/repopage"
)

// fixture returns a fresh fixture whose hermetic environment the test's own
// process uses, so the git that repopage runs sees no user config.
func fixture(t *testing.T) *gitfixture.Fixture {
	t.Helper()
	f := gitfixture.New(t)
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "GIT_") {
			t.Setenv(k, "")
			os.Unsetenv(k)
		}
	}
	for _, kv := range f.Env() {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	return f
}

// read reads the page of the repo in dir and fails the test on error.
func read(t *testing.T, dir string) repopage.Page {
	t.Helper()
	p, err := repopage.Read(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Changes are listed staged first, then unstaged, then untracked, each in
// path order, with the code `git status --short` shows for that side alone.
// A file changed in both the index and the working tree is listed on each
// side; a staged rename keeps the path it had.
func TestReadListsStagedThenUnstagedThenUntrackedChanges(t *testing.T) {
	f := fixture(t)
	api := f.Clone(f.Remote("api"), "work/api")
	f.Commit(api, "src/auth.go", "package auth\n", "Add auth")
	f.Commit(api, "src/old.go", "package auth\n\ntype Session struct{}\n", "Add sessions")
	f.Commit(api, "LICENSE", "MIT\n", "Add licence")
	f.Write(api, "src/routes.go", "package routes\n")
	f.Git(api, "add", "src/routes.go")
	f.Write(api, "src/auth.go", "package auth\n\nfunc Login() {}\n")
	f.Git(api, "add", "src/auth.go")
	f.Write(api, "src/auth.go", "package auth\n\nfunc Login() {}\n\nfunc Logout() {}\n")
	f.Git(api, "mv", "src/old.go", "src/session.go")
	f.Write(api, "README.md", "# api, edited\n")
	if err := os.Remove(api + "/LICENSE"); err != nil {
		t.Fatal(err)
	}
	f.Write(api, "src/auth_test.go", "package auth\n")
	f.Write(api, "notes/todo.md", "- refresh tokens\n")

	got := read(t, api).Changes
	want := []repopage.Change{
		{Area: repopage.Staged, Code: "M ", Path: "src/auth.go"},
		{Area: repopage.Staged, Code: "A ", Path: "src/routes.go"},
		{Area: repopage.Staged, Code: "R ", Path: "src/session.go", Orig: "src/old.go"},
		{Area: repopage.Unstaged, Code: " D", Path: "LICENSE"},
		{Area: repopage.Unstaged, Code: " M", Path: "README.md"},
		{Area: repopage.Unstaged, Code: " M", Path: "src/auth.go"},
		{Area: repopage.Untracked, Code: "??", Path: "notes/todo.md"},
		{Area: repopage.Untracked, Code: "??", Path: "src/auth_test.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("changes:\n got %+v\nwant %+v", got, want)
	}
}

// show returns what the page shows beside it and fails the test on error.
func show(t *testing.T, dir string, it repopage.Item) repopage.Detail {
	t.Helper()
	d, err := repopage.Show(context.Background(), dir, it)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Line kinds, short for the tables below.
const (
	hunk = repopage.Hunk
	same = repopage.Context
	add  = repopage.Added
	del  = repopage.Removed
	meta = repopage.Meta
)

// Beside a change the page shows the diff of that side alone, with the old
// and new line number of each line. An untracked file shows in full as
// added; a deleted one in full as removed.
func TestShowDiffsAChangeOnItsOwnSide(t *testing.T) {
	f := fixture(t)
	api := f.Clone(f.Remote("api"), "work/api")
	f.Commit(api, "src/auth.go", "package auth\n", "Add auth")
	f.Commit(api, "LICENSE", "MIT\n", "Add licence")
	f.Write(api, "src/auth.go", "package auth\n\nfunc Login() {}\n")
	f.Git(api, "add", "src/auth.go")
	f.Write(api, "src/auth.go", "package auth\n\nfunc Login() {}\n\nfunc Logout() {}\n")
	if err := os.Remove(api + "/LICENSE"); err != nil {
		t.Fatal(err)
	}
	f.Write(api, "notes/todo.md", "- refresh tokens\n- revoke sessions")

	for _, c := range []struct {
		change repopage.Change
		want   []repopage.DiffLine
	}{
		{repopage.Change{Area: repopage.Staged, Code: "M ", Path: "src/auth.go"}, []repopage.DiffLine{
			{Kind: hunk, Text: "@@ -1 +1,3 @@"},
			{Kind: same, Text: "package auth", Old: 1, New: 1},
			{Kind: add, Text: "", New: 2},
			{Kind: add, Text: "func Login() {}", New: 3},
		}},
		{repopage.Change{Area: repopage.Unstaged, Code: " M", Path: "src/auth.go"}, []repopage.DiffLine{
			{Kind: hunk, Text: "@@ -1,3 +1,5 @@"},
			{Kind: same, Text: "package auth", Old: 1, New: 1},
			{Kind: same, Text: "", Old: 2, New: 2},
			{Kind: same, Text: "func Login() {}", Old: 3, New: 3},
			{Kind: add, Text: "", New: 4},
			{Kind: add, Text: "func Logout() {}", New: 5},
		}},
		{repopage.Change{Area: repopage.Untracked, Code: "??", Path: "notes/todo.md"}, []repopage.DiffLine{
			{Kind: meta, Text: "new file mode 100644"},
			{Kind: hunk, Text: "@@ -0,0 +1,2 @@"},
			{Kind: add, Text: "- refresh tokens", New: 1},
			{Kind: add, Text: "- revoke sessions", New: 2},
			{Kind: meta, Text: `\ No newline at end of file`},
		}},
		{repopage.Change{Area: repopage.Unstaged, Code: " D", Path: "LICENSE"}, []repopage.DiffLine{
			{Kind: meta, Text: "deleted file mode 100644"},
			{Kind: hunk, Text: "@@ -1 +0,0 @@"},
			{Kind: del, Text: "MIT", Old: 1},
		}},
	} {
		got := show(t, api, repopage.Item{Change: c.change})
		want := repopage.Detail{Files: []repopage.FileDiff{{Path: c.change.Path, Lines: c.want}}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("beside %q %s:\n got %+v\nwant %+v", c.change.Code, c.change.Path, got, want)
		}
	}
}

// subjects returns the subject of each commit.
func subjects(commits []repopage.Commit) []string {
	var out []string
	for _, c := range commits {
		out = append(out, c.Subject)
	}
	return out
}

// assertSubjects fails unless commits have the subjects want, in order.
func assertSubjects(t *testing.T, what string, commits []repopage.Commit, want ...string) {
	t.Helper()
	if got := subjects(commits); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: %q, want %q", what, got, want)
	}
}

// fixtureTime is when every fixture commit was made.
var fixtureTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// A branch with an upstream lists the commits to push and to pull, newest
// first, with how many there are. The log holds the other recent commits of
// HEAD, so none is listed twice.
func TestReadListsCommitsToPushAndToPullAndTheLog(t *testing.T) {
	f := fixture(t)
	remote := f.Remote("api")
	api := f.Clone(remote, "work/api")
	f.Commit(api, "auth.go", "package auth\n", "Add login")
	f.Git(api, "push", "--quiet")
	f.Commit(api, "refresh.go", "package auth\n", "Add session refresh endpoint")
	f.Commit(api, "validate.go", "package auth\n", "Validate login payload")
	f.PushElsewhere(remote, "main", 3)
	f.Git(api, "fetch", "--quiet")

	p := read(t, api)
	if p.Upstream != "origin/main" || p.Ahead != 2 || p.Behind != 3 {
		t.Errorf("upstream %q, ahead %d, behind %d; want origin/main, 2, 3", p.Upstream, p.Ahead, p.Behind)
	}
	assertSubjects(t, "to push", p.ToPush, "Validate login payload", "Add session refresh endpoint")
	assertSubjects(t, "to pull", p.ToPull, "Teammate change 3", "Teammate change 2", "Teammate change 1")
	assertSubjects(t, "log", p.Log, "Add login", "Initial commit")

	head := f.Git(api, "rev-parse", "HEAD")
	want := repopage.Commit{Hash: head, Short: head[:7], Author: "Ada Example", When: fixtureTime, Subject: "Validate login payload"}
	if got := p.ToPush[0]; got.Hash != want.Hash || got.Short != want.Short || got.Author != want.Author || !got.When.Equal(want.When) || got.Subject != want.Subject {
		t.Errorf("newest commit to push: %+v, want %+v", got, want)
	}
}

// With no upstream, the commits to push are those on no remote branch, as
// the rail counts them; there is nothing to pull. A detached HEAD has
// neither, and a repo with no commits yet has no log.
func TestReadListsLocalCommitsWithNoUpstream(t *testing.T) {
	f := fixture(t)
	cli := f.Clone(f.Remote("cli"), "work/cli")
	f.Git(cli, "switch", "--quiet", "--create", "spike/parser")
	f.Commit(cli, "lexer.go", "package cli\n", "WIP tokenizer")
	f.Commit(cli, "flags.go", "package cli\n", "Parse flags")

	p := read(t, cli)
	if p.Upstream != "" || p.Detached || p.Ahead != 2 || p.Behind != 0 {
		t.Errorf("upstream %q, detached %v, ahead %d, behind %d; want none, false, 2, 0", p.Upstream, p.Detached, p.Ahead, p.Behind)
	}
	assertSubjects(t, "local commits", p.ToPush, "Parse flags", "WIP tokenizer")
	assertSubjects(t, "to pull", p.ToPull)
	assertSubjects(t, "log", p.Log, "Initial commit")

	f.Git(cli, "switch", "--quiet", "--detach", "HEAD~1")
	p = read(t, cli)
	if !p.Detached || p.Ahead != 0 {
		t.Errorf("detached: detached %v, ahead %d; want true, 0", p.Detached, p.Ahead)
	}
	assertSubjects(t, "detached, to push", p.ToPush)
	assertSubjects(t, "detached, log", p.Log, "WIP tokenizer", "Initial commit")

	empty := f.Init("work/empty")
	f.Write(empty, "notes.md", "ideas\n")
	p = read(t, empty)
	if want := []repopage.Change{{Area: repopage.Untracked, Code: "??", Path: "notes.md"}}; !reflect.DeepEqual(p.Changes, want) || p.Ahead != 0 || p.Log != nil {
		t.Errorf("with no commits: %+v, want only the change %+v", p, want)
	}
}

// Beside a commit the page shows its whole message and the diff of every
// file it changed, renames included. A merge shows what it brought into its
// first parent.
func TestShowDiffsEveryFileOfACommit(t *testing.T) {
	f := fixture(t)
	api := f.Clone(f.Remote("api"), "work/api")
	f.Commit(api, "auth.go", "package auth\n", "Add auth")
	f.Commit(api, "session.go", "package auth\n\ntype Session struct{}\n", "Add sessions")
	f.Write(api, "auth.go", "package auth\n\nfunc Refresh() {}\n")
	f.Write(api, "refresh_test.go", "package auth_test\n")
	f.Git(api, "mv", "session.go", "sessions.go")
	f.Git(api, "add", "-A")
	f.Git(api, "commit", "--quiet", "-m", "Add session refresh endpoint", "-m", "Tokens last an hour.")

	got := show(t, api, repopage.Item{Commit: f.Git(api, "rev-parse", "HEAD")})
	want := repopage.Detail{Message: "Add session refresh endpoint\n\nTokens last an hour.", Files: []repopage.FileDiff{
		{Path: "auth.go", Lines: []repopage.DiffLine{
			{Kind: hunk, Text: "@@ -1 +1,3 @@"},
			{Kind: same, Text: "package auth", Old: 1, New: 1},
			{Kind: add, Text: "", New: 2},
			{Kind: add, Text: "func Refresh() {}", New: 3},
		}},
		{Path: "refresh_test.go", Lines: []repopage.DiffLine{
			{Kind: meta, Text: "new file mode 100644"},
			{Kind: hunk, Text: "@@ -0,0 +1 @@"},
			{Kind: add, Text: "package auth_test", New: 1},
		}},
		{Path: "sessions.go", Orig: "session.go", Lines: []repopage.DiffLine{
			{Kind: meta, Text: "similarity index 100%"},
			{Kind: meta, Text: "rename from session.go"},
			{Kind: meta, Text: "rename to sessions.go"},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("beside the commit:\n got %+v\nwant %+v", got, want)
	}

	f.Git(api, "switch", "--quiet", "--create", "fix/expiry", "HEAD~1")
	f.Commit(api, "expiry.go", "package auth\n", "Fix token expiry")
	f.Git(api, "switch", "--quiet", "main")
	f.Git(api, "merge", "--quiet", "--no-ff", "-m", "Merge fix/expiry", "fix/expiry")
	got = show(t, api, repopage.Item{Commit: f.Git(api, "rev-parse", "HEAD")})
	want = repopage.Detail{Message: "Merge fix/expiry", Files: []repopage.FileDiff{
		{Path: "expiry.go", Lines: []repopage.DiffLine{
			{Kind: meta, Text: "new file mode 100644"},
			{Kind: hunk, Text: "@@ -0,0 +1 @@"},
			{Kind: add, Text: "package auth", New: 1},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("beside the merge:\n got %+v\nwant %+v", got, want)
	}
}

// Branches lists the current branch first, then the other local branches,
// each with its upstream and how far ahead and behind it is, then the
// remote branches no local branch tracks.
func TestReadListsLocalAndRemoteOnlyBranches(t *testing.T) {
	f := fixture(t)
	remote := f.Remote("api")
	api := f.Clone(remote, "work/api")
	f.Git(api, "push", "--quiet", "origin", "main:release/1.9")
	f.Git(api, "branch", "fix/token-expiry")
	f.Git(api, "switch", "--quiet", "--create", "old/feature")
	f.Git(api, "push", "--quiet", "--set-upstream", "origin", "old/feature")
	f.Git(api, "push", "--quiet", "origin", "--delete", "old/feature")
	f.Git(api, "switch", "--quiet", "--create", "feature/login", "main")
	f.Commit(api, "login.go", "package auth\n", "Add login")
	f.Git(api, "push", "--quiet", "--set-upstream", "origin", "feature/login")
	f.Commit(api, "refresh.go", "package auth\n", "Add session refresh endpoint")
	f.PushElsewhere(remote, "main", 2)
	f.Git(api, "fetch", "--quiet")

	got := read(t, api).Branches
	want := []repopage.Branch{
		{Name: "feature/login", Ref: "refs/heads/feature/login", Current: true, Upstream: "origin/feature/login", Ahead: 1},
		{Name: "fix/token-expiry", Ref: "refs/heads/fix/token-expiry"},
		{Name: "main", Ref: "refs/heads/main", Upstream: "origin/main", Behind: 2},
		{Name: "old/feature", Ref: "refs/heads/old/feature", Upstream: "origin/old/feature", Gone: true},
		{Name: "origin/release/1.9", Ref: "refs/remotes/origin/release/1.9", Remote: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("branches:\n got %+v\nwant %+v", got, want)
	}
}

// Beside a branch the page shows its ten most recent commits, newest first,
// whether it is local or only on the remote.
func TestShowListsTheRecentCommitsOfABranch(t *testing.T) {
	f := fixture(t)
	remote := f.Remote("api")
	api := f.Clone(remote, "work/api")
	for i := 1; i <= 11; i++ {
		f.Commit(api, "log.txt", strings.Repeat("x\n", i), "Step "+strconv.Itoa(i))
	}
	f.Git(api, "push", "--quiet", "origin", "HEAD~9:refs/heads/release/1.9")
	f.Git(api, "fetch", "--quiet")

	got := show(t, api, repopage.Item{Branch: "refs/heads/main"})
	assertSubjects(t, "beside main", got.Commits, "Step 11", "Step 10", "Step 9", "Step 8", "Step 7", "Step 6", "Step 5", "Step 4", "Step 3", "Step 2")
	got = show(t, api, repopage.Item{Branch: "refs/remotes/origin/release/1.9"})
	assertSubjects(t, "beside origin/release/1.9", got.Commits, "Step 2", "Step 1", "Initial commit")
}

// A file a merge left in conflict is listed first, as unmerged, and shows
// the working tree against both sides; its lines have only new numbers.
func TestReadAndShowAFileInConflict(t *testing.T) {
	f := fixture(t)
	api := f.Clone(f.Remote("api"), "work/api")
	f.Commit(api, "auth.go", "base\n", "Add auth")
	f.Git(api, "switch", "--quiet", "--create", "fix/expiry")
	f.Commit(api, "auth.go", "theirs\n", "Fix expiry")
	f.Git(api, "switch", "--quiet", "main")
	f.Commit(api, "auth.go", "ours\n", "Rework auth")
	f.Write(api, "notes.md", "ideas\n")
	cmd := exec.Command("git", "merge", "--quiet", "fix/expiry")
	cmd.Dir, cmd.Env = api, f.Env()
	if err := cmd.Run(); err == nil {
		t.Fatal("the merge did not conflict")
	}

	changes := read(t, api).Changes
	conflict := repopage.Change{Area: repopage.Unmerged, Code: "UU", Path: "auth.go"}
	if want := []repopage.Change{conflict, {Area: repopage.Untracked, Code: "??", Path: "notes.md"}}; !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes:\n got %+v\nwant %+v", changes, want)
	}
	if got, want := show(t, api, repopage.Item{Change: conflict}).Files[0].Lines, []repopage.DiffLine{
		{Kind: hunk, Text: "@@@ -1,1 -1,1 +1,5 @@@"},
		{Kind: add, Text: "<<<<<<< HEAD", New: 1},
		{Kind: add, Text: "ours", New: 2},
		{Kind: add, Text: "=======", New: 3},
		{Kind: add, Text: "theirs", New: 4},
		{Kind: add, Text: ">>>>>>> fix/expiry", New: 5},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("beside the conflict:\n got %+v\nwant %+v", got, want)
	}
}

// A binary file shows as git words it, and a repo nested in the repo, which
// git lists as an untracked dir, as a repo of its own. A huge file shows
// its first lines, each cut short, and then says it was cut.
func TestShowCopesWithOddAndHugeFiles(t *testing.T) {
	f := fixture(t)
	api := f.Clone(f.Remote("api"), "work/api")
	if err := os.WriteFile(api+"/logo.png", []byte{0x89, 'P', 'N', 'G', 0, 0, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	f.Clone(f.Remote("sdk"), "work/api/vendor/sdk")
	f.Write(api, "dump.sql", strings.Repeat(strings.Repeat("x", 3000)+"\n", 6000))

	changes := read(t, api).Changes
	want := []repopage.Change{
		{Area: repopage.Untracked, Code: "??", Path: "dump.sql"},
		{Area: repopage.Untracked, Code: "??", Path: "logo.png"},
		{Area: repopage.Untracked, Code: "??", Path: "vendor/sdk/"},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes:\n got %+v\nwant %+v", changes, want)
	}

	if got, want := show(t, api, repopage.Item{Change: changes[1]}).Files[0].Lines, []repopage.DiffLine{
		{Kind: meta, Text: "new file mode 100644"},
		{Kind: meta, Text: "Binary files /dev/null and b/logo.png differ"},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("beside a binary file:\n got %+v\nwant %+v", got, want)
	}
	if got, want := show(t, api, repopage.Item{Change: changes[2]}).Files[0].Lines, []repopage.DiffLine{
		{Kind: meta, Text: "a repo of its own"},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("beside a nested repo:\n got %+v\nwant %+v", got, want)
	}

	lines := show(t, api, repopage.Item{Change: changes[0]}).Files[0].Lines
	if got, want := len(lines), 5000-4+1; got != want { // less diff --git, index, --- and +++, plus the cut
		t.Errorf("beside a 6000-line file: %d lines, want %d", got, want)
	}
	if got, want := lines[len(lines)-1], (repopage.DiffLine{Kind: meta, Text: "… cut after 5000 lines"}); got != want {
		t.Errorf("beside a 6000-line file, the last line is %+v, want %+v", got, want)
	}
	if got, want := lines[2], (repopage.DiffLine{Kind: add, Text: strings.Repeat("x", 1999), New: 1}); got != want {
		t.Errorf("beside a file of 3000-byte lines, the first line is %d bytes, want %d", len(got.Text), len(want.Text))
	}
}

// files returns every file under dir with its size, mode, mtime and
// content.
func files(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = fmt.Sprintf("%d %v %v %x", info.Size(), info.Mode(), info.ModTime().UnixNano(), sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Reading a page and every item on it never writes to the repo, not even
// to refresh the index after a file was touched.
func TestReadAndShowNeverWriteToTheRepo(t *testing.T) {
	f := fixture(t)
	remote := f.Remote("api")
	api := f.Clone(remote, "work/api")
	f.Commit(api, "auth.go", "package auth\n", "Add auth")
	f.Commit(api, "routes.go", "package auth\n", "Add routes")
	f.PushElsewhere(remote, "main", 1)
	f.Git(api, "fetch", "--quiet")
	f.Write(api, "routes.go", "package routes\n")
	f.Git(api, "add", "routes.go")
	f.Write(api, "notes.md", "ideas\n")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(api+"/auth.go", later, later); err != nil { // stat info the index doesn't match
		t.Fatal(err)
	}
	before := files(t, api)

	p := read(t, api)
	var items []repopage.Item
	for _, c := range p.Changes {
		items = append(items, repopage.Item{Change: c})
	}
	for _, c := range append(append(p.ToPush, p.ToPull...), p.Log...) {
		items = append(items, repopage.Item{Commit: c.Hash})
	}
	for _, b := range p.Branches {
		items = append(items, repopage.Item{Branch: b.Ref})
	}
	if len(items) < 7 {
		t.Fatalf("only %d items on the page: %+v", len(items), p)
	}
	for _, it := range items {
		show(t, api, it)
	}

	after := files(t, api)
	for path, was := range before {
		if now, ok := after[path]; !ok || now != was {
			t.Errorf("%s changed: %q, now %q", path, was, now)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("%s was created", path)
		}
	}
}
