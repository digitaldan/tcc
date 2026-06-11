package app

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	debugFile *os.File
	debugOnce sync.Once
)

// debugf appends to the file named by TCC_DEBUG, if set. No-op otherwise.
func debugf(format string, args ...any) {
	debugOnce.Do(func() {
		if path := os.Getenv("TCC_DEBUG"); path != "" {
			debugFile, _ = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		}
	})
	if debugFile == nil {
		return
	}
	fmt.Fprintf(debugFile, time.Now().Format("15:04:05.000 ")+format+"\n", args...)
}
