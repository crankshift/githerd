package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
)

// One table covers the state rules: untracked files count as uncommitted;
// ahead and behind come from the upstream; with no upstream, ahead counts the
// commits on no remote branch; detached alone does not need attention.
func TestStatusReportsEveryRepoStateSortedByPath(t *testing.T) {
	f := gitfixture.New(t)
	root := f.EveryState()
	pinned := f.Git(filepath.Join(root, gitfixture.DetachedRepo), "rev-parse", "--short", "HEAD")

	got := githerd(t, f.Env(), f.Path("."), "status", root)

	assertOutput(t, got, ""+
		"REPO                      BRANCH        UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
		".                         main          0            0      0       origin/main\n"+
		"repos/acme/api            main          3            0      0       origin/main\n"+
		"repos/acme/billing        main          0            2      0       origin/main\n"+
		"repos/acme/docs           main          0            0      0       origin/main\n"+
		"repos/acme/mobile         main          0            1      2       origin/main\n"+
		"repos/acme/web            main          0            0      4       origin/main\n"+
		"repos/tools/cli           spike/parser  0            3      0       -            no upstream\n"+
		"repos/tools/lint-rules    "+pinned+"       0            0      0       -            detached\n"+
		"sandbox/experiments       main          0            2      0       -            no upstream\n"+
		"sandbox/experiments/data  main          0            0      0       origin/main\n"+
		"6 of 10 repos need attention\n")
}

func TestStatusTreatsARepoWithNoCommitsYetAsNoUpstream(t *testing.T) {
	f := gitfixture.New(t)
	f.Init("work/sandbox/blank")

	assertOutput(t, githerd(t, f.Env(), f.Path("work"), "status"), ""+
		"REPO           BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM  NOTE\n"+
		"sandbox/blank  main    0            0      0       -         no upstream\n"+
		"1 of 1 repos need attention\n")
}

func TestStatusShowsDetachedCommitWithoutCountingItAsNeedingAttention(t *testing.T) {
	f := gitfixture.New(t)
	lint := f.Clone(f.Remote("lint-rules"), "work/repos/tools/lint-rules")
	f.Git(lint, "tag", "v1.4.0")
	f.Commit(lint, "rules.txt", "no-tabs\n", "Add rules")
	f.Git(lint, "switch", "--quiet", "--detach", "v1.4.0")
	pinned := f.Git(lint, "rev-parse", "--short", "HEAD")

	scratch := f.Clone(f.Remote("fmt"), "work/repos/tools/fmt")
	f.Git(scratch, "switch", "--quiet", "--detach", "HEAD")
	f.Write(scratch, "draft.txt", "wip\n")
	scratchHead := f.Git(scratch, "rev-parse", "--short", "HEAD")

	got := githerd(t, f.Env(), f.Path("."), "status", f.Path("work"))

	assertOutput(t, got, ""+
		"REPO                    BRANCH   UNCOMMITTED  AHEAD  BEHIND  UPSTREAM  NOTE\n"+
		"repos/tools/fmt         "+scratchHead+"  1            0      0       -         detached\n"+
		"repos/tools/lint-rules  "+pinned+"  0            0      0       -         detached\n"+
		"1 of 2 repos need attention\n")
}

func TestStatusFindsTheRootAndNestedReposButSkipsHiddenDirsAndNodeModules(t *testing.T) {
	f := gitfixture.New(t)
	root := f.Clone(f.Remote("work"), "work")
	f.Commit(root, ".gitignore", "repos/\nsandbox/\nnode_modules/\n.cache/\n", "Ignore the repos")
	f.Git(root, "push", "--quiet")

	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Git(api, "worktree", "add", "--quiet", "-b", "hotfix", "../api-hotfix", "origin/main")

	experiments := f.Init("work/sandbox/experiments")
	f.Commit(experiments, ".gitignore", "data/\n", "Ignore the data repo")
	f.Clone(f.Remote("data"), "work/sandbox/experiments/data")

	f.Clone(f.Remote("mirror"), "work/.cache/mirror")
	f.Clone(f.Remote("left-pad"), "work/node_modules/left-pad")

	got := githerd(t, f.Env(), f.Path("."), "status", root)

	assertOutput(t, got, ""+
		"REPO                      BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
		".                         main    0            0      0       origin/main\n"+
		"repos/acme/api            main    0            0      0       origin/main\n"+
		"repos/acme/api-hotfix     hotfix  0            0      0       origin/main\n"+
		"sandbox/experiments       main    0            1      0       -            no upstream\n"+
		"sandbox/experiments/data  main    0            0      0       origin/main\n"+
		"1 of 5 repos need attention\n")
}

