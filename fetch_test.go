package main_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
)

func TestStatusFetchTurnsANewUpstreamCommitIntoBehind(t *testing.T) {
	f := gitfixture.New(t)
	webRemote := f.Remote("web")
	f.Clone(webRemote, "work/repos/acme/web")
	f.PushElsewhere(webRemote, "main", 2)
	root := f.Path("work")

	assertOutput(t, githerd(t, f.Env(), root, "status"), ""+
		"REPO            BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
		"repos/acme/web  main    0            0      0       origin/main\n"+
		"0 of 1 repos need attention\n")

	assertOutput(t, githerd(t, f.Env(), root, "status", "--fetch"), ""+
		"REPO            BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
		"repos/acme/web  main    0            0      2       origin/main\n"+
		"1 of 1 repos need attention\n")
}

// The TTL clock is the mtime of FETCH_HEAD: a repo fetched within the TTL is
// skipped, and --force fetches it anyway.
func TestStatusFetchSkipsReposFetchedWithinTheTTL(t *testing.T) {
	f := gitfixture.New(t)
	remotes := map[string]string{}
	for _, name := range []string{"api", "docs", "web"} {
		remotes[name] = f.Remote(name)
		f.Clone(remotes[name], "work/repos/acme/"+name)
	}
	f.FetchedAgo(f.Path("work/repos/acme/api"), 5*time.Minute)
	f.FetchedAgo(f.Path("work/repos/acme/docs"), 20*time.Minute)
	// web was never fetched since it was cloned.
	pushToAll := func() {
		for _, remote := range remotes {
			f.PushElsewhere(remote, "main", 1)
		}
	}
	behind := func(args ...string) map[string]string {
		t.Helper()
		got := githerd(t, f.Env(), f.Path("work"), append([]string{"status", "--json"}, args...)...)
		return field(t, got, "behind")
	}

	pushToAll()
	assertField(t, "behind", behind("--fetch"), map[string]string{
		"repos/acme/api": "0", "repos/acme/docs": "1", "repos/acme/web": "1",
	})
	assertField(t, "behind", behind("--fetch", "--ttl", "2m"), map[string]string{
		"repos/acme/api": "1", "repos/acme/docs": "1", "repos/acme/web": "1",
	})

	pushToAll()
	assertField(t, "behind", behind("--fetch"), map[string]string{
		"repos/acme/api": "1", "repos/acme/docs": "1", "repos/acme/web": "1",
	})
	assertField(t, "behind", behind("--fetch", "--force"), map[string]string{
		"repos/acme/api": "2", "repos/acme/docs": "2", "repos/acme/web": "2",
	})
}

// Nothing is fetched for a detached HEAD, a branch with no upstream, a repo
// with no remote at all, or a branch whose upstream is another local branch.
func TestStatusFetchSkipsReposWithNoUpstreamRemote(t *testing.T) {
	f := gitfixture.New(t)
	cli := f.Clone(f.Remote("cli"), "work/repos/tools/cli")
	f.Git(cli, "switch", "--quiet", "--create", "spike/parser")

	lint := f.Clone(f.Remote("lint-rules"), "work/repos/tools/lint-rules")
	f.Git(lint, "switch", "--quiet", "--detach", "HEAD")

	experiments := f.Init("work/sandbox/experiments")
	f.Commit(experiments, "notes.md", "ideas\n", "Start notes")

	site := f.Clone(f.Remote("site"), "work/repos/acme/site")
	f.Git(site, "switch", "--quiet", "--create", "redesign", "--track", "main")

	got := githerd(t, f.Env(), f.Path("work"), "status", "--json", "--fetch", "--force")

	if got.stderr != "" {
		t.Errorf("want nothing on stderr; got:\n%s", got.stderr)
	}
	nothingFetched := map[string]string{
		"repos/acme/site":        "null",
		"repos/tools/cli":        "null",
		"repos/tools/lint-rules": "null",
		"sandbox/experiments":    "null",
	}
	assertField(t, "fetchedAt", field(t, got, "fetchedAt"), nothingFetched)
	assertField(t, "stale", field(t, got, "stale"), nothingFetched)
}

