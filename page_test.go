package main_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
)

// waitForCall waits until herdr was called with argv, and fails the test
// if that takes over 10 seconds.
func (h *fakeHerdr) waitForCall(argv ...string) {
	h.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !slices.ContainsFunc(h.peekCalls(), func(c []string) bool { return slices.Equal(c, argv) }); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			h.t.Fatalf("waited 10s for herdr %q; got %q", argv, h.peekCalls())
		}
	}
}

// Enter on a repo shows its repo page, read from the repo, with the diff
// of its first change, and has Herdr zoom the rail's own pane. z toggles
// the zoom, and Esc goes back to the rail and unzooms.
func TestRepoPageOpensZoomedFromTheRailAndShowsTheReposChanges(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	work := f.EveryState()
	api := filepath.Join(work, gitfixture.UncommittedRepo)
	h := newFakeHerdr(t, f)
	h.roots(work + "\n")
	h.snapshot(at("w1", api))
	term := newPTY(t, 120, 30)

	exited := h.startRail(term, api)
	pos := term.waitFor(0, "\x1b[7mrepos/acme/api")
	pos = h.waitForSync(term, pos)

	term.typed("\r")
	pos = term.waitFor(pos, "Changes 3")
	pos = term.waitFor(pos, "+# api, edited")
	h.waitForCall("pane", "zoom", "--pane", "w1:p9", "--on")
	term.typed("z")
	h.waitForCall("pane", "zoom", "--pane", "w1:p9", "--toggle")
	term.typed("\x1b")
	h.waitForCall("pane", "zoom", "--pane", "w1:p9", "--off")
	term.waitFor(pos, "\x1b[7mrepos/acme/api")

	term.typed("q")
	if err := <-exited; err != nil {
		t.Fatalf("the rail exited with %v", err)
	}
	var zooms [][]string
	for _, c := range h.peekCalls() {
		if len(c) > 1 && c[0] == "pane" && c[1] == "zoom" {
			zooms = append(zooms, c[len(c)-1:])
		}
	}
	if want := [][]string{{"--on"}, {"--toggle"}, {"--off"}}; !slices.EqualFunc(zooms, want, slices.Equal) {
		t.Errorf("zooms in order %q, want %q", zooms, want)
	}
}
