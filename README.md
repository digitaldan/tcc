# tcc — Tabbed Claude Code

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
- **Status from Claude Code's own hooks.** tcc passes each session `--settings ~/.tcc/hooks-settings.json` (merged by Claude with your own settings — they're untouched), registering `tcc _hook` for `SessionStart`, `UserPromptSubmit`, `Stop`, `StopFailure`, `PermissionRequest`, `Notification`, and `SessionEnd`. The hook writes one small state file per tab; tcc watches the directory and updates badges in real time. No polling, no output scraping.
- **Raw input passthrough.** Keystrokes are forwarded to the active session byte-for-byte (no re-encoding), so paste, ESC, Ctrl+C, and modifier chords behave exactly as they would in a bare terminal. Mouse tracking is always on: clicks on the tab bar switch tabs, and session-area reports are row-shifted and forwarded only at the level the inner session actually requested (clicks / drags / hover).

## Install

```sh
go build -o tcc ./cmd/tcc   # Go 1.22+
```

Requires the `claude` CLI on PATH. macOS and Linux (incl. WSL).

## Use

Run `tcc` — it starts on a splash screen listing the available commands. There, bare keys work: `c` new session, `r` resume, `a` background agents, `q` quit. When a Claude session exits (e.g. `/exit`), its tab closes itself; closing the last tab returns to the splash screen.

Inside a session, commands live behind the **Ctrl+Q** prefix (configurable):

| Keys | Action |
|---|---|
| `^Q c` | New session — a directory browser opens: arrows/`Enter` navigate (never open), `←` goes up, and `o` starts the session in the directory shown in the header. `★` rows are recent Claude projects. `e` for manual path entry. |
| `^Q r` | Resume a past session (from `~/.claude/projects/`, with titles) |
| `^Q a` | Background agents — live and completed, like Claude's agent view |
| `^Q n` / `^Q p` / `^Q 1–9` | Next / previous / nth tab |
| `Ctrl+Shift+←` / `Ctrl+Shift+→` | Previous / next tab (no prefix needed) |
| mouse click on a tab | Switch to it (works any time; hold Option in iTerm for native text selection) |
| `^Q x` | Close tab (kills the session; attached agents just detach) |
| `^Q d` | Quit tcc |
| `^Q ^Q` | Send a literal Ctrl+Q to the session |
| `Esc` | Cancel the prefix or any picker |

In the background-agents picker, `Enter` is state-aware: a **live** agent attaches as a live view (the worker's current screen — `claude attach` doesn't repaint past conversation), while a **finished** agent resumes its conversation with full history. Press `s` on a live agent to stop its worker and resume with history instead. The resume picker handles live workers automatically: sessions currently running as background agents are marked with ● and `Enter` stops the worker before resuming (a bare `claude --resume` would refuse).

Attached background agents get their status from the daemon's job state (`~/.claude/jobs/<id>/state.json`) since hooks can't be injected into an already-running worker. Closing an attach tab detaches without killing the worker.

A bell rings when a background tab needs your input.

## Config

`~/.tcc/config.toml`:

```toml
prefix = "ctrl+a"   # default: ctrl+q
```

## Notes & limitations

- One row of terminal height is reserved for the tab bar; sessions believe the terminal is one row shorter.
- Scrollback/copy-mode for past output isn't implemented yet — Claude Code's own transcript scrolling works as usual.
- `tcc _hook` is the hidden subcommand Claude Code invokes; it exits silently when run outside a tcc session.
- Upstream quirk worked around in `internal/term/ansipatch.go`: `x/ansi` treats a raw `0x9C` byte inside OSC strings as the 8-bit string terminator, which corrupts UTF-8 titles like Claude's `✳ Claude Code` (worth an upstream issue/PR).
- `hack/m0harness` runs tcc headlessly inside a PTY and replays its output through a second emulator — useful for regression-testing rendering without a human at a terminal.
