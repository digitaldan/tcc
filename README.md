# ctmux

A tabbed terminal manager for [Claude Code](https://claude.com/claude-code) sessions — like tmux, but purpose-built around Claude. One window, one tab per Claude session, with live status badges showing which session is thinking, which one is waiting on you, and which one hit an error.

```
┌──────────────────────────────────────────────────────────────┐
│ 1:api-fix ✶  2:webapp ●  3:infra ○          ^Q for commands  │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│   (the active Claude Code session, live)                     │
│                                                              │
└──────────────────────────────────────────────────────────────┘
   ✶ busy   ● needs input   ○ idle   ◌ starting   ✕ error   ▢ exited
```

## How it works

- **Embedded terminals, no tmux required.** Each `claude` child runs in its own PTY, parsed by an embedded terminal emulator ([charmbracelet/x/vt](https://github.com/charmbracelet/x)). The active tab's screen renders below a one-row tab bar; background sessions keep running and stay renderable for instant switching.
- **Status from Claude Code's own hooks.** ctmux passes each session `--settings ~/.ctmux/hooks-settings.json` (merged by Claude with your own settings — they're untouched), registering `ctmux _hook` for `SessionStart`, `UserPromptSubmit`, `Stop`, `StopFailure`, `PermissionRequest`, `Notification`, and `SessionEnd`. The hook writes one small state file per tab; ctmux watches the directory and updates badges in real time. No polling, no output scraping.
- **Raw input passthrough.** Keystrokes are forwarded to the active session byte-for-byte (no re-encoding), so paste, ESC, Ctrl+C, and modifier chords behave exactly as they would in a bare terminal. Mouse tracking is always on: clicks on the tab bar switch tabs, and session-area reports are row-shifted and forwarded only at the level the inner session actually requested (clicks / drags / hover).

## Install

```sh
go build -o ctmux ./cmd/ctmux   # Go 1.22+
```

Requires the `claude` CLI on PATH. macOS and Linux (incl. WSL).

## Use

Run `ctmux` — it starts on a splash screen listing the available commands. There, bare keys work: `c` new session, `r` resume, `a` background agents, `q` quit. When a Claude session exits (e.g. `/exit`), its tab closes itself; closing the last tab returns to the splash screen.

Inside a session, commands live behind the **Ctrl+Q** prefix (configurable):

| Keys | Action |
|---|---|
| `^Q c` | New session (pick a directory) |
| `^Q r` | Resume a past session (from `~/.claude/projects/`, with titles) |
| `^Q a` | Background agents — attach to a `claude --bg` / agent-view worker |
| `^Q n` / `^Q p` / `^Q 1–9` | Next / previous / nth tab |
| `Ctrl+Shift+←` / `Ctrl+Shift+→` | Previous / next tab (no prefix needed) |
| mouse click on a tab | Switch to it (works any time; hold Option in iTerm for native text selection) |
| `^Q x` | Close tab (kills the session; attached agents just detach) |
| `^Q d` | Quit ctmux |
| `^Q ^Q` | Send a literal Ctrl+Q to the session |
| `Esc` | Cancel the prefix or any picker |

Attached background agents get their status from the daemon's job state (`~/.claude/jobs/<id>/state.json`) since hooks can't be injected into an already-running worker. Closing an attach tab leaves the worker running.

A bell rings when a background tab needs your input.

## Config

`~/.ctmux/config.toml`:

```toml
prefix = "ctrl+a"   # default: ctrl+q
```

## Notes & limitations

- One row of terminal height is reserved for the tab bar; sessions believe the terminal is one row shorter.
- Scrollback/copy-mode for past output isn't implemented yet — Claude Code's own transcript scrolling (mouse wheel / Ctrl+O) works as usual.
- `ctmux _hook` is the hidden subcommand Claude Code invokes; it exits silently when run outside a ctmux session.
- Upstream quirk worked around in `internal/term/ansipatch.go`: `x/ansi` treats a raw `0x9C` byte inside OSC strings as the 8-bit string terminator, which corrupts UTF-8 titles like Claude's `✳ Claude Code` (worth an upstream issue/PR).
- `hack/m0harness` runs ctmux headlessly inside a PTY and replays its output through a second emulator — useful for regression-testing rendering without a human at a terminal.
