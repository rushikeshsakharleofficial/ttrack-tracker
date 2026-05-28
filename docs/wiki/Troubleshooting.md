# Troubleshooting

## `ttrack rec` hangs / nothing records

Check the daemon:

```bash
sudo systemctl status ttrackd
sudo journalctl -u ttrackd --since '5 min ago' --no-pager
```

If stopped, `ttrack rec` still works (fail-open: saves to `~/.local/share/ttrack`). Start the daemon — those files are ingested on its next startup:

```bash
sudo systemctl start ttrackd
ls ~/.local/share/ttrack/    # files here = recorded while daemon was down
```

## `man ttrack` shows an old version

A manual install may have left a stale page at `/usr/local/man/man1/ttrack.1` that shadows the package-installed one:

```bash
man -w ttrack
# /usr/local/man/man1/ttrack.1   ← stale

sudo rm -f /usr/local/man/man1/ttrack.1
man ttrack   # now shows current version
```

## Scroll view: garbled or concatenated lines

The scrollback viewer parses terminal output heuristically. Cursor-movement sequences (`\x1b[H`, `\x1b[A`, `\x1b[F`, `\x1bM`) are treated as implicit line breaks so tools that repaint their line (dpkg, apt, bash prompts) appear as separate lines. Full-screen TUIs (vim, htop) draw to arbitrary cells — they look approximate in scroll view, but the main player renders them exactly.

## Scroll view: colors bleed into empty cells

The scroll renderer resets SGR attributes before and after each line, and erases trailing cells with the default background. If you see color bleed, check your version:

```bash
ttrack --version
# should be v1.0.2 or later
```

Upgrade if on an older build.

## Player shows `^[[...` garbage or ignores keys

The player requires a real TTY. If output is piped or redirected, it runs straight through:

```bash
ttrack play file.cast | cat    # no player — plain through-mode
ttrack play file.cast          # player opens (stdout is a TTY)
```

## Central store shows nothing after `ttrack rec`

The daemon must be running **when the session starts**. Sessions started while the daemon is down are saved locally. They are ingested when the daemon next starts:

```bash
ls ~/.local/share/ttrack/     # check for locally saved files
sudo systemctl start ttrackd  # ingests them on startup
sudo ttrack ls-user alice     # should now appear
```

## `ttrack export` fails: "key file missing"

The encryption key at `/var/lib/ttrack/.ttrack.key` was removed or the recording is from a different host. Without the key, encrypted recordings cannot be decrypted. Restore from backup:

```bash
sudo cp /backup/ttrack.key /var/lib/ttrack/.ttrack.key
sudo chmod 0600 /var/lib/ttrack/.ttrack.key
sudo chattr +i /var/lib/ttrack/.ttrack.key
sudo systemctl restart ttrackd
```

If the key is permanently lost, the encrypted recordings are unrecoverable.

## `ttrackd` fails to start: "key missing, encrypted recordings exist"

The daemon found encrypted `.cast` files but no key. Restore from backup (see above), or permanently discard the encrypted recordings and let the daemon generate a fresh key:

```bash
# WARNING: destroys all existing encrypted recordings
sudo rm /var/lib/ttrack/.ttrack.key
sudo systemctl start ttrackd   # generates a new key
```

## `ttrack ansible list` shows nothing after a playbook run

1. Confirm the callback plugin is enabled:

```bash
ansible-config dump | grep -E 'CALLBACKS_ENABLED|CALLBACK_PLUGINS'
# CALLBACKS_ENABLED(env: ANSIBLE_CALLBACKS_ENABLED) = ttrack
# DEFAULT_CALLBACK_PLUGIN_PATH(...) = /usr/share/ttrack/ansible:...
```

2. Check if the run was saved locally (daemon was down):

```bash
ls ~/.local/share/ttrack/ansible/
```

3. If files exist there, start `ttrackd` to ingest them:

```bash
sudo systemctl start ttrackd
sudo ttrack ansible list   # should now appear
```

## SSH: recorded commands run under a PTY (cooked output)

Commands recorded via the `ForceCommand` wrapper run under a PTY. This means:
- Output is line-cooked (CR+LF added).
- `isatty()` returns `true` inside the command.
- Some tools change behavior (git enables colors, curl shows a progress bar).

This is expected. The playback is accurate — the PTY behavior is part of the recording.
