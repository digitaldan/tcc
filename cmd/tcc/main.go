package main

import (
	"fmt"
	"os"

	"github.com/digitaldan/tcc/internal/app"
	"github.com/digitaldan/tcc/internal/hookcmd"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "_hook" {
		// Invoked by Claude Code on session events; records tab status.
		hookcmd.Run()
		os.Exit(0)
	}
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tcc:", err)
		os.Exit(1)
	}
}
