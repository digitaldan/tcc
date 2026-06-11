package main

import (
	"fmt"

	"github.com/dcunningham/ctmux/internal/claude"
)

func main() {
	for i, rs := range claude.ListSessions() {
		if i >= 12 {
			break
		}
		bg := ""
		if rs.Background {
			bg = " [bg]"
		}
		fmt.Printf("%-45.45s %-55.55s %s%s\n", rs.Title, rs.Dir, rs.SessionID[:8], bg)
	}
}
