package main_test

import (
	"testing"

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
