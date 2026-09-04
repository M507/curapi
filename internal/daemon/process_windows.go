//go:build windows

package daemon

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("tasklist", "/NH", "/FI", fmt.Sprintf("PID eq %d", pid)).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strconv.Itoa(pid))
}

func Terminate(pid int) error {
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid)).Run()
}

func Kill(pid int) error {
	return exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
}
