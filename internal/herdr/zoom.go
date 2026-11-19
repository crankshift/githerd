package herdr

import "context"

// ZoomMode is what Zoom does to a pane.
type ZoomMode uint8

// The zoom modes.
const (
	ZoomToggle ZoomMode = iota // zoom the pane in when it is out, and out when it is in
	ZoomOn                     // zoom the pane in to the whole tab
	ZoomOff                    // zoom the pane out
)

// zoomFlags are the flags of `herdr pane zoom` for each mode.
var zoomFlags = map[ZoomMode]string{ZoomToggle: "--toggle", ZoomOn: "--on", ZoomOff: "--off"}

// Zoom zooms pane in or out of its tab, or toggles it, as mode says. It
// names the pane, since Herdr would otherwise zoom whichever pane has focus.
func Zoom(ctx context.Context, env Env, pane string, mode ZoomMode) error {
	_, err := cli{bin: env.Bin}.run(ctx, "pane", "zoom", "--pane", pane, zoomFlags[mode])
	return err
}
