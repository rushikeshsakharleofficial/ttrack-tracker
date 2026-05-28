# Audit Mode

`ttrackd` is the root daemon that collects all users' sessions into a root-only encrypted central store at `/var/lib/ttrack`. All audit commands require root.

## Daemon status

```bash
sudo systemctl status ttrackd
```

```
● ttrackd.service - ttrack session collector daemon
     Loaded: loaded (/lib/systemd/system/ttrackd.service; enabled; preset: enabled)
     Active: active (running) since Wed 2026-05-28 09:00:01 UTC; 1h 23min ago
   Main PID: 1024 (ttrackd)
```

```bash
sudo journalctl -u ttrackd --since '10 min ago' --no-pager
```

```
May 28 09:00:01 host ttrackd[1024]: ttrackd starting, central store: /var/lib/ttrack
May 28 09:00:01 host ttrackd[1024]: WARNING: back up /var/lib/ttrack/.ttrack.key — losing it makes all encrypted recordings permanently unreadable
May 28 09:00:01 host ttrackd[1024]: listening on /run/ttrackd.sock
```

## List users

```bash
sudo ttrack ls-user
```

```
USER                  SESSIONS
root                  3
alice                 12
bob                   5
```

## List a user's sessions

```bash
sudo ttrack ls-user alice
```

```
STATUS   TYPE             SESSION                       STARTED              DURATION   COMMAND
SAVED    interactive      20260528T093012-1428810.cast  2026-05-28 09:30:12  14s        /bin/bash
SAVED    non-interactive  20260528T093101-1430210.cast  2026-05-28 09:31:01  3s         /bin/bash -c df -h; free -h
ACTIVE   interactive      20260528T101423-1498011.cast  2026-05-28 10:14:23  23m+       /bin/bash
```

`TYPE` is `interactive` (login shell) or `non-interactive` (`ssh host "cmd"` via ForceCommand). `ACTIVE` sessions show elapsed time with a trailing `+`.

## Tree view

```bash
sudo ttrack tree
```

```
/var/lib/ttrack
├─ root
│  └─ 20260528T090001-1024000.cast  [SAVED interactive]  2026-05-28 09:00:01  5m12s  /bin/bash
├─ alice
│  ├─ 20260528T093012-1428810.cast  [SAVED interactive]  2026-05-28 09:30:12  14s    /bin/bash
│  ├─ 20260528T093101-1430210.cast  [SAVED non-interactive]  2026-05-28 09:31:01  3s  /bin/bash -c df -h; free -h
│  └─ 20260528T101423-1498011.cast  [ACTIVE interactive]  2026-05-28 10:14:23  23m+  /bin/bash
└─ bob
   └─ 20260528T095500-1460000.cast  [SAVED interactive]  2026-05-28 09:55:00  8m44s  /bin/bash
```

## Replay a session (any user)

```bash
sudo ttrack play-user 20260528T093101-1430210.cast
```

Opens the full-screen player. See [[Player-Controls]].

## Live-tail an active session

```bash
sudo ttrack tail 20260528T101423-1498011.cast
```

```
ttrack: tailing 20260528T101423-1498011.cast (alice) — Ctrl-C to stop
alice@host:~$ ls /etc/
adjtime  bash.bashrc  cron.d  cron.daily  default  environment ...
alice@host:~$ systemctl status nginx
● nginx.service - A high performance web server
     Active: active (running) since ...
```

Output streams in real time as the user types. `Ctrl-C` stops the tail (does not affect the recorded session).

## Search across recordings

```bash
sudo ttrack search nginx
```

```
user=alice  when=2026-05-28 09:45:18  session=20260528T094518-1445600
    cmd: /bin/bash -c systemctl restart nginx; echo done
    > Failed to restart nginx.service: Unit nginx.service not found.
```

```bash
sudo ttrack search --from "2026-05-28 09:00" --to "2026-05-28 12:00" --user alice -i DEPLOY
```

```
user=alice  when=2026-05-28 09:45:18  session=20260528T094518-1445600
    cmd: /bin/bash -c echo starting deploy; systemctl restart nginx; echo deploy done
    > starting deploy
    > deploy done
```

Search flags: `--from`/`--to` (date or `YYYY-MM-DD HH:MM`), `--user`, `-i` (case-insensitive), `--all` (list every session regardless of match).

## Export a session to plaintext

Central recordings are encrypted at rest. Export to a standard asciinema `.cast` file:

```bash
sudo ttrack export -o session.cast 20260528T093101-1430210.cast
```

```
exported plaintext cast to session.cast
```

Then replay with `asciinema play session.cast` or share via [asciinema.org](https://asciinema.org).

## Prune old recordings

```bash
sudo ttrack prune
```

```
Storage overview:
  alice     12 sessions    47.3 MB
  bob        5 sessions    18.1 MB
  root       3 sessions     9.2 MB
  Total     20 sessions    74.6 MB

Prune which user? (username / all / cancel): alice
Delete: all / days N / range FROM TO: days 30
Deleting alice sessions older than 30 days... 9 sessions (38.2 MB) — confirm? [y/N]: y
Deleted 9 sessions.
```

`--yes` skips the confirmation prompt (for scripted use). Never deletes active (in-progress) sessions.

## Encryption at rest

```bash
sudo strings /var/lib/ttrack/alice/20260528T093012-1428810.cast | head -2
```

```
TTEC1
(binary ciphertext — unreadable without the key)
```

The AES-256-GCM key lives at `/var/lib/ttrack/.ttrack.key` (`root:root 0600`, `chattr +i`). It cannot be removed or modified without first running `chattr -i`. **Back it up** — losing it makes every encrypted recording permanently unreadable. See [[Configuration#encryption-key]] for backup and rotation.
