package main_test

import (
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func ioctl(f *os.File, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// pty is a pseudo-terminal for running githerd as a terminal would, with
// everything githerd writes to it kept.
type pty struct {
	t             *testing.T
	master, slave *os.File
	mu            sync.Mutex
	out           strings.Builder
}

func newPTY(t *testing.T, width, height int) *pty {
	t.Helper()
	master, slave, err := openPTY()
	if err != nil {
		t.Fatal(err)
	}
	p := &pty{t: t, master: master, slave: slave}
	t.Cleanup(func() { slave.Close(); master.Close() })
	p.resize(width, height)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			p.mu.Lock()
			p.out.Write(buf[:n])
			p.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return p
}

// resize sets the terminal's size, which signals SIGWINCH to the program
// in the foreground.
func (p *pty) resize(width, height int) {
	p.t.Helper()
	ws := struct{ Row, Col, X, Y uint16 }{Row: uint16(height), Col: uint16(width)}
	if err := ioctl(p.slave, syscall.TIOCSWINSZ, unsafe.Pointer(&ws)); err != nil {
		p.t.Fatal(err)
	}
}

// termios returns the terminal's mode. It reads it through the master,
// which still works once a session that had the terminal as its
// controlling terminal has ended and the slave was hung up.
func (p *pty) termios() syscall.Termios {
	p.t.Helper()
	var t syscall.Termios
	if err := ioctl(p.master, getTermiosReq, unsafe.Pointer(&t)); err != nil {
		p.t.Fatal(err)
	}
	return t
}

// typed sends keys as the user would type them.
func (p *pty) typed(keys string) {
	p.t.Helper()
	if _, err := p.master.WriteString(keys); err != nil {
		p.t.Fatal(err)
	}
}

// output returns everything written to the terminal so far.
func (p *pty) output() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out.String()
}

// waitFor waits until the output written since from holds s and returns
// where s ends, failing the test after 10 seconds.
func (p *pty) waitFor(from int, s string) int {
	p.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		out := p.output()
		if i := strings.Index(out[from:], s); i >= 0 {
			return from + i + len(s)
		}
		if time.Now().After(deadline) {
			p.t.Fatalf("waited 10s for %q in the terminal; got %q", s, out[from:])
		}
	}
}
