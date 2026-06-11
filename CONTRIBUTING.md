# Contributing to tcc

Thanks for your interest! Bug reports, feature requests, and PRs are all welcome.

## Development

```sh
go build -o tcc ./cmd/tcc
go test ./...
go vet ./...
```

Requires Go 1.22+ and the `claude` CLI on PATH for manual testing. CI runs build, vet, `go test -race`, gofmt, and a `go mod tidy` check on Linux and macOS — please run those locally before opening a PR.

## Testing TUI changes headlessly

`hack/m0harness` runs tcc inside a PTY, captures everything it draws, replays the bytes through a second terminal emulator, and prints the final screen — no human at a terminal required:

```sh
go build -o /tmp/m0harness ./hack/m0harness
/tmp/m0harness -bin ./tcc -dir /tmp/some-project -wait 5s SNAP CTRLQ 400ms c 1s SNAP
```

Script tokens: durations sleep, `ENTER`/`ESC`/`CTRLQ`/`CTRLU` send keys, `NUDGE` forces a resize repaint, `SNAP` prints the screen, anything else is typed verbatim. One caveat: consecutive fast writes coalesce into a single key event, so drive list filtering with delays between keystrokes.

## Project layout

| Package | Responsibility |
|---|---|
| `internal/term` | embedded vt emulator wrapper, raw stdin router (prefix key, mouse rewriting) |
| `internal/session` | claude child processes: PTY lifecycle, spawn/resume/attach command lines |
| `internal/status` | hook events → tab state machine, state-file watcher |
| `internal/hookcmd` | the `tcc _hook` subcommand Claude Code invokes |
| `internal/claude` | Claude Code's files & CLI: transcripts, hooks settings, background agents |
| `internal/app` | bubbletea UI: tab bar, pickers, splash, key handling |

## Releases

Maintainers cut releases by pushing a tag: `git tag v0.x.y && git push origin v0.x.y`. GoReleaser builds darwin/linux × amd64/arm64 binaries and publishes them on the GitHub release.
