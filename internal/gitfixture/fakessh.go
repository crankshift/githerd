package gitfixture

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HangingHost is a host the fake ssh hangs on for HangTime.
const HangingHost = "hang.example.com"

// HangTime is how long the fake ssh hangs on HangingHost before it logs that
// it outlived the fetch and exits, unless it was killed first.
const HangTime = 2 * time.Second

// fakeSSH logs each call. For HangingHost it then hangs, as a host that
// drops packets would; for any other host it fails at once, as an
// unreachable host would. It never touches the network.
const fakeSSH = `#!/bin/sh
printf 'GIT_TERMINAL_PROMPT=%s %s\n' "${GIT_TERMINAL_PROMPT-unset}" "$*" >> '{{log}}'
case "$*" in
*` + HangingHost + `*)
	sleep {{hang}}
	printf 'outlived the fetch: %s\n' "$*" >> '{{log}}'
	exit 255 ;;
esac
echo 'ssh: connect to host port 22: Connection refused' >&2
exit 255
`

// FakeSSH puts a fake ssh first on the PATH of Env, so git reaches no real
// host over ssh. See SSHCalls for what it records.
func (f *Fixture) FakeSSH() {
	f.t.Helper()
	bin := f.Path("bin")
	f.mkdir(bin)
	script := strings.NewReplacer(
		"{{log}}", f.sshLog(),
		"{{hang}}", strconv.Itoa(int(HangTime/time.Second)),
	).Replace(fakeSSH)
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755); err != nil {
		f.t.Fatal(err)
	}
	for i, kv := range f.env {
		if path, ok := strings.CutPrefix(kv, "PATH="); ok {
			f.env[i] = "PATH=" + bin + string(os.PathListSeparator) + path
		}
	}
}

// SSHCalls returns one line per call of the fake ssh: the GIT_TERMINAL_PROMPT
// it saw ("unset" when it had none), then its arguments, such as
// "GIT_TERMINAL_PROMPT=0 -o BatchMode=yes git@host git-upload-pack '/a.git'".
// A call that hung for HangTime without being killed adds a second line,
// "outlived the fetch: " and its arguments.
func (f *Fixture) SSHCalls() []string {
	f.t.Helper()
	b, err := os.ReadFile(f.sshLog())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func (f *Fixture) sshLog() string { return f.Path("ssh-calls.log") }
