package term

import "github.com/charmbracelet/x/ansi/parser"

// Claude Code sets the terminal title to strings like "✳ Claude Code".
// U+2733 encodes as E2 9C B3, and x/ansi's transition table treats a raw
// 0x9C inside OSC/DCS/SOS/PM/APC strings as the 8-bit C1 String Terminator —
// the sequence dispatches mid-rune and the rest of the title is printed into
// the screen. Modern UTF-8 terminals (xterm in UTF-8 mode, kitty, wezterm)
// do not honor raw 8-bit ST; remap 0x9C to data collection so multi-byte
// UTF-8 survives. Sequences still terminate via BEL or ESC \.
func init() {
	for _, st := range []parser.State{
		parser.OscStringState,
		parser.DcsStringState,
		parser.SosStringState,
		parser.PmStringState,
		parser.ApcStringState,
	} {
		parser.Table.AddOne(0x9c, st, parser.PutAction, st)
	}
}
