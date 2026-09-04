//go:build unix

package daemon

import "syscall"

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	return !isZombie(pid)
}

func Terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func Kill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
