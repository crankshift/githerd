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
