package main_test

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

const getTermiosReq = syscall.TIOCGETA

// openPTY opens a pseudo-terminal and returns its master and slave sides.
func openPTY() (master, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	var name [128]byte
	for _, req := range []struct {
		req uintptr
		arg unsafe.Pointer
	}{{syscall.TIOCPTYGRANT, nil}, {syscall.TIOCPTYUNLK, nil}, {syscall.TIOCPTYGNAME, unsafe.Pointer(&name)}} {
		if err := ioctl(master, req.req, req.arg); err != nil {
			master.Close()
			return nil, nil, err
		}
	}
	slave, err = os.OpenFile(string(name[:bytes.IndexByte(name[:], 0)]), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
