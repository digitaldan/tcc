package session

import (
	"os"
	"strconv"
)

// processCwd returns a process's current working directory via /proc.
func processCwd(pid int) (string, bool) {
	dir, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	if err != nil || dir == "" {
		return "", false
	}
	return dir, true
}
