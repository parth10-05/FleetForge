//go:build !windows

package simulatorstate

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockExclusive(file *os.File) error   { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func unlockExclusive(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_UN) }
