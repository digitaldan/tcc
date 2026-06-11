package main

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
)

func main() {
	p := ansi.NewParser()
	p.SetParamsSize(32)
	p.SetDataSize(1024)
	p.SetHandler(ansi.Handler{
		Print:   func(r rune) { fmt.Printf("PRINT %q\n", r) },
		Execute: func(b byte) { fmt.Printf("EXEC %q\n", b) },
		HandleOsc: func(cmd int, data []byte) {
			fmt.Printf("OSC cmd=%d data=%q\n", cmd, data)
		},
	})
	seq := []byte("\x1b]0;✳ Claude Code\x07X")
	for i := range seq {
		p.Advance(seq[i])
	}
}
