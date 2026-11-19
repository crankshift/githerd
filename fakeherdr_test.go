package main_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/crankshift/githerd/internal/gitfixture"
)

// fakeHerdrDirEnv names the dir of a fake herdr. The test binary, run with
// it set, acts as herdr instead of running tests: githerd reaches it through
// HERDR_BIN_PATH and never talks to a real Herdr.
const fakeHerdrDirEnv = "GITHERD_TEST_FAKE_HERDR"

// Files in a fake herdr's dir.
const (
	fakeSnapshotFile = "snapshot.json" // the snapshot `api snapshot` serves; without it, herdr is not running
	fakeCallsFile    = "calls.jsonl"   // one JSON array of argv per call
	fakeHoldFile     = "hold"          // while it exists, `api snapshot` waits
	fakeClosedFile   = "closed"        // ids of workspaces closed since the snapshot, one per line
	fakeShellsFile   = "shells"        // ids of panes that came back as plain shells after a restart, one per line
)

// fakeOpenedPane is the id of the pane `plugin pane open` opens in workspace
// ws.
func fakeOpenedPane(ws string) string { return ws + ":p9" }

// runFakeHerdr is the fake herdr. It records its argv, serves the canned
// snapshot, and accepts every workspace report-metadata call silently, as
// herdr does on success, unless the workspace was closed. It opens, focuses
// and resizes plugin panes, answering as Herdr 0.9.5 does.
func runFakeHerdr(dir string, args []string) int {
	line, _ := json.Marshal(args)
	calls, err := os.OpenFile(filepath.Join(dir, fakeCallsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake herdr:", err)
		return 1
	}
	_, err = calls.Write(append(line, '\n'))
	calls.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake herdr:", err)
		return 1
	}

	switch {
	case len(args) == 2 && args[0] == "api" && args[1] == "snapshot":
		for deadline := time.Now().Add(10 * time.Second); exists(filepath.Join(dir, fakeHoldFile)); {
			if time.Now().After(deadline) {
				fmt.Fprintln(os.Stderr, "fake herdr: held for too long")
				return 1
			}
			time.Sleep(5 * time.Millisecond)
		}
		snap, err := os.ReadFile(filepath.Join(dir, fakeSnapshotFile))
		if err != nil {
			fmt.Fprintln(os.Stderr, `{"id":"cli:api:snapshot","error":{"code":"server_not_running","message":"herdr server is not running"}}`)
			return 1
		}
		fmt.Printf(`{"id":"cli:api:snapshot","result":{"type":"session_snapshot","snapshot":%s}}`+"\n", bytes.TrimSpace(snap))
		return 0
	case len(args) > 2 && args[0] == "workspace" && args[1] == "report-metadata":
		closed, _ := os.ReadFile(filepath.Join(dir, fakeClosedFile))
		for _, id := range strings.Fields(string(closed)) {
			if id == args[2] {
				fmt.Fprintf(os.Stderr, `{"id":"cli:request","error":{"code":"workspace_not_found","message":"workspace %s not found"}}`+"\n", id)
				return 1
			}
		}
		return 0
	case len(args) > 2 && args[0] == "pane" && args[1] == "report-metadata":
		return 0
	case len(args) > 3 && args[0] == "plugin" && args[1] == "pane" && args[2] == "open":
		ws := "w1"
		for i, arg := range args {
			if arg == "--target-pane" && i+1 < len(args) {
				ws, _, _ = strings.Cut(args[i+1], ":")
			}
		}
		fmt.Printf(`{"id":"cli:plugin","result":{"type":"plugin_pane_opened","plugin_pane":{"plugin_id":"githerd","entrypoint":"rail",`+
			`"pane":{"pane_id":%q,"workspace_id":%q,"tab_id":"%s:t1","focused":true,"label":"githerd","agent_status":"unknown","revision":1}}}}`+"\n",
			fakeOpenedPane(ws), ws, ws)
		return 0
	case len(args) == 4 && args[0] == "plugin" && args[1] == "pane" && args[2] == "focus":
		shells, _ := os.ReadFile(filepath.Join(dir, fakeShellsFile))
		for _, id := range strings.Fields(string(shells)) {
			if id == args[3] {
				fmt.Fprintln(os.Stderr, `{"id":"cli:plugin","error":{"code":"plugin_pane_not_found","message":"plugin pane not found"}}`)
				return 1
			}
		}
		ws, _, _ := strings.Cut(args[3], ":")
		fmt.Printf(`{"id":"cli:plugin","result":{"type":"plugin_pane_focused","plugin_pane":{"plugin_id":"githerd","entrypoint":"rail",`+
			`"pane":{"pane_id":%q,"workspace_id":%q,"focused":true,"agent_status":"unknown","revision":1}}}}`+"\n", args[3], ws)
		return 0
	case len(args) > 2 && args[0] == "pane" && args[1] == "resize":
		fmt.Println(`{"id":"cli:pane:resize","result":{"type":"pane_resize","resize":{"changed":true}}}`)
		return 0
	}
	fmt.Fprintf(os.Stderr, "fake herdr: unexpected command %q\n", args)
	return 2
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fakeHerdr is a fake Herdr session for githerd herdr sync: the fake herdr
// binary, the plugin's config and state dirs, and the session's snapshot.
type fakeHerdr struct {
	t   *testing.T
	f   *gitfixture.Fixture
	dir string
}

func newFakeHerdr(t *testing.T, f *gitfixture.Fixture) *fakeHerdr {
	t.Helper()
	h := &fakeHerdr{t: t, f: f, dir: f.Path("herdr")}
	for _, dir := range []string{h.dir, h.configDir(), h.stateDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func (h *fakeHerdr) configDir() string { return h.f.Path("plugin-config") }
func (h *fakeHerdr) stateDir() string  { return h.f.Path("plugin-state") }

// env returns the fixture's environment plus what Herdr gives a plugin hook.
func (h *fakeHerdr) env() []string {
	h.t.Helper()
	self, err := os.Executable()
	if err != nil {
		h.t.Fatal(err)
	}
	return append(h.f.Env(),
		fakeHerdrDirEnv+"="+h.dir,
		"HERDR_BIN_PATH="+self,
		"HERDR_ENV=1",
		"HERDR_PLUGIN_ID=githerd",
		"HERDR_PLUGIN_CONFIG_DIR="+h.configDir(),
		"HERDR_PLUGIN_STATE_DIR="+h.stateDir(),
	)
}

// roots writes the plugin's roots file.
func (h *fakeHerdr) roots(content string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.configDir(), "roots"), []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// sync runs githerd herdr sync as a Herdr hook would, with extra env added.
func (h *fakeHerdr) sync(extra ...string) result {
	h.t.Helper()
	return githerd(h.t, append(h.env(), extra...), h.f.Path("."), "herdr", "sync")
}

// startSync starts githerd herdr sync as a Herdr hook would, without
// waiting for it. wait waits for it to exit and returns what it produced.
func (h *fakeHerdr) startSync(extra ...string) (wait func() result) {
	h.t.Helper()
	cmd := exec.Command(githerdBin, "herdr", "sync")
	cmd.Dir = h.f.Path(".")
	cmd.Env = append(append(h.env(), extra...), "PWD="+cmd.Dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		h.t.Fatal(err)
	}
	return func() result {
		h.t.Helper()
		err := cmd.Wait()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			h.t.Fatalf("running githerd: %v", err)
		}
		return result{stdout: stdout.String(), stderr: stderr.String(), code: cmd.ProcessState.ExitCode()}
	}
}

// hold makes `api snapshot` wait until release is called, so a sync can be
// caught while it runs.
func (h *fakeHerdr) hold() (release func()) {
	h.t.Helper()
	path := filepath.Join(h.dir, fakeHoldFile)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		h.t.Fatal(err)
	}
	return func() {
		h.t.Helper()
		if err := os.Remove(path); err != nil {
			h.t.Fatal(err)
		}
	}
}

// forget makes Herdr forget that pane is a plugin pane, as a live handoff
// does, so `plugin pane focus` on it fails.
func (h *fakeHerdr) forget(pane string) {
	h.t.Helper()
	f, err := os.OpenFile(filepath.Join(h.dir, fakeShellsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, pane); err != nil {
		h.t.Fatal(err)
	}
}

// waitForCalls waits until n herdr calls were made since the last calls,
// and fails the test if that takes over 10 seconds.
func (h *fakeHerdr) waitForCalls(n int) {
	h.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); len(h.peekCalls()) < n; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			h.t.Fatalf("waited 10s for %d herdr calls; got %q", n, h.peekCalls())
		}
	}
}