// A failed fetch empties FETCH_HEAD, so a stale repo is fetched again on the
// next run whatever the TTL, and stays stale until a fetch succeeds.
func TestStatusFetchMarksARepoWithAnUnreachableRemoteStale(t *testing.T) {
	f := gitfixture.New(t)
	webRemote := f.Remote("web")
	f.Clone(webRemote, "work/repos/acme/web")
	f.Clone(f.Remote("docs"), "work/repos/acme/docs")
	f.PushElsewhere(webRemote, "main", 1)
	moved := f.Path("remotes/web-moved.git")
	rename(t, webRemote, moved)
	root := f.Path("work")
	reason := "fatal: '" + webRemote + "' does not appear to be a git repository"

	got := githerd(t, f.Env(), root, "status", "--fetch")

	assertOutput(t, got, ""+
		"REPO             BRANCH  UNCOMMITTED  AHEAD  BEHIND  UPSTREAM     NOTE\n"+
		"repos/acme/docs  main    0            0      0       origin/main\n"+
		"repos/acme/web   main    0            0      0       origin/main  stale\n"+
		"0 of 2 repos need attention, 1 stale\n")
	if want := "githerd: repos/acme/web: fetch origin: " + reason + "\n"; got.stderr != want {
		t.Errorf("stderr mismatch\n--- got ---\n%s\n--- want ---\n%s", got.stderr, want)
	}

	got = githerd(t, f.Env(), root, "status", "--json", "--fetch")

	assertField(t, "stale", field(t, got, "stale"), map[string]string{
		"repos/acme/docs": "null",
		"repos/acme/web":  jsonString(t, reason),
	})
	if at := field(t, got, "fetchedAt")["repos/acme/web"]; at != "null" {
		t.Errorf("want fetchedAt null for the stale repo, got %s", at)
	}

	rename(t, moved, webRemote)
	got = githerd(t, f.Env(), root, "status", "--json", "--fetch")

	assertField(t, "stale", field(t, got, "stale"), map[string]string{
		"repos/acme/docs": "null", "repos/acme/web": "null",
	})
	assertField(t, "behind", field(t, got, "behind"), map[string]string{
		"repos/acme/docs": "0", "repos/acme/web": "1",
	})
}

// Fetches run in parallel, each with its own timeout, so three hanging
// remotes cost one timeout, not three, and the healthy repo is still fetched.
func TestStatusFetchTimesOutHangingRemotesAndMarksThemStale(t *testing.T) {
	f := gitfixture.New(t)
	f.FakeSSH()
	for _, name := range []string{"api", "billing", "web"} {
		repo := f.Clone(f.Remote(name), "work/repos/acme/"+name)
		f.Git(repo, "remote", "set-url", "origin", "ssh://git@"+gitfixture.HangingHost+"/acme/"+name+".git")
	}
	docsRemote := f.Remote("docs")
	f.Clone(docsRemote, "work/repos/acme/docs")
	f.PushElsewhere(docsRemote, "main", 1)

	start := time.Now()
	got := githerd(t, f.Env(), f.Path("work"), "status", "--json", "--fetch", "--fetch-timeout", "1s")
	elapsed := time.Since(start)

	timedOut := jsonString(t, "timed out after 1s")
	assertField(t, "stale", field(t, got, "stale"), map[string]string{
		"repos/acme/api": timedOut, "repos/acme/billing": timedOut, "repos/acme/web": timedOut,
		"repos/acme/docs": "null",
	})
	assertField(t, "behind", field(t, got, "behind"), map[string]string{
		"repos/acme/api": "0", "repos/acme/billing": "0", "repos/acme/web": "0",
		"repos/acme/docs": "1",
	})
	if elapsed > 2500*time.Millisecond {
		t.Errorf("status took %v: want about one 1s timeout for all three hanging fetches", elapsed)
	}

	// A killed fetch takes its ssh down with it.
	time.Sleep(time.Until(start.Add(gitfixture.HangTime + 500*time.Millisecond)))
	for _, call := range f.SSHCalls() {
		if strings.HasPrefix(call, "outlived the fetch") {
			t.Errorf("ssh kept running after its fetch timed out: %s", call)
		}
	}
}

