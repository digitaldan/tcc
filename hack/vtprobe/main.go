// vtprobe feeds a captured raw terminal byte stream into the vt emulator one
// byte at a time and reports the offset at which a target row's content
// starts matching a marker — for pinpointing emulation bugs.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/vt"
)

func rowString(em *vt.Emulator, y int) string {
	var sb strings.Builder
	for x := 0; x < em.Width(); x++ {
		c := em.CellAt(x, y)
		if c == nil || c.Content == "" {
			continue
		}
		sb.WriteString(c.Content)
	}
	return sb.String()
}

func main() {
	file := flag.String("file", "", "raw capture file")
	cols := flag.Int("cols", 100, "columns")
	rows := flag.Int("rows", 29, "rows")
	row := flag.Int("row", 0, "row to watch (0-based)")
	marker := flag.String("marker", "ClauClau", "substring that signals corruption")
	ctx := flag.Int("ctx", 300, "bytes of context to dump")
	flag.Parse()

	data, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	em := vt.NewEmulator(*cols, *rows)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := em.Read(buf); err != nil {
				return
			}
		}
	}()

	for i := 0; i < len(data); i++ {
		_, _ = em.Write(data[i : i+1])
		if strings.Contains(rowString(em, *row), *marker) {
			fmt.Printf("corruption first present after byte offset %d\n", i)
			s := i - *ctx
			if s < 0 {
				s = 0
			}
			fmt.Printf("context %d..%d:\n%q\n", s, i+1, data[s:i+1])
			return
		}
	}
	fmt.Println("marker never appeared; final row content:")
	fmt.Println(rowString(em, *row))
}
