# ttrack

A minimal Linux terminal session recorder and replayer. `ttrack rec` forks your shell under a PTY, captures all output to an [asciinema v2](https://docs.asciinema.org/manual/asciicast/v2/) cast file, and `ttrack play` replays it later. Think `script(1)` or `asciinema`, packaged as a single self-contained Go binary with no external runtime dependencies.

## Build

Requires Go 1.25.

```bash
CGO_ENABLED=0 go build -trimpath -o bin/ttrack ./cmd/ttrack
```

Or with make:

```bash
make build
```

Dependencies: [`github.com/creack/pty`](https://github.com/creack/pty), [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term).

## Quick start

```bash
# Record a session (auto-named file in the store dir)
ttrack rec

# Exit the shell when done, then list recordings
ttrack ls

# Replay a recording
ttrack play ~/.local/share/ttrack/20260526T120000-12345.cast
```

## Commands

### `ttrack rec [-o file] [cmd...]`

Record a session. Forks `cmd` (or `$SHELL`, falling back to `/bin/bash`) under a PTY and writes all output to a cast file.

| Flag | Default | Description |
|------|---------|-------------|
| `-o file` | auto-named in store dir | Output file path |

If no `cmd` is given, the user's `$SHELL` is launched interactively.

### `ttrack play [--speed N] [--idle N] <file>`

Replay a recording.

| Flag | Default | Description |
|------|---------|-------------|
| `--speed N` | `1.0` | Playback speed multiplier |
| `--idle N` | `2.0` | Cap idle gaps to N seconds (`0` disables the cap) |

### `ttrack ls`

List recordings in the store dir. Columns: `FILE`, `STARTED`, `COMMAND`.

## Storage and file format

The store directory is `$TTRACK_DIR` if set, otherwise `~/.local/share/ttrack`.

Auto-named recordings use the pattern `<YYYYMMDDThhmmss>-<pid>.cast`.

Files are written in **asciinema v2 cast format** (JSON-lines):

- First line: header object — `version`, `width`, `height`, `timestamp`, `command`, `env`.
- Subsequent lines: event arrays — `[time_seconds, "o", "output-data"]`.

Because the format is standard asciinema v2, recordings are also playable with `asciinema play <file>` if asciinema is installed.

## Limitations

- Records PTY output only — keystrokes are not captured.
- Single local session scope: no daemon, no system-wide auto-recording, no PAM integration, and no remote storage.
- Only the shell launched under `ttrack rec` is recorded, not pre-existing sessions.

## License

[LICENSE](LICENSE)