// calls returns the argv of every herdr call made since the last calls, and
// forgets them.
func (h *fakeHerdr) calls() [][]string {
	h.t.Helper()
	calls := h.peekCalls()
	if err := os.Remove(filepath.Join(h.dir, fakeCallsFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		h.t.Fatal(err)
	}
	return calls
}

// peekCalls returns the argv of every herdr call made since the last calls.
func (h *fakeHerdr) peekCalls() [][]string {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dir, fakeCallsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		h.t.Fatal(err)
	}
	var calls [][]string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		var argv []string
		if err := json.Unmarshal(sc.Bytes(), &argv); err != nil {
			h.t.Fatalf("fake herdr log: %v", err)
		}
		calls = append(calls, argv)
	}
	return calls
}

// workspace is one workspace of a fake snapshot.
type workspace struct {
	id     string
	tabs   [][]pane          // the panes of each tab, in tab-bar and layout order
	tokens map[string]string // tokens the sidebar shows for it now
}

type pane struct {
	cwd           string
	foregroundCwd string
	focused       bool              // the session's focused pane; without one, the first workspace's first pane is
	tokens        map[string]string // pane tokens reported to it
}

// at returns a workspace with one tab holding one pane whose cwd is cwd.
func at(id, cwd string) workspace {
	return workspace{id: id, tabs: [][]pane{{{cwd: cwd}}}}
}