// Fetch turns terminal prompts off, even when the user turned them on, and
// runs ssh in batch mode unless the user set their own SSH command.
func TestStatusFetchNeverPrompts(t *testing.T) {
	const ownCommand = "ssh -o ConnectTimeout=7"
	for _, tc := range []struct {
		name      string
		env       []string
		sshConfig string // core.sshCommand of the repo
		wantSSH   string // how the fake ssh was called, up to the host
	}{
		{name: "default", wantSSH: "-o BatchMode=yes -o SendEnv=GIT_PROTOCOL"},
		{name: "own GIT_SSH_COMMAND", env: []string{"GIT_SSH_COMMAND=" + ownCommand},
			wantSSH: "-o ConnectTimeout=7 -o SendEnv=GIT_PROTOCOL"},
		{name: "own core.sshCommand", sshConfig: ownCommand,
			wantSSH: "-o ConnectTimeout=7 -o SendEnv=GIT_PROTOCOL"},
		{name: "own GIT_SSH", env: []string{"GIT_SSH=ssh"},
			wantSSH: "-o SendEnv=GIT_PROTOCOL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := gitfixture.New(t)
			f.FakeSSH()
			api := f.Clone(f.Remote("api"), "work/repos/acme/api")
			f.Git(api, "remote", "set-url", "origin", "ssh://git@git.example.com/acme/api.git")
			if tc.sshConfig != "" {
				f.Git(api, "config", "core.sshCommand", tc.sshConfig)
			}
			env := append(append(f.Env(), "GIT_TERMINAL_PROMPT=1"), tc.env...)

			got := githerd(t, env, f.Path("work"), "status", "--fetch")

			if got.code != 0 || !strings.HasSuffix(got.stdout, "0 of 1 repos need attention, 1 stale\n") {
				t.Errorf("want exit 0 and the repo stale; got %+v", got)
			}
			want := []string{"GIT_TERMINAL_PROMPT=0 " + tc.wantSSH + " git@git.example.com git-upload-pack '/acme/api.git'"}
			if calls := f.SSHCalls(); !reflect.DeepEqual(calls, want) {
				t.Errorf("ssh calls:\n got %q\nwant %q", calls, want)
			}
		})
	}
}

func TestStatusRejectsFetchOptionsWithoutFetch(t *testing.T) {
	f := gitfixture.New(t)
	f.Clone(f.Remote("web"), "work/repos/acme/web")

	for _, args := range [][]string{{"--ttl", "5m"}, {"--force"}, {"--fetch-timeout", "5s"}} {
		got := githerd(t, f.Env(), f.Path("work"), append([]string{"status"}, args...)...)
		if want := "githerd status: " + args[0] + " needs --fetch\n"; got.code != 2 || got.stdout != "" || got.stderr != want {
			t.Errorf("status %v: want exit 2 and stderr %q; got %+v", args, want, got)
		}
	}
}

// field decodes the JSON of a successful status run and returns, for each
// repo path, the raw JSON of the named field, such as "2" or "null".
func field(t *testing.T, got result, name string) map[string]string {
	t.Helper()
	if got.code != 0 {
		t.Fatalf("exit code %d, stderr:\n%s", got.code, got.stderr)
	}
	var report struct {
		Repos []map[string]json.RawMessage `json:"repos"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, got.stdout)
	}
	values := map[string]string{}
	for _, r := range report.Repos {
		var path string
		if err := json.Unmarshal(r["path"], &path); err != nil {
			t.Fatalf("repo without a path: %v\n%s", err, got.stdout)
		}
		values[path] = string(r[name])
	}
	return values
}

func assertField(t *testing.T, name string, got, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s by repo:\n got %v\nwant %v", name, got, want)
	}
}

func rename(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}
