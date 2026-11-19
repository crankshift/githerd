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

// OpenShell opens a shell in dir beside the rail and focuses it: in a split
// to the right of beside, the pane the rail was opened from, which has the
// width the rail lacks, or below rail, the rail's own pane, when beside is
// gone or unknown. With command set, it then runs command in the shell.
// Panes are always named: without --pane, Herdr would split whichever pane
// has focus.
func OpenShell(ctx context.Context, env Env, beside, rail, dir, command string) error {
	herdr := cli{bin: env.Bin}
	split := func(pane, direction string) ([]byte, error) {
		return herdr.run(ctx, "pane", "split", "--pane", pane, "--direction", direction, "--cwd", dir, "--focus")
	}
	var out []byte
	var err error
	if beside != "" {
		out, err = split(beside, "right")
	}
	if beside == "" || errorCode(err) == "pane_not_found" {
		out, err = split(rail, "down")
	}
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
