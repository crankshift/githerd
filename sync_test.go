package main_test

import (
	"testing"

	"github.com/crankshift/githerd/internal/gitfixture"
)

// A mapped workspace gets its repo's uncommitted count; the root's summary
// and the stale mark don't apply to it, so they are cleared.
func TestSyncReportsUncommittedChangesToAMappedWorkspace(t *testing.T) {
	f := gitfixture.New(t)
	api := f.Clone(f.Remote("api"), "work/repos/acme/api")
	f.Write(api, "README.md", "# api, edited\n")
	f.Write(api, "src/auth.go", "package auth\n")
	h := newFakeHerdr(t, f)
	h.roots(f.Path("work") + "\n")
	h.snapshot(at("w1", api))

	h.assertSynced(h.sync(), map[string]tokens{
		"w1": {"changes": "●2", "repos": cleared, "stale": cleared},
	})
}