func TestStatusScansThreeLevelsBelowTheRootUnlessDepthIsSet(t *testing.T) {
	f := gitfixture.New(t)
	f.Clone(f.Remote("scripts"), "work/scripts")
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Commit(api, ".gitignore", "vendor/\n", "Ignore vendored repos")
	f.Git(api, "push", "--quiet")
	f.Clone(f.Remote("sdk"), "work/repos/acme/api/vendor/sdk")
	root := f.Path("work")

	t.Run("default", func(t *testing.T) {
		assertOutput(t, githerd(t, f.Env(), root, "status"), ""+
			"REPO            BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
			"repos/acme/api  main    0            0      0       origin/main\n"+
			"scripts         main    0            0      0       origin/main\n"+
			"0 of 2 repos need attention\n")
	})
	t.Run("shallower, flag before root", func(t *testing.T) {
		assertOutput(t, githerd(t, f.Env(), f.Path("."), "status", "--depth", "2", root), ""+
			"REPO     BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
			"scripts  main    0            0      0       origin/main\n"+
			"0 of 1 repos need attention\n")
	})
	t.Run("deeper, flag after root", func(t *testing.T) {
		assertOutput(t, githerd(t, f.Env(), f.Path("."), "status", root, "--depth", "5"), ""+
			"REPO                       BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
			"repos/acme/api             main    0            0      0       origin/main\n"+
			"repos/acme/api/vendor/sdk  main    0            0      0       origin/main\n"+
			"scripts                    main    0            0      0       origin/main\n"+
			"0 of 3 repos need attention\n")
	})
}

func TestStatusJSONCarriesTheSameStateAsTheTable(t *testing.T) {
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Git(api, "switch", "--quiet", "--create", "feature/login")
	f.Git(api, "push", "--quiet", "--set-upstream", "origin", "feature/login")
	f.Commit(api, "session.go", "package api\n", "Add session refresh endpoint")
	f.Commit(api, "login.go", "package api\n", "Validate login payload")
	f.Write(api, "login_test.go", "package api\n")
	f.Git(api, "fetch", "--quiet")
	fetchedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(api, ".git", "FETCH_HEAD"), fetchedAt, fetchedAt); err != nil {
		t.Fatal(err)
	}

	lint := f.Clone(f.Remote("lint-rules"), "work/repos/tools/lint-rules")
	f.Git(lint, "switch", "--quiet", "--detach", "HEAD")
	pinned := f.Git(lint, "rev-parse", "--short", "HEAD")

	experiments := f.Init("work/sandbox/experiments")
	f.Commit(experiments, "notes.md", "ideas\n", "Start notes")

	got := githerd(t, f.Env(), f.Path("."), "status", "--json", f.Path("work"))

	assertJSON(t, got, `{"root": `+jsonString(t, f.Path("work"))+`, "repos": [
		{"path": "repos/acme/api", "branch": "feature/login", "detached": null,
		 "upstream": "origin/feature/login", "uncommitted": 1, "ahead": 2, "behind": 0,
		 "needsAttention": true, "stale": null, "fetchedAt": "2026-03-04T05:06:07Z"},
		{"path": "repos/tools/lint-rules", "branch": null, "detached": "`+pinned+`",
		 "upstream": null, "uncommitted": 0, "ahead": 0, "behind": 0,
		 "needsAttention": false, "stale": null, "fetchedAt": null},
		{"path": "sandbox/experiments", "branch": "main", "detached": null,
		 "upstream": null, "uncommitted": 0, "ahead": 1, "behind": 0,
		 "needsAttention": true, "stale": null, "fetchedAt": null}
	]}`)
}

func TestStatusOnARootWithNoReposPrintsOnlyTheSummary(t *testing.T) {
	f := gitfixture.New(t)
	f.Write(f.Path("work"), "notes.txt", "no repos here\n")

	assertOutput(t, githerd(t, f.Env(), f.Path("work"), "status"), "0 of 0 repos need attention\n")
	assertJSON(t, githerd(t, f.Env(), f.Path("work"), "status", "--json"),
		`{"root": `+jsonString(t, f.Path("work"))+`, "repos": []}`)
}

func TestStatusFailsOnAMissingRoot(t *testing.T) {
	f := gitfixture.New(t)

	got := githerd(t, f.Env(), f.Path("."), "status", f.Path("missing"))

	if got.code != 1 || got.stdout != "" || !strings.Contains(got.stderr, f.Path("missing")) {
		t.Errorf("want exit 1, no stdout and the root named on stderr; got %+v", got)
	}
}

func TestStatusReportsAnUnreadableRepoAndStillListsTheOthers(t *testing.T) {
	f := gitfixture.New(t)
	f.Clone(f.Remote("docs"), "work/repos/acme/docs")
	f.Write(f.Path("work/repos/acme/broken"), ".git", "gitdir: ../nowhere\n")

	got := githerd(t, f.Env(), f.Path("."), "status", f.Path("work"))

	if got.code != 1 || !strings.Contains(got.stderr, "repos/acme/broken") {
		t.Errorf("want exit 1 and the broken repo named on stderr; got code %d, stderr:\n%s", got.code, got.stderr)
	}
	want := "" +
		"REPO             BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n" +
		"repos/acme/docs  main    0            0      0       origin/main\n" +
		"0 of 1 repos need attention\n"
	if got.stdout != want {
		t.Errorf("stdout mismatch\n--- got ---\n%s\n--- want ---\n%s", got.stdout, want)
	}
}

// assertJSON fails unless the run exited 0 and printed JSON equal to want.
func assertJSON(t *testing.T, got result, want string) {
	t.Helper()
	if got.code != 0 {
		t.Fatalf("exit code %d, stderr:\n%s", got.code, got.stderr)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got.stdout), &gotValue); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, got.stdout)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("JSON mismatch\n--- got ---\n%s\n--- want ---\n%s", got.stdout, want)
	}
}

func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
