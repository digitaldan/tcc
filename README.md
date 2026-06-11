# tcc — Tabbed Claude Code

[![CI](https://github.com/digitaldan/tcc/actions/workflows/ci.yml/badge.svg)](https://github.com/digitaldan/tcc/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/digitaldan/tcc)](https://github.com/digitaldan/tcc/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

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

## Features

- **Tabs for parallel Claude sessions** — spawn sessions in any directory, switch instantly (`Ctrl+Shift+←/→`, number keys, or click a tab)
- **Live status without polling** — tab badges driven by Claude Code's own hook events: busy, idle, waiting for permission/input, errored
- **Resume anything** — a filterable picker over your past sessions (`~/.claude/projects/`) with their titles, projects, and ages
- **Background agents, first-class** — see live *and* completed agents like Claude's agent view; attach to a running worker, or stop it and pull the conversation into the foreground with full history
- **A bell when a background tab needs you**
- **Single static binary** — no tmux, no daemons of its own, no config required

## Install

Download a binary from the [latest release](https://github.com/digitaldan/tcc/releases/latest), or:

```sh
go install github.com/digitaldan/tcc/cmd/tcc@latest
```

Or build from source (Go 1.22+):

```sh
git clone https://github.com/digitaldan/tcc && cd tcc
go build -o tcc ./cmd/tcc
```

Requires the [`claude` CLI](https://claude.com/claude-code) on PATH. Supported platforms: **macOS** and **Linux**.

**Windows**: there is no native Windows binary — use [WSL](https://learn.microsoft.com/windows/wsl/) and run the **Linux** release binary (`tcc_*_linux_amd64.tar.gz` or `linux_arm64`) inside it, alongside Claude Code installed in the same WSL environment. Native Windows (ConPTY) support would need a different PTY backend and is not currently planned.

## Use

Run `tcc` — it starts on a splash screen listing the available commands. There, bare keys work: `c` new session, `r` resume, `a` background agents, `q` quit. When a Claude session exits (e.g. `/exit`), its tab closes itself; closing the last tab returns to the splash screen.

Inside a session, commands live behind the **Ctrl+Q** prefix (configurable):

| Keys | Action |
|---|---|
| `^Q c` | New session — a directory browser opens: `Enter` starts the session in the selected directory, `→` browses into it, `←` goes up (`Enter` on `../` also goes up). The "▶ start session here" row picks the current directory; `★` rows are recent Claude projects; `e` for manual path entry. |
| `^Q r` | Resume a past session (from `~/.claude/projects/`, with titles) |
| `^Q a` | Background agents — live and completed, like Claude's agent view |
| `^Q n` / `^Q p` / `^Q 1–9` | Next / previous / nth tab |
| `Ctrl+Shift+←` / `Ctrl+Shift+→` | Previous / next tab (no prefix needed) |
| mouse click on a tab | Switch to it (works any time; hold Option in iTerm for native text selection) |
| `^Q x` | Close tab (kills the session; attached agents just detach) |
| `^Q d` | Quit tcc |
| `^Q ^Q` | Send a literal Ctrl+Q to the session |
| `Esc` | Cancel the prefix or any picker |

### Background agents

In the agents picker, `Enter` is state-aware: a **live** agent attaches as a live view (the worker's current screen — `claude attach` doesn't repaint past conversation), while a **finished** agent resumes its conversation with full history. Press `s` on a live agent to stop its worker and resume with history instead. The resume picker handles live workers automatically: sessions currently running as background agents are marked with ● and `Enter` stops the worker before resuming (a bare `claude --resume` would refuse).

Attached agents get their status from the daemon's job state (`~/.claude/jobs/<id>/state.json`) since hooks can't be injected into an already-running worker. Closing an attach tab detaches without killing the worker.

## How it works

- **Embedded terminals, no tmux required.** Each `claude` child runs in its own PTY, parsed by an embedded terminal emulator ([charmbracelet/x/vt](https://github.com/charmbracelet/x)). The active tab's screen renders below a one-row tab bar; background sessions keep running and stay renderable for instant switching.
- **Status from Claude Code's own hooks.** tcc passes each session `--settings ~/.tcc/hooks-settings.json` (merged by Claude with your own settings — they're untouched), registering `tcc _hook` for `SessionStart`, `UserPromptSubmit`, `Stop`, `StopFailure`, `PermissionRequest`, `Notification`, and `SessionEnd`. The hook writes one small state file per tab; tcc watches the directory and updates badges in real time.
- **Raw input passthrough.** Keystrokes are forwarded to the active session byte-for-byte (no re-encoding), so paste, ESC, Ctrl+C, and modifier chords behave exactly as they would in a bare terminal. Mouse tracking is always on: clicks on the tab bar switch tabs, and session-area reports are row-shifted and forwarded only at the level the inner session actually requested.

## Config

`~/.tcc/config.toml`:

```toml
prefix = "ctrl+a"   # default: ctrl+q
```

## Notes & limitations

- One row of terminal height is reserved for the tab bar; sessions believe the terminal is one row shorter.
- No native Windows build: tcc relies on Unix pseudo-terminals. On Windows, run the Linux binary under WSL (works fully, including mouse and status badges, in Windows Terminal).
- Scrollback/copy-mode for past output isn't implemented yet — Claude Code's own transcript scrolling works as usual.
- `tcc _hook` is the hidden subcommand Claude Code invokes; it exits silently when run outside a tcc session.
- Claude Code's hook/session/agent file formats are not a stable public API; tcc degrades gracefully when they change, but a Claude Code update may occasionally need a tcc update to match.
- Upstream quirk worked around in `internal/term/ansipatch.go`: `x/ansi` treats a raw `0x9C` byte inside OSC strings as the 8-bit string terminator, which corrupts UTF-8 titles like Claude's `✳ Claude Code`.

## Contributing

Issues and PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md), including a headless PTY harness for testing rendering changes without a human at a terminal.

## License

[MIT](LICENSE)
