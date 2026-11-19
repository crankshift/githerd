package term

import "syscall"

// The ioctl requests that get and set a terminal's attributes.
const (
	getTermios = syscall.TCGETS
	setTermios = syscall.TCSETS
)
