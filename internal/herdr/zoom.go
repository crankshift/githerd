package herdr

import "context"

// Zoom zooms pane in or out of its tab, or toggles it, as mode says: on,
// off or toggle. It names the pane, since Herdr would otherwise zoom
// whichever pane has focus.
func Zoom(ctx context.Context, env Env, pane, mode string) error {
	_, err := cli{bin: env.Bin}.run(ctx, "pane", "zoom", "--pane", pane, "--"+mode)
	return err
}
