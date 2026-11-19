//go:build darwin || linux

// Package term is the thin terminal layer under the TUI: raw mode through
// termios, the alternate screen, the terminal's size, and the keys read
// from it.
package term

import (
	"errors"
	"os"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

// Escape sequences that enter and leave the TUI's screen: the alternate
// screen, with the cursor hidden and cleared to start.
const (
	enter = "\x1b[?1049h\x1b[?25l\x1b[2J"
	leave = "\x1b[0m\x1b[?25h\x1b[?1049l"
)

// Terminal is a terminal in raw mode showing the alternate screen.
type Terminal struct {
	in, out *os.File
	saved   syscall.Termios
}

// Open puts the terminal on in and out into raw mode and switches to the
// alternate screen. Close puts both back. It fails when in is not a
// terminal.
func Open(in, out *os.File) (*Terminal, error) {
	t := &Terminal{in: in, out: out}
	if err := ioctl(in, getTermios, unsafe.Pointer(&t.saved)); err != nil {
		return nil, errors.New("not a terminal")
	}
	raw := t.saved
	// As cfmakeraw: bytes as typed, no echo, no signals from keys, no
	// output processing.
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN], raw.Cc[syscall.VTIME] = 1, 0
	if err := ioctl(in, setTermios, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	if _, err := out.WriteString(enter); err != nil {
		t.Close()
		return nil, err
	}
	return t, nil
}

// Close leaves the alternate screen and restores the terminal's mode.
func (t *Terminal) Close() error {
	_, werr := t.out.WriteString(leave)
	return errors.Join(werr, ioctl(t.in, setTermios, unsafe.Pointer(&t.saved)))
}

// Size returns the terminal's width and height in cells.
func (t *Terminal) Size() (width, height int, err error) {
	var ws struct{ Row, Col, X, Y uint16 }
	if err := ioctl(t.out, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return 0, 0, err
	}
	return int(ws.Col), int(ws.Row), nil
}

// Draw writes a frame in one write, so it never shows half drawn.
func (t *Terminal) Draw(frame string) error {
	_, err := t.out.WriteString(frame)
	return err
}

func ioctl(f *os.File, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// Keys returns the keys in b, a chunk read from a raw terminal: characters
// as themselves, and "up", "down", "left", "right", "enter", "esc",
// "backspace" and "ctrl+c" for those keys. Other control keys and escape
// sequences are dropped. A lone ESC is Esc: a key sends its whole escape
// sequence in one write, so it arrives in one chunk.
func Keys(b []byte) []string {
	var keys []string
	for len(b) > 0 {
		switch c := b[0]; {
		case c == 0x1b && len(b) > 1 && (b[1] == '[' || b[1] == 'O'):
			end := 2
			for end < len(b) && (b[end] < 0x40 || b[end] > 0x7e) {
				end++
			}
			if end < len(b) {
				if name, ok := arrows[b[end]]; ok {
					keys = append(keys, name)
				}
				end++
			}
			b = b[end:]
		case c == 0x1b:
			keys, b = append(keys, "esc"), b[1:]
		case c == '\r' || c == '\n':
			keys, b = append(keys, "enter"), b[1:]
		case c == 0x7f || c == 0x08:
			keys, b = append(keys, "backspace"), b[1:]
		case c == 0x03:
			keys, b = append(keys, "ctrl+c"), b[1:]
		case c < 0x20:
			b = b[1:]
		default:
			r, n := utf8.DecodeRune(b)
			if r != utf8.RuneError {
				keys = append(keys, string(r))
			}
			b = b[n:]
		}
	}
	return keys
}

// arrows maps the final byte of an arrow key's escape sequence to its name.
var arrows = map[byte]string{'A': "up", 'B': "down", 'C': "right", 'D': "left"}
