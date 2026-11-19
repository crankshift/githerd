package herdr

import "context"

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
	return idIn("herdr workspace create", out, "workspace", "workspace_id")
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
	shell, err := idIn("herdr pane split", out, "pane", "pane_id")
	if err != nil {
		return err
	}
	_, err = herdr.run(ctx, "pane", "run", shell, command)
	return err
}
