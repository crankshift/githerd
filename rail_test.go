package main_test

import (
	"reflect"
	"testing"

	"github.com/crankshift/githerd/internal/gitfixture"
)

// railAction runs githerd herdr rail as Herdr runs the rail action.
func (h *fakeHerdr) railAction() result {
	h.t.Helper()
	return githerd(h.t, h.env(), h.f.Path("."), "herdr", "rail")
}

// assertCalls fails unless the run exited 0 with no output, and herdr was
// called with exactly want, in order.
func (h *fakeHerdr) assertCalls(got result, want [][]string) {
	h.t.Helper()
	if got.code != 0 || got.stdout != "" || got.stderr != "" {
		h.t.Fatalf("want exit 0 and no output; got %+v", got)
	}
	if calls := h.calls(); !reflect.DeepEqual(calls, want) {
		h.t.Errorf("herdr calls:\n got %q\nwant %q", calls, want)
	}
}

// With no rail in the current workspace, the action opens one as a split
// to the right of the focused pane, marks it with the rail's pane token, and
// narrows it to about a quarter of the width: the split starts at half, and
// the amount is a ratio delta.
func TestRailActionOpensTheRailBesideTheFocusedPane(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	h := newFakeHerdr(t, f)
	h.snapshot(
		at("w1", f.Path("work")),
		workspace{id: "w2", tabs: [][]pane{{{cwd: f.Path("work")}, {cwd: f.Path("work/api"), focused: true}}}},
	)

	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "open", "--plugin", "githerd", "--entrypoint", "rail", "--placement", "split",
			"--target-pane", "w2:p2", "--direction", "right", "--focus"},
		{"pane", "report-metadata", "w2:p9", "--source", "githerd", "--token", "githerd=rail"},
		{"pane", "resize", "--pane", "w2:p9", "--direction", "right", "--amount", "0.25"},
	})
}

// A pane can keep the rail's token after Herdr has forgotten it is a plugin
// pane, as after a live handoff, so focusing it fails. The action then tries
// the next marked pane, and opens a fresh rail when none can be focused, so
// pressing the key again never piles up rails.
func TestRailActionOpensAFreshRailWhenTheMarkedPaneIsNoLongerAPluginPane(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	rail := map[string]string{"githerd": "rail"}
	h := newFakeHerdr(t, f)
	h.forget("w1:p2")
	h.snapshot(workspace{id: "w1", tabs: [][]pane{{{cwd: f.Path("work"), focused: true}, {cwd: f.Path("work"), tokens: rail}}}})

	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "focus", "w1:p2"},
		{"plugin", "pane", "open", "--plugin", "githerd", "--entrypoint", "rail", "--placement", "split",
			"--target-pane", "w1:p1", "--direction", "right", "--focus"},
		{"pane", "report-metadata", "w1:p9", "--source", "githerd", "--token", "githerd=rail"},
		{"pane", "resize", "--pane", "w1:p9", "--direction", "right", "--amount", "0.25"},
	})

	h.snapshot(workspace{id: "w1", tabs: [][]pane{{{cwd: f.Path("work"), focused: true}, {cwd: f.Path("work"), tokens: rail}, {cwd: f.Path("work"), tokens: rail}}}})
	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "focus", "w1:p2"},
		{"plugin", "pane", "focus", "w1:p3"},
	})
}

// A rail already open anywhere in the current workspace, in any tab, is
// focused instead of opening a second one. A rail in another workspace
// doesn't count.
func TestRailActionFocusesTheRailOfTheCurrentWorkspace(t *testing.T) {
	t.Parallel()
	f := gitfixture.New(t)
	rail := map[string]string{"githerd": "rail"}
	h := newFakeHerdr(t, f)
	h.snapshot(
		workspace{id: "w1", tabs: [][]pane{{{cwd: f.Path("work")}, {cwd: f.Path("work"), tokens: rail}}}},
		workspace{id: "w2", tabs: [][]pane{
			{{cwd: f.Path("work/api"), focused: true}},
			{{cwd: f.Path("work/api")}, {cwd: f.Path("work/api"), tokens: map[string]string{"other": "rail"}}, {cwd: f.Path("work"), tokens: rail}},
		}},
	)

	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "focus", "w2:p4"},
	})

	h.snapshot(
		workspace{id: "w1", tabs: [][]pane{{{cwd: f.Path("work")}, {cwd: f.Path("work"), tokens: rail}}}},
		workspace{id: "w2", tabs: [][]pane{{{cwd: f.Path("work/api"), focused: true}}}},
	)
	h.assertCalls(h.railAction(), [][]string{
		{"api", "snapshot"},
		{"plugin", "pane", "open", "--plugin", "githerd", "--entrypoint", "rail", "--placement", "split",
			"--target-pane", "w2:p1", "--direction", "right", "--focus"},
		{"pane", "report-metadata", "w2:p9", "--source", "githerd", "--token", "githerd=rail"},
		{"pane", "resize", "--pane", "w2:p9", "--direction", "right", "--amount", "0.25"},
	})
}
