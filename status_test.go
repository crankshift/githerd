package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
)

func TestStatusListsACleanRepo(t *testing.T) {
	f := gitfixture.New(t)
	f.Clone(f.Remote("docs"), "work/repos/acme/docs")

	got := githerd(t, f.Env(), f.Path("."), "status", f.Path("work"))

	assertOutput(t, got, ""+
		"REPO             BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
		"repos/acme/docs  main    0            0      0       origin/main\n"+
		"0 of 1 repos need attention\n")
}

func TestStatusCountsModifiedAndUntrackedFilesAsUncommitted(t *testing.T) {
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Write(api, "README.md", "# api, edited\n")
	f.Write(api, "src/auth.go", "package auth\n")
	f.Write(api, "src/auth_test.go", "package auth\n")

	got := githerd(t, f.Env(), f.Path("."), "status", f.Path("work"))

	assertOutput(t, got, ""+
		"REPO            BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
		"repos/acme/api  main    3            0      0       origin/main\n"+
		"1 of 1 repos need attention\n")
}

func TestStatusCountsCommitsAheadAndBehindTheUpstream(t *testing.T) {
	f := gitfixture.New(t)
	billingRemote, mobileRemote, webRemote := f.Remote("billing"), f.Remote("mobile"), f.Remote("web")

	billing := f.Clone(billingRemote, "work/repos/acme/billing")
	f.Commit(billing, "invoice.go", "package billing\n", "Add invoices")
	f.Commit(billing, "refund.go", "package billing\n", "Add refunds")

	mobile := f.Clone(mobileRemote, "work/repos/acme/mobile")
	f.Commit(mobile, "crash.go", "package mobile\n", "Fix crash on resume")
	f.PushElsewhere(mobileRemote, "main", 2)
	f.Git(mobile, "fetch", "--quiet")

	web := f.Clone(webRemote, "work/repos/acme/web")
	f.PushElsewhere(webRemote, "main", 4)
	f.Git(web, "fetch", "--quiet")

	got := githerd(t, f.Env(), f.Path("."), "status", f.Path("work"))

	assertOutput(t, got, ""+
		"REPO                BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
		"repos/acme/billing  main    0            2      0       origin/main\n"+
		"repos/acme/mobile   main    0            1      2       origin/main\n"+
		"repos/acme/web      main    0            0      4       origin/main\n"+
		"3 of 3 repos need attention\n")
}

func TestStatusCountsCommitsOnNoRemoteBranchWhenThereIsNoUpstream(t *testing.T) {
	f := gitfixture.New(t)
	cli := f.Clone(f.Remote("cli"), "work/repos/tools/cli")
	f.Git(cli, "switch", "--quiet", "--create", "spike/parser")
	f.Commit(cli, "lexer.go", "package cli\n", "WIP tokenizer")
	f.Commit(cli, "flags.go", "package cli\n", "Parse flags")
	f.Commit(cli, "ast.go", "package cli\n", "Spike: AST printer")

	experiments := f.Init("work/sandbox/experiments")
	f.Commit(experiments, "notes.md", "ideas\n", "Start notes")
	f.Commit(experiments, "plot.py", "print()\n", "Add plot")

	got := githerd(t, f.Env(), f.Path("."), "status", f.Path("work"))

	assertOutput(t, got, ""+
		"REPO                 BRANCH        UNCOMMITTED  AHEAD  BEHIND  UPSTREAM  NOTE\n"+
		"repos/tools/cli      spike/parser  0            3      0       -         no upstream\n"+
		"sandbox/experiments  main          0            2      0       -         no upstream\n"+
		"2 of 2 repos need attention\n")
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
	f.Commit(root, ".gitignore", "repos/\nsandbox/\nnode_modules/\n.cache/\n", "Ignore the clones")
	f.Git(root, "push", "--quiet")

	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Git(api, "worktree", "add", "--quiet", "-b", "hotfix", "../api-hotfix", "origin/main")

	experiments := f.Init("work/sandbox/experiments")
	f.Commit(experiments, ".gitignore", "data/\n", "Ignore the data clone")
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
	f.Commit(api, ".gitignore", "vendor/\n", "Ignore vendored clones")
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
