package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// callTimeout bounds one herdr call, so a wedged Herdr can't hold a hook.
const callTimeout = 10 * time.Second

// cli runs the herdr CLI.
type cli struct{ bin string }

// snapshot returns the session snapshot.
func (c cli) snapshot(ctx context.Context) (snapshot, error) {
	out, err := c.run(ctx, "api", "snapshot")
	if err != nil {
		return snapshot{}, err
	}
	var resp struct {
		Result struct {
			Type     string   `json:"type"`
			Snapshot snapshot `json:"snapshot"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return snapshot{}, fmt.Errorf("herdr api snapshot: %v", err)
	}
	if resp.Result.Type != "session_snapshot" {
		return snapshot{}, fmt.Errorf("herdr api snapshot: result type %q, want session_snapshot", resp.Result.Type)
	}
	return resp.Result.Snapshot, nil
}

// report sets the tokens of workspace id that toks holds and clears the
// other names githerd reports, all in one call. There is never a TTL: an
// expired token would vanish, and a missing token reads as all clean.
func (c cli) report(ctx context.Context, id string, seq uint64, toks map[string]string) error {
	args := []string{"workspace", "report-metadata", id, "--source", source, "--seq", strconv.FormatUint(seq, 10)}
	for _, name := range tokenNames {
		if v, ok := toks[name]; ok {
			args = append(args, "--token", name+"="+v)
		}
	}
	for _, name := range tokenNames {
		if _, ok := toks[name]; !ok {
			args = append(args, "--clear-token", name)
		}
	}
	_, err := c.run(ctx, args...)
	return err
}

// run runs herdr with args and returns its stdout. Its error carries the
// message herdr printed: an error response as JSON, or plain usage text.
func (c cli) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	name := "herdr " + strings.Join(args[:min(2, len(args))], " ")
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("%s: timed out after %v", name, callTimeout)
	}
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := strings.TrimSpace(stderr.String())
	if json.Unmarshal([]byte(msg), &resp) == nil && resp.Error.Message != "" {
		msg = resp.Error.Message
	}
	if msg == "" {
		msg = err.Error()
	}
	return nil, fmt.Errorf("%s: %s", name, msg)
}

// snapshot is the part of Herdr's session snapshot that githerd reads.
// Workspaces come in sidebar order; tabs in tab-bar order, grouped by
// workspace; panes in layout order, grouped by tab.
type snapshot struct {
	Workspaces []struct {
		ID     string            `json:"workspace_id"`
		Label  string            `json:"label"`
		Tokens map[string]string `json:"tokens"`
	} `json:"workspaces"`
	Tabs []struct {
		ID          string `json:"tab_id"`
		WorkspaceID string `json:"workspace_id"`
	} `json:"tabs"`
	Panes []struct {
		TabID string `json:"tab_id"`
		Cwd   string `json:"cwd"`
	} `json:"panes"`
}

// firstPaneCwd returns the cwd of the first pane listed for the first tab
// of workspace id, or "" when it has none. The snapshot has no layout tree,
// so this stands in for Herdr's root pane; the two differ only when panes
// were moved in that tab.
func (s snapshot) firstPaneCwd(id string) string {
	for _, t := range s.Tabs {
		if t.WorkspaceID != id {
			continue
		}
		for _, p := range s.Panes {
			if p.TabID == t.ID {
				return p.Cwd
			}
		}
		return ""
	}
	return ""
}
