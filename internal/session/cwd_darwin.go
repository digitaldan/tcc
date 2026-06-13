package session

import (
	"bytes"
	"syscall"
	"unsafe"
)

// macOS exposes a process's cwd via proc_pidinfo(PROC_PIDVNODEPATHINFO),
// reached through the proc_info syscall (no cgo). The cwd path lives at a
// fixed offset inside the returned proc_vnodepathinfo struct.
const (
	sysProcInfo          = 336 // SYS_proc_info
	procInfoCallPidInfo  = 2   // PROC_INFO_CALL_PIDINFO
	procPIDVnodePathInfo = 9   // PROC_PIDVNODEPATHINFO
	vipPathOffset        = 152 // offset of vip_path within vnode_info_path
	maxPathLen           = 1024
	vnodePathInfoSize    = 2352 // sizeof(struct proc_vnodepathinfo)
)

// processCwd returns a process's current working directory.
func processCwd(pid int) (string, bool) {
	buf := make([]byte, vnodePathInfoSize)
	r, _, errno := syscall.Syscall6(sysProcInfo,
		uintptr(procInfoCallPidInfo), uintptr(pid), uintptr(procPIDVnodePathInfo),
		0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if errno != 0 || int(r) < vipPathOffset+1 {
		return "", false
	}
	path := buf[vipPathOffset : vipPathOffset+maxPathLen]
	if i := bytes.IndexByte(path, 0); i >= 0 {
		path = path[:i]
	}
	if len(path) == 0 {
		return "", false
	}
	return string(path), true
}
