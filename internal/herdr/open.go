package herdr

import (
	"context"
	"encoding/json"
	"fmt"
)

// FocusWorkspace focuses workspace id.
func FocusWorkspace(ctx context.Context, env Env, id string) error {
	_, err := cli{bin: env.Bin}.run(ctx, "workspace", "focus", id)
	return err
}

// CreateWorkspace creates a workspace whose first pane's cwd is dir,
// labelled label, focuses it, and returns its id.
func CreateWorkspace(ctx context.Context, env Env, dir, label string) (string, error) {
	out, err := cli{bin: env.Bin}.run(ctx, "workspace", "create", "--cwd", dir, "--label", label, "--focus")
	if err != nil {
		return "", err
	}
	var resp struct {
		Result struct {
			Workspace struct {
				ID string `json:"workspace_id"`
			} `json:"workspace"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || resp.Result.Workspace.ID == "" {
		return "", fmt.Errorf("herdr workspace create: no workspace id in %s", out)
	}
	return resp.Result.Workspace.ID, nil
}

// OpenShell splits pane with a shell in dir below it and focuses the shell.
// With command set, it then runs command in the shell. The pane is always
// named: without --pane, Herdr would split whichever pane has focus.
func OpenShell(ctx context.Context, env Env, pane, dir, command string) error {
	herdr := cli{bin: env.Bin}
	out, err := herdr.run(ctx, "pane", "split", "--pane", pane, "--direction", "down", "--cwd", dir, "--focus")
	if err != nil || command == "" {
		return err
	}
	var resp struct {
		Result struct {
			Pane struct {
				ID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || resp.Result.Pane.ID == "" {
		return fmt.Errorf("herdr pane split: no pane id in %s", out)
	}
	_, err = herdr.run(ctx, "pane", "run", resp.Result.Pane.ID, command)
	return err
}
