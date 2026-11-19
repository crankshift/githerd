package main_test

import (
	"encoding/json"
	"reflect"
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
	root := f.Path("work")
	status := func(args ...string) map[string]int {
		t.Helper()
		return behindByPath(t, githerd(t, f.Env(), root, append([]string{"status", "--json"}, args...)...))
	}

	pushToAll()
	assertBehind(t, status("--fetch"), map[string]int{
		"repos/acme/api": 0, "repos/acme/docs": 1, "repos/acme/web": 1,
	})
	assertBehind(t, status("--fetch", "--ttl", "2m"), map[string]int{
		"repos/acme/api": 1, "repos/acme/docs": 1, "repos/acme/web": 1,
	})

	pushToAll()
	assertBehind(t, status("--fetch"), map[string]int{
		"repos/acme/api": 1, "repos/acme/docs": 1, "repos/acme/web": 1,
	})
	assertBehind(t, status("--fetch", "--force"), map[string]int{
		"repos/acme/api": 2, "repos/acme/docs": 2, "repos/acme/web": 2,
	})
}

func TestStatusRejectsFetchOptionsWithoutFetch(t *testing.T) {
	f := gitfixture.New(t)
	f.Clone(f.Remote("web"), "work/repos/acme/web")

	for _, args := range [][]string{{"--ttl", "5m"}, {"--force"}} {
		got := githerd(t, f.Env(), f.Path("work"), append([]string{"status"}, args...)...)
		if want := "githerd status: " + args[0] + " needs --fetch\n"; got.code != 2 || got.stdout != "" || got.stderr != want {
			t.Errorf("status %v: want exit 2 and stderr %q; got %+v", args, want, got)
		}
	}
}

// behindByPath decodes the JSON of a status run into each repo's behind count.
func behindByPath(t *testing.T, got result) map[string]int {
	t.Helper()
	if got.code != 0 {
		t.Fatalf("exit code %d, stderr:\n%s", got.code, got.stderr)
	}
	var report struct {
		Repos []struct {
			Path   string `json:"path"`
			Behind int    `json:"behind"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, got.stdout)
	}
	behind := map[string]int{}
	for _, r := range report.Repos {
		behind[r.Path] = r.Behind
	}
	return behind
}

func assertBehind(t *testing.T, got, want map[string]int) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("behind counts: got %v, want %v", got, want)
	}
}
