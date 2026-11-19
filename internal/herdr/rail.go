package herdr

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// The plugin and pane that run the rail, as the manifest declares them.
const (
	pluginID  = "githerd"
	railEntry = "rail"
)

// The pane token that marks the rail's pane, so the rail action can find it
// in the snapshot: Herdr's own registry of plugin panes can't be queried,
// and is lost on restart.
const (
	railToken = "githerd"
	railValue = "rail"
)

// railWidth is how far the rail action moves the split after opening the
// rail. A split opens at half the width and the amount is a ratio delta, so
// the rail ends up with about a quarter.
const railWidth = "0.25"

// OpenRail focuses the rail if a pane of the current workspace carries the
// rail's token. Otherwise it opens the rail as a split to the right of the
// focused pane, marks it, and narrows it.
func OpenRail(ctx context.Context, env Env) error {
	herdr := cli{bin: env.Bin}
	out, err := herdr.run(ctx, "api", "snapshot")
	if err != nil {
		return err
	}
	var resp struct {
		Result struct {
			Snapshot struct {
				FocusedWorkspace string `json:"focused_workspace_id"`
				FocusedPane      string `json:"focused_pane_id"`
				Panes            []struct {
					ID        string            `json:"pane_id"`
					Workspace string            `json:"workspace_id"`
					Tokens    map[string]string `json:"tokens"`
				} `json:"panes"`
			} `json:"snapshot"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return fmt.Errorf("herdr api snapshot: %v", err)
	}
	snap := resp.Result.Snapshot
	for _, p := range snap.Panes {
		if p.Workspace != snap.FocusedWorkspace || p.Tokens[railToken] != railValue {
			continue
		}
		// Focusing fails on a pane Herdr no longer counts as a plugin
		// pane, though it kept the token, as after a live handoff.
		if _, err := herdr.run(ctx, "plugin", "pane", "focus", p.ID); err == nil {
			return nil
		}
	}

	args := []string{"plugin", "pane", "open", "--plugin", pluginID, "--entrypoint", railEntry, "--placement", "split"}
	if p := snap.FocusedPane; p != "" {
		args = append(args, "--target-pane", p)
	}
	out, err = herdr.run(ctx, append(args, "--direction", "right", "--focus")...)
	if err != nil {
		return err
	}
	var opened struct {
		Result struct {
			PluginPane struct {
				Pane struct {
					ID string `json:"pane_id"`
				} `json:"pane"`
			} `json:"plugin_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &opened); err != nil || opened.Result.PluginPane.Pane.ID == "" {
		return fmt.Errorf("herdr plugin pane open: no pane id in %s", out)
	}
	rail := opened.Result.PluginPane.Pane.ID
	if err := MarkRail(ctx, env, rail); err != nil {
		return err
	}
	_, err = herdr.run(ctx, "pane", "resize", "--pane", rail, "--direction", "right", "--amount", railWidth)
	return err
}

// MarkRail reports the rail's token on pane. It carries no --seq, so it
// always applies.
func MarkRail(ctx context.Context, env Env, pane string) error {
	_, err := cli{bin: env.Bin}.run(ctx, "pane", "report-metadata", pane, "--source", source, "--token", railToken+"="+railValue)
	return err
}

// RootsFile returns the path of the roots file in the plugin config dir.
func RootsFile(configDir string) string { return filepath.Join(configDir, rootsFile) }

// ReadRoots returns the roots that the roots file in the plugin config dir
// lists, as Sync reads them, and the problems with its lines.
func ReadRoots(configDir string) ([]string, []error) { return readRoots(RootsFile(configDir)) }

// OpenedFrom returns the cwd of the pane a plugin pane was opened from, read
// from HERDR_PLUGIN_CONTEXT_JSON: for a split, the pane it split, as it was
// at open time. It returns "" when the context doesn't say.
func OpenedFrom(contextJSON string) string {
	var c struct {
		FocusedPaneCwd string `json:"focused_pane_cwd"`
		WorkspaceCwd   string `json:"workspace_cwd"`
	}
	if json.Unmarshal([]byte(contextJSON), &c) != nil {
		return ""
	}
	if c.FocusedPaneCwd != "" {
		return c.FocusedPaneCwd
	}
	return c.WorkspaceCwd
}
