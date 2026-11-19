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

// callError is a herdr call that failed: what herdr said, and the code of
// its error response, when it printed one.
type callError struct {
	call string // such as "herdr pane split"
	code string // such as "pane_not_found"; empty without an error response
	msg  string
}

func (e *callError) Error() string { return e.call + ": " + e.msg }

// errorCode returns the code of the error response a failed herdr call
// printed, or "" when err is not one.
func errorCode(err error) string {
	var ce *callError
	if errors.As(err, &ce) {
		return ce.code
	}
	return ""
}

// run runs herdr with args and returns its stdout. Its error, a
// *callError, carries what herdr printed: an error response as JSON, or
// plain usage text.
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
	ce := &callError{call: "herdr " + strings.Join(args[:min(2, len(args))], " ")}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		ce.msg = fmt.Sprintf("timed out after %v", callTimeout)
		return nil, ce
	}
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	ce.msg = strings.TrimSpace(stderr.String())
	if json.Unmarshal([]byte(ce.msg), &resp) == nil && resp.Error.Message != "" {
		ce.code, ce.msg = resp.Error.Code, resp.Error.Message
	}
	if ce.msg == "" {
		ce.msg = err.Error()
	}
	return nil, ce
}

// idIn returns the id that the JSON result of a herdr call holds at path,
// such as "pane", "pane_id" for the pane `pane split` opened. out is what
// the call printed, and call names it in the error when there is no id.
func idIn(call string, out []byte, path ...string) (string, error) {
	var node any
	if json.Unmarshal(out, &node) == nil {
		for _, key := range append([]string{"result"}, path...) {
			fields, _ := node.(map[string]any)
			node = fields[key]
		}
	}
	if id, _ := node.(string); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("%s: no %s in %s", call, strings.ReplaceAll(path[len(path)-1], "_", " "), out)
}

// snapshot is the part of Herdr's session snapshot that githerd reads.
// Workspaces come in sidebar order; tabs in tab-bar order, grouped by
// workspace; panes in layout order, grouped by tab.
type snapshot struct {
	FocusedWorkspace string `json:"focused_workspace_id"`
	FocusedPane      string `json:"focused_pane_id"`
	Workspaces       []struct {
		ID     string            `json:"workspace_id"`
		Label  string            `json:"label"`
		Tokens map[string]string `json:"tokens"`
	} `json:"workspaces"`
	Tabs []struct {
		ID          string `json:"tab_id"`
		WorkspaceID string `json:"workspace_id"`
	} `json:"tabs"`
	Panes []struct {
		ID          string            `json:"pane_id"`
		WorkspaceID string            `json:"workspace_id"`
		TabID       string            `json:"tab_id"`
		Cwd         string            `json:"cwd"`
		Tokens      map[string]string `json:"tokens"` // pane tokens, such as the rail's mark
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
