// m0harness runs ctmux inside a PTY, captures everything it draws, replays
// the byte stream through a vt emulator, and prints the final screen as
// plain text — a headless way to verify the M0 rendering gate.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

func main() {
	bin := flag.String("bin", "./ctmux", "ctmux binary")
	dir := flag.String("dir", ".", "working directory for ctmux")
	wait := flag.Duration("wait", 15*time.Second, "time to let claude start")
	keys := flag.String("keys", "", "bytes to send after wait (supports \\x11 etc. via Go quoting upstream)")
	cols := flag.Int("cols", 100, "columns")
	rows := flag.Int("rows", 30, "rows")
	flag.Parse()

	cmd := exec.Command(*bin)
	cmd.Dir = *dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(*rows), Cols: uint16(*cols)})
	if err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		os.Exit(1)
	}

	em := vt.NewSafeEmulator(*cols, *rows)
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := f.Read(buf)
			if n > 0 {
				_, _ = em.Write(buf[:n])
			}
			if rerr != nil {
				close(done)
				return
			}
		}
	}()
	// Drain emulator responses back to ctmux (it may query the terminal too).
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

	// Quit ctmux: Ctrl+Q then d
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
