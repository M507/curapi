//go:build linux

package daemon

import (
	"fmt"
	"os"
	"strings"
)

func isZombie(pid int) bool {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(line)
			return len(fields) >= 2 && fields[1] == "Z"
		}
	}
	return false
}
