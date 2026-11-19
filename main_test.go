package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// githerdBin is the githerd binary built once for the whole test run.
var githerdBin string

func TestMain(m *testing.M) {
	os.Exit(buildAndRun(m))
}

func buildAndRun(m *testing.M) int {
	dir, err := os.MkdirTemp("", "githerd-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	githerdBin = filepath.Join(dir, "githerd")
	// No VCS stamping: it would run git with the user's own config.
	build := exec.Command("go", "build", "-buildvcs=false", "-o", githerdBin, ".")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building githerd:", err)
		return 1
	}
	return m.Run()
}

// result is what one run of the githerd binary produced.
type result struct {
	stdout, stderr string
	code           int
}

// githerd runs the built binary in dir with env and args. Like a shell, it
// sets PWD to dir.
func githerd(t *testing.T, env []string, dir string, args ...string) result {
	t.Helper()
	cmd := exec.Command(githerdBin, args...)
	cmd.Dir = dir
	cmd.Env = append(append([]string(nil), env...), "PWD="+dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("running githerd: %v", err)
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: cmd.ProcessState.ExitCode()}
}

// assertOutput fails unless the run exited 0 with exactly want on stdout.
func assertOutput(t *testing.T, got result, want string) {
	t.Helper()
	if got.code != 0 {
		t.Fatalf("exit code %d, stderr:\n%s", got.code, got.stderr)
	}
	if got.stdout != want {
		t.Errorf("stdout mismatch\n--- got ---\n%s\n--- want ---\n%s", got.stdout, want)
	}
}
