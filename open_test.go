package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
)

// binDir returns a dir holding a shell script for each name, with the body
// that scripts holds for it.
func binDir(t *testing.T, scripts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// onlyGit returns a dir in which git runs the git the tests run, so that a
// PATH of that dir alone finds git and nothing else.
func onlyGit(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	return binDir(t, map[string]string{"git": `exec '` + git + `' "$@"`})
}

// withPath returns the PATH entry of an environment that finds only the
// programs in dirs, in that order.
func withPath(dirs ...string) string {
	return "PATH=" + strings.Join(dirs, string(os.PathListSeparator))
}

// resolved returns path with symlinks resolved, as the rail names repos to
// Herdr.
func resolved(t *testing.T, path string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// expectCalls waits for the herdr calls want, made since the last calls, and
// fails unless they are exactly want.
func (h *fakeHerdr) expectCalls(when string, want ...[]string) {
	h.t.Helper()
	h.waitForCalls(len(want))
	time.Sleep(50 * time.Millisecond) // for any call that should not come
	if got := h.calls(); !reflect.DeepEqual(got, want) {
		h.t.Errorf("%s: herdr calls\n got %q\nwant %q", when, got, want)
	}
}

// From the rail, o focuses the workspace mapped to the selected repo, or
// creates one at the repo and focuses that one from then on. s opens a
// shell in the repo below the rail, and L opens lazygit there, as it is on
// PATH. Each line names the workspaces mapped to its repo.
func TestRailOpensTheSelectedRepoInHerdr(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	work := f.EveryState()
	api := filepath.Join(work, gitfixture.UncommittedRepo)
	billing := resolved(t, filepath.Join(work, gitfixture.AheadRepo))
	h := newFakeHerdr(t, f)
	h.roots(work + "\n")
	h.snapshot(at("w1", work), at("w2", api))
	term := newPTY(t, 60, 20)

	exited := h.startRail(term, api, withPath(binDir(t, map[string]string{"lazygit": "exit 0"}), onlyGit(t)))
	pos := term.waitFor(0, "⧉ws-w2")
	h.calls()

	term.typed("o")
	h.expectCalls("o on api", []string{"workspace", "focus", "w2"})

	term.typed("j")
	pos = term.waitFor(pos, "\x1b[7mrepos/acme/billing")
	term.typed("o")
	h.expectCalls("o on billing", []string{"workspace", "create", "--cwd", billing, "--label", "billing", "--focus"})
	term.waitFor(pos, "⧉billing")

	term.typed("s")
	split := []string{"pane", "split", "--pane", "w1:p1", "--direction", "right", "--cwd", billing, "--focus"}
	h.expectCalls("s on billing", split)
	term.typed("L")
	h.expectCalls("L on billing", split, []string{"pane", "run", fakeSplitPane("w1:p1"), "lazygit"})

	// o waits while Herdr creates the workspace, so press it until it acts.
	for deadline := time.Now().Add(10 * time.Second); len(h.peekCalls()) == 0; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("o on billing again made no herdr call")
		}
		term.typed("o")
	}
	focus := []string{"workspace", "focus", fakeCreatedWorkspace}
	if calls := h.calls(); !slices.ContainsFunc(calls, func(c []string) bool { return slices.Equal(c, focus) }) ||
		slices.ContainsFunc(calls, func(c []string) bool { return !slices.Equal(c, focus) }) {
		t.Errorf("o on billing again: herdr calls %q, want only %q", calls, focus)
	}

	term.typed("q")
	if err := <-exited; err != nil {
		t.Fatalf("the rail exited with %v", err)
	}
}

// s opens the shell in a split to the right of the pane the rail was opened
// from, so it gets half of that pane rather than of the narrow rail. When
// that pane is gone, s splits the rail downwards instead.
func TestRailOpensTheShellBesideThePaneItWasOpenedFrom(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	work := f.EveryState()
	api := filepath.Join(work, gitfixture.UncommittedRepo)
	h := newFakeHerdr(t, f)
	h.roots(work + "\n")
	h.snapshot(at("w1", work), at("w2", api))
	term := newPTY(t, 60, 20)

	exited := h.startRail(term, api)
	term.waitFor(0, "⧉ws-w2")
	h.calls()

	term.typed("s")
	beside := []string{"pane", "split", "--pane", "w1:p1", "--direction", "right", "--cwd", resolved(t, api), "--focus"}
	h.expectCalls("s", beside)
	writeFile(t, filepath.Join(h.dir, fakeClosedFile), "w1:p1\n")
	term.typed("s")
	h.expectCalls("s with that pane closed", beside,
		[]string{"pane", "split", "--pane", "w1:p9", "--direction", "down", "--cwd", resolved(t, api), "--focus"})

	term.typed("q")
	if err := <-exited; err != nil {
		t.Fatalf("the rail exited with %v", err)
	}
}

// Without lazygit on PATH, L opens nothing and the rail says so. When Herdr
// refuses a request, the rail shows why.
func TestRailSaysWhyItCannotOpenTheRepo(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	work := f.EveryState()
	api := filepath.Join(work, gitfixture.UncommittedRepo)
	h := newFakeHerdr(t, f)
	h.roots(work + "\n")
	h.snapshot(at("w1", work), at("w2", api))
	term := newPTY(t, 60, 20)

	exited := h.startRail(term, api, withPath(onlyGit(t)))
	pos := term.waitFor(0, "⧉ws-w2")
	h.calls()

	term.typed("L")
	pos = term.waitFor(pos, "lazygit is not on PATH")
	h.expectCalls("L without lazygit")

	writeFile(t, filepath.Join(h.dir, fakeClosedFile), "w2\n")
	term.typed("o")
	h.expectCalls("o on api", []string{"workspace", "focus", "w2"})
	term.waitFor(pos, "⚠ herdr workspace focus: workspace w2 not found")

	term.typed("q")
	if err := <-exited; err != nil {
		t.Fatalf("the rail exited with %v", err)
	}
}
