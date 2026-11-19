package herdr

import "encoding/json"

// Origin is the pane a plugin pane was opened from: for a split, the pane
// it split, as it was at open time.
type Origin struct {
	Pane string // its id; empty when the context doesn't say
	Cwd  string // its cwd, with symlinks resolved as a Root's Dir has; empty when the context doesn't say
}

// OpenedFrom returns the pane a plugin pane was opened from, read from its
// HERDR_PLUGIN_CONTEXT_JSON.
func OpenedFrom(contextJSON string) Origin {
	var c struct {
		FocusedPaneID  string `json:"focused_pane_id"`
		FocusedPaneCwd string `json:"focused_pane_cwd"`
		WorkspaceCwd   string `json:"workspace_cwd"`
	}
	if json.Unmarshal([]byte(contextJSON), &c) != nil {
		return Origin{}
	}
	o := Origin{Pane: c.FocusedPaneID, Cwd: canonical(c.FocusedPaneCwd)}
	if o.Cwd == "" {
		o.Cwd = canonical(c.WorkspaceCwd)
	}
	return o
}