// snapshot sets the session `herdr api snapshot` serves, laid out the way
// Herdr 0.9.5 lays it out: workspaces in sidebar order, then for each one its
// tabs in tab-bar order, each tab's layout, and its panes in layout order.
func (h *fakeHerdr) snapshot(workspaces ...workspace) {
	h.t.Helper()
	type obj = map[string]any
	var wss, tabs, panes, layouts []obj
	focusedWs, focusedPane := 0, ""
	if len(workspaces) > 0 && len(workspaces[0].tabs) > 0 && len(workspaces[0].tabs[0]) > 0 {
		focusedPane = workspaces[0].id + ":p1"
	}
	for i, w := range workspaces {
		n := 0
		for _, tabPanes := range w.tabs {
			for _, p := range tabPanes {
				if n++; p.focused {
					focusedWs, focusedPane = i, fmt.Sprintf("%s:p%d", w.id, n)
				}
			}
		}
	}
	for i, w := range workspaces {
		paneCount := 0
		for ti, tabPanes := range w.tabs {
			tabID := fmt.Sprintf("%s:t%d", w.id, ti+1)
			tabs = append(tabs, obj{"tab_id": tabID, "workspace_id": w.id, "number": ti + 1, "label": "shell",
				"focused": ti == 0, "pane_count": len(tabPanes), "agent_status": "idle"})
			var layoutPanes []obj
			for _, p := range tabPanes {
				paneCount++
				paneID := fmt.Sprintf("%s:p%d", w.id, paneCount)
				info := obj{"pane_id": paneID, "terminal_id": "term-" + paneID, "workspace_id": w.id, "tab_id": tabID,
					"focused": paneID == focusedPane, "agent_status": "unknown", "revision": 1}
				if len(p.tokens) > 0 {
					info["tokens"] = p.tokens
				}
				if p.cwd != "" {
					info["cwd"] = p.cwd
				}
				if p.foregroundCwd != "" {
					info["foreground_cwd"] = p.foregroundCwd
				}
				panes = append(panes, info)
				layoutPanes = append(layoutPanes, obj{"pane_id": paneID, "focused": paneID == focusedPane,
					"rect": obj{"x": 0, "y": 0, "width": 80, "height": 24}})
			}
			layouts = append(layouts, obj{"workspace_id": w.id, "tab_id": tabID, "zoomed": false,
				"area": obj{"x": 0, "y": 0, "width": 80, "height": 24}, "panes": layoutPanes, "splits": []obj{}})
		}
		info := obj{"workspace_id": w.id, "number": i + 1, "label": "ws-" + w.id, "focused": i == focusedWs,
			"pane_count": paneCount, "tab_count": len(w.tabs), "active_tab_id": w.id + ":t1", "agent_status": "idle"}
		if len(w.tokens) > 0 {
			info["tokens"] = w.tokens
		}
		wss = append(wss, info)
	}
	session := obj{"version": "0.9.5", "protocol": 22,
		"workspaces": wss, "tabs": tabs, "panes": panes, "layouts": layouts, "agents": []obj{}}
	if focusedPane != "" {
		ws := workspaces[focusedWs].id
		session["focused_workspace_id"], session["focused_tab_id"], session["focused_pane_id"] = ws, ws+":t1", focusedPane
	}
	snap, err := json.Marshal(session)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.dir, fakeSnapshotFile), snap, 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// cleared stands for a token that a report clears with --clear-token.
const cleared = "(cleared)"

