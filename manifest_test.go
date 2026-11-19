package main_test

import (
	"bufio"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The plugin builds githerd at install, and its startup hook and its
// workspace and agent event hooks all run the sync: Herdr has no timers and
// restores no tokens after a restart.
func TestPluginManifestRunsTheSyncOnStartupAndOnWorkspaceAndAgentEvents(t *testing.T) {
	m := readManifest(t, "herdr-plugin.toml")
	sync := []string{"./bin/githerd", "herdr", "sync"}

	for key, want := range map[string]any{
		"id":                "githerd",
		"min_herdr_version": "0.9.5",
		"platforms":         []any{"linux", "macos"},
	} {
		if got := m.top[key]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if got, want := m.commands("build"), [][]string{{"go", "build", "-o", "bin/githerd", "."}}; !reflect.DeepEqual(got, want) {
		t.Errorf("[[build]] commands %q, want %q", got, want)
	}
	if got, want := m.commands("startup"), [][]string{sync}; !reflect.DeepEqual(got, want) {
		t.Errorf("[[startup]] commands %q, want %q", got, want)
	}
	events := map[string][]string{}
	for _, e := range m.tables["events"] {
		on, _ := e["on"].(string)
		events[on] = argv(e["command"])
	}
	want := map[string][]string{
		"workspace.created":         sync,
		"workspace.closed":          sync,
		"workspace.focused":         sync,
		"pane.agent_status_changed": sync,
	}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("[[events]]:\n got %q\nwant %q", events, want)
	}
}

// manifest is a herdr-plugin.toml, read just far enough for these tests:
// top-level keys, and the keys of each [[array]] table, with values that are
// strings or arrays of strings.
type manifest struct {
	top    map[string]any
	tables map[string][]map[string]any
}

func readManifest(t *testing.T, path string) manifest {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := manifest{top: map[string]any{}, tables: map[string][]map[string]any{}}
	current := m.top
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if name, ok := strings.CutPrefix(line, "[["); ok {
			name = strings.TrimSuffix(name, "]]")
			current = map[string]any{}
			m.tables[name] = append(m.tables[name], current)
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("%s:%d: not a key = value line: %s", path, n, line)
		}
		var v any
		if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &v); err != nil {
			t.Fatalf("%s:%d: value is not a plain string or array of strings: %s", path, n, line)
		}
		current[strings.TrimSpace(key)] = v
	}
	return m
}

// commands returns the command of each [[name]] table.
func (m manifest) commands(name string) [][]string {
	var cmds [][]string
	for _, table := range m.tables[name] {
		cmds = append(cmds, argv(table["command"]))
	}
	return cmds
}

// argv returns a command value as a list of strings; nil when it is not one.
func argv(command any) []string {
	args, _ := command.([]any)
	var argv []string
	for _, arg := range args {
		s, ok := arg.(string)
		if !ok {
			return nil
		}
		argv = append(argv, s)
	}
	return argv
}
