//go:build !linux && !darwin

package session

// processCwd has no portable implementation on this platform; terminal tabs
// fall back to the shell's OSC reports for cwd/title.
func processCwd(pid int) (string, bool) { return "", false }