// tokens are the token changes of one report: name to value, or cleared.
type tokens map[string]string

// report is one workspace report-metadata call.
type report struct {
	workspace string
	seq       uint64
	tokens    tokens
}

// reports returns the workspace report-metadata calls among calls, in call
// order. It fails the test unless each is one githerd would make: source
// githerd, a numeric --seq, no --ttl-ms, and only known flags.
func reports(t *testing.T, calls [][]string) []report {
	t.Helper()
	var reps []report
	for _, argv := range calls {
		if len(argv) < 2 || argv[0] != "workspace" || argv[1] != "report-metadata" {
			continue
		}
		if len(argv) < 3 || strings.HasPrefix(argv[2], "-") {
			t.Fatalf("report without a workspace id: %q", argv)
		}
		rep := report{workspace: argv[2], tokens: tokens{}}
		source, haveSeq := "", false
		flags := argv[3:]
		for i := 0; i < len(flags); i += 2 {
			if i+1 >= len(flags) {
				t.Fatalf("flag %s without a value: %q", flags[i], argv)
			}
			value := flags[i+1]
			switch flags[i] {
			case "--source":
				source = value
			case "--seq":
				seq, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					t.Fatalf("--seq %q is not a u64: %q", value, argv)
				}
				rep.seq, haveSeq = seq, true
			case "--token":
				name, v, ok := strings.Cut(value, "=")
				if !ok {
					t.Fatalf("--token %q has no '=': %q", value, argv)
				}
				rep.tokens[name] = v
			case "--clear-token":
				rep.tokens[value] = cleared
			default:
				t.Fatalf("unexpected flag %s: %q", flags[i], argv)
			}
		}
		if source != "githerd" || !haveSeq || len(rep.tokens) == 0 {
			t.Fatalf("want --source githerd, a --seq and a token in %q", argv)
		}
		reps = append(reps, rep)
	}
	return reps
}

// tokensByWorkspace returns the tokens of each report by workspace id. It
// fails the test if a workspace got more than one report.
func tokensByWorkspace(t *testing.T, calls [][]string) map[string]tokens {
	t.Helper()
	byID := map[string]tokens{}
	for _, rep := range reports(t, calls) {
		if _, dup := byID[rep.workspace]; dup {
			t.Fatalf("workspace %s got more than one report: %q", rep.workspace, calls)
		}
		byID[rep.workspace] = rep.tokens
	}
	return byID
}

// round is the reports a sync sent under one --seq, by workspace id.
type round struct {
	seq    uint64
	tokens map[string]tokens
}

// rounds groups the reports among calls into rounds, in call order. It
// fails the test if a workspace got two reports in one round.
func rounds(t *testing.T, calls [][]string) []round {
	t.Helper()
	var rs []round
	for _, rep := range reports(t, calls) {
		if len(rs) == 0 || rs[len(rs)-1].seq != rep.seq {
			rs = append(rs, round{seq: rep.seq, tokens: map[string]tokens{}})
		}
		r := rs[len(rs)-1]
		if _, dup := r.tokens[rep.workspace]; dup {
			t.Fatalf("workspace %s got two reports under --seq %d: %q", rep.workspace, rep.seq, calls)
		}
		r.tokens[rep.workspace] = rep.tokens
	}
	return rs
}

// assertRounds fails unless the reports among calls came in the rounds of
// want, each under a higher --seq than the one before.
func assertRounds(t *testing.T, calls [][]string, want ...map[string]tokens) {
	t.Helper()
	got := rounds(t, calls)
	var tokensOf []map[string]tokens
	for i, r := range got {
		tokensOf = append(tokensOf, r.tokens)
		if i > 0 && r.seq <= got[i-1].seq {
			t.Errorf("round %d has --seq %d, not above %d", i+1, r.seq, got[i-1].seq)
		}
	}
	if !reflect.DeepEqual(tokensOf, want) {
		t.Errorf("reports by round:\n got %v\nwant %v", tokensOf, want)
	}
}

// assertSynced fails unless the sync exited 0 with no output and reported
// exactly want.
func (h *fakeHerdr) assertSynced(got result, want map[string]tokens) {
	h.t.Helper()
	if got.code != 0 || got.stdout != "" || got.stderr != "" {
		h.t.Fatalf("want exit 0 and no output; got %+v", got)
	}
	assertTokens(h.t, tokensByWorkspace(h.t, h.calls()), want)
}

func assertTokens(t *testing.T, got, want map[string]tokens) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reports by workspace:\n got %v\nwant %v", got, want)
	}
}
