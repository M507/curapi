//go:build !linux

package daemon

func isZombie(pid int) bool {
	return false
}
