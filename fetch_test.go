package main_test

import (
	"testing"

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
