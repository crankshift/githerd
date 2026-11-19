package herdr

import "context"

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
	snap, err := herdr.snapshot(ctx)
	if err != nil {
		return err
	}
	for _, p := range snap.Panes {
		if p.WorkspaceID != snap.FocusedWorkspace || p.Tokens[railToken] != railValue {
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
	out, err := herdr.run(ctx, append(args, "--direction", "right", "--focus")...)
	if err != nil {
		return err
	}
	rail, err := idIn("herdr plugin pane open", out, "plugin_pane", "pane", "pane_id")
	if err != nil {
		return err
	}
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
