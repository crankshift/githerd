package gitfixture

// The repos of the root that EveryState builds, relative to that root.
const (
	CleanRepo       = "repos/acme/docs"          // on main, level with origin/main
	UncommittedRepo = "repos/acme/api"           // one modified and two untracked files
	AheadRepo       = "repos/acme/billing"       // two commits ahead of origin/main
	BehindRepo      = "repos/acme/web"           // four commits behind origin/main
	DivergedRepo    = "repos/acme/mobile"        // one commit ahead of origin/main, two behind
	NoUpstreamRepo  = "repos/tools/cli"          // branch spike/parser with three commits on no remote
	DetachedRepo    = "repos/tools/lint-rules"   // detached at tag v1.4.0, clean
	NoRemoteRepo    = "sandbox/experiments"      // two commits and no remote at all
	NestedRepo      = "sandbox/experiments/data" // clean repo nested in NoRemoteRepo
)

// EveryState builds a root at work/ in the fixture dir and returns its path.
// The root is itself a clean repo that ignores the repos under it, and it
// holds one repo in each state named by the constants above.
func (f *Fixture) EveryState() string {
	f.t.Helper()
	root := f.Clone(f.Remote("work"), "work")
	f.Commit(root, ".gitignore", "repos/\nsandbox/\n", "Ignore the repos")
	f.Git(root, "push", "--quiet")
	at := func(rel string) string { return "work/" + rel }

	f.Clone(f.Remote("docs"), at(CleanRepo))

	api := f.Clone(f.Remote("api"), at(UncommittedRepo))
	f.Write(api, "README.md", "# api, edited\n")
	f.Write(api, "src/auth.go", "package auth\n")
	f.Write(api, "src/auth_test.go", "package auth\n")

	billing := f.Clone(f.Remote("billing"), at(AheadRepo))
	f.Commit(billing, "invoice.go", "package billing\n", "Add invoices")
	f.Commit(billing, "refund.go", "package billing\n", "Add refunds")

	webRemote := f.Remote("web")
	web := f.Clone(webRemote, at(BehindRepo))
	f.PushElsewhere(webRemote, "main", 4)
	f.Git(web, "fetch", "--quiet")

	mobileRemote := f.Remote("mobile")
	mobile := f.Clone(mobileRemote, at(DivergedRepo))
	f.Commit(mobile, "crash.go", "package mobile\n", "Fix crash on resume")
	f.PushElsewhere(mobileRemote, "main", 2)
	f.Git(mobile, "fetch", "--quiet")

	cli := f.Clone(f.Remote("cli"), at(NoUpstreamRepo))
	f.Git(cli, "switch", "--quiet", "--create", "spike/parser")
	f.Commit(cli, "lexer.go", "package cli\n", "WIP tokenizer")
	f.Commit(cli, "flags.go", "package cli\n", "Parse flags")
	f.Commit(cli, "ast.go", "package cli\n", "Spike: AST printer")

	lint := f.Clone(f.Remote("lint-rules"), at(DetachedRepo))
	f.Git(lint, "tag", "v1.4.0")
	f.Commit(lint, "rules.txt", "no-tabs\n", "Add rules")
	f.Git(lint, "switch", "--quiet", "--detach", "v1.4.0")

	experiments := f.Init(at(NoRemoteRepo))
	f.Commit(experiments, ".gitignore", "data/\n", "Ignore the data repo")
	f.Commit(experiments, "notes.md", "ideas\n", "Start notes")
	f.Clone(f.Remote("data"), at(NestedRepo))

	return root
}
