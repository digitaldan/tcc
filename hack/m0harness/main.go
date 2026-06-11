// m0harness runs tcc inside a PTY, captures everything it draws, replays
// the byte stream through a vt emulator, and prints the final screen as
// plain text — a headless way to verify the M0 rendering gate.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	// side effect: 0x9C-in-OSC parser patch, same as tcc proper
	_ "github.com/digitaldan/tcc/internal/term"
)

func main() {
	bin := flag.String("bin", "./tcc", "tcc binary")
	dir := flag.String("dir", ".", "working directory for tcc")
	wait := flag.Duration("wait", 15*time.Second, "time to let claude start")
	keys := flag.String("keys", "", "bytes to send after wait (supports \\x11 etc. via Go quoting upstream)")
	cols := flag.Int("cols", 100, "columns")
	rows := flag.Int("rows", 30, "rows")
	rawOut := flag.String("raw", "", "file to append all captured PTY output bytes to")
	argsFlag := flag.String("binargs", "", "space-separated arguments for the binary")
	flag.Parse()

	var binArgs []string
	if *argsFlag != "" {
		binArgs = strings.Fields(*argsFlag)
	}

	cmd := exec.Command(*bin, binArgs...)
	cmd.Dir = *dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(*rows), Cols: uint16(*cols)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		os.Exit(1)
	}

	var rawFile *os.File
	if *rawOut != "" {
		rawFile, _ = os.Create(*rawOut)
		defer rawFile.Close()
	}

	em := vt.NewSafeEmulator(*cols, *rows)
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := f.Read(buf)
			if n > 0 {
				_, _ = em.Write(buf[:n])
				if rawFile != nil {
					_, _ = rawFile.Write(buf[:n])
				}
			}
			if rerr != nil {
				close(done)
				return
			}
		}
	}()
	// Drain emulator responses back to tcc (it may query the terminal too).
	go func() { _, _ = io.Copy(f, em) }()

	time.Sleep(*wait)

	if *keys != "" {
		_, _ = f.WriteString(*keys)
		time.Sleep(5 * time.Second)
	}

	// Remaining args form a script: durations sleep, "ENTER"/"ESC"/"CTRLQ"
	// send control bytes, "SNAP" prints the screen, anything else is typed.
	for _, step := range flag.Args() {
		switch step {
		case "ENTER":
			_, _ = f.Write([]byte("\r"))
		case "ESC":
			_, _ = f.Write([]byte{0x1b})
		case "CTRLQ":
			_, _ = f.Write([]byte{0x11})
		case "CTRLU":
			_, _ = f.Write([]byte{0x15})
		case "NUDGE":
			// Force a SIGWINCH repaint: shrink one column, restore.
			_ = pty.Setsize(f, &pty.Winsize{Rows: uint16(*rows), Cols: uint16(*cols - 1)})
			time.Sleep(80 * time.Millisecond)
			_ = pty.Setsize(f, &pty.Winsize{Rows: uint16(*rows), Cols: uint16(*cols)})
		case "SNAP":
			fmt.Println("===== SCREEN =====")
			fmt.Println(em.String())
			fmt.Println("===== END =====")
		default:
			if d, derr := time.ParseDuration(step); derr == nil {
				time.Sleep(d)
			} else {
				_, _ = f.WriteString(step)
			}
		}
	}

	fmt.Println("===== SCREEN =====")
	fmt.Println(em.String())
	fmt.Println("===== END =====")

	// Quit tcc: Ctrl+Q then d
	_, _ = f.Write([]byte{0x11})
	time.Sleep(300 * time.Millisecond)
	_, _ = f.Write([]byte("d"))

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		fmt.Println("(timeout waiting for exit; killing)")
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}
