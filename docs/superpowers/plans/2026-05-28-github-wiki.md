# ttrack GitHub Wiki Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a comprehensive GitHub wiki for ttrack covering installation, player controls, audit mode, Ansible tracking, configuration, troubleshooting, and development — with real sample command outputs on every page.

**Architecture:** The GitHub wiki is a separate Git repository at `https://github.com/rushikeshsakharleofficial/ttrack-tracker.wiki.git`. We clone it locally, write Markdown pages, and push. Each wiki page is a `.md` file in the root of that repo. The sidebar (`_Sidebar.md`) provides navigation.

**Tech Stack:** `gh` CLI, Git, Markdown, GitHub Wiki (already enabled via `gh api PATCH has_wiki=true`).

---

## File Structure

```
ttrack-tracker.wiki/
  Home.md                  — landing page, feature list, navigation
  Installation.md          — deb/rpm/source install, verify
  Quick-Start.md           — first session in 5 min + sample output
  Player-Controls.md       — full keyboard + mouse reference, bar format
  Audit-Mode.md            — ttrackd, ls-user, tree, search, tail, export, prune
  Ansible-Tracking.md      — callback plugin install, list/show with sample output
  Configuration.md         — env vars, filesystem table, encryption key, systemd
  Troubleshooting.md       — common issues + fixes with exact commands
  Development.md           — build, test, packaging, contributing, architecture
  _Sidebar.md              — auto-rendered navigation sidebar
```

---

### Task 1: Clone the wiki repository

**Files:**
- Creates: `/tmp/ttrack-wiki/` (temp working dir, not committed to main repo)

- [ ] **Step 1: Clone the wiki repo**

```bash
cd /tmp
git clone https://github.com/rushikeshsakharleofficial/ttrack-tracker.wiki.git ttrack-wiki 2>&1 || \
  (mkdir -p ttrack-wiki && cd ttrack-wiki && git init && git remote add origin https://github.com/rushikeshsakharleofficial/ttrack-tracker.wiki.git)
cd ttrack-wiki
```

> Note: A newly-enabled wiki has no commits yet. The clone may produce "warning: You appear to have cloned an empty repository." — that is expected. Proceed.

- [ ] **Step 2: Configure git identity in the wiki repo**

```bash
cd /tmp/ttrack-wiki
git config user.email "ramsharath@instantly.ai"
git config user.name "Rushikesh Sakharle"
```

---

### Task 2: Create `_Sidebar.md` (navigation)

**Files:**
- Create: `/tmp/ttrack-wiki/_Sidebar.md`

- [ ] **Step 1: Write the sidebar**

```bash
cat > /tmp/ttrack-wiki/_Sidebar.md << 'EOF'
## ttrack

- [[Home]]
- [[Installation]]
- [[Quick-Start|Quick Start]]
- [[Player-Controls|Player Controls]]
- [[Audit-Mode|Audit Mode]]
- [[Ansible-Tracking|Ansible Tracking]]
- [[Configuration]]
- [[Troubleshooting]]
- [[Development]]
EOF
```

---

### Task 3: Create `Home.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Home.md`

- [ ] **Step 1: Write Home page**

```bash
cat > /tmp/ttrack-wiki/Home.md << 'EOF'
# ttrack — Terminal Session Recorder

`ttrack` records and replays Linux terminal sessions as [asciinema v2](https://docs.asciinema.org/manual/asciicast/v2/) cast files. A companion root daemon (`ttrackd`) collects every user's sessions into a root-only encrypted central store for audit.

Single static Go binary. No runtime dependencies.

## Features

- **Record & replay** any interactive shell session.
- **Full-screen player** with plain-text `[####   ]` progress bar, pause, seek, variable speed, goto-time, scrollback viewer, and mouse click-to-seek.
- **Central audit store** via `ttrackd`: all users' sessions in `/var/lib/ttrack` (`root:root 0700`).
- **Encrypted at rest** — AES-256-GCM; recordings are opaque to `cat`/`strings`/`grep`.
- **Live tail** an in-progress session (`ttrack tail <id>`, root).
- **Audit CLI**: list users, list sessions, replay by id, tree view, full-text search, export to plaintext.
- **Auto-record on login** via optional `profile.d` hook.
- **Non-interactive SSH recording** via optional sshd `ForceCommand` wrapper.
- **Ansible tracking** — callback plugin records playbook runs (play → task → host → status/output) into the central store.
- **Fail-open**: if the daemon is down, recording falls back to a user-local file.
- **Bash tab-completion** for subcommands, flags, sessions, and users.

## Quick navigation

| I want to… | Go to |
|:-----------|:------|
| Install ttrack | [[Installation]] |
| Record my first session | [[Quick-Start]] |
| Learn player keyboard shortcuts | [[Player-Controls]] |
| Browse audit logs as root | [[Audit-Mode]] |
| Track Ansible playbooks | [[Ansible-Tracking]] |
| Configure paths / env vars | [[Configuration]] |
| Fix a problem | [[Troubleshooting]] |
| Build or contribute | [[Development]] |

## Links

- [GitHub repository](https://github.com/rushikeshsakharleofficial/ttrack-tracker)
- [Latest release](https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases/latest)
- [Issue tracker](https://github.com/rushikeshsakharleofficial/ttrack-tracker/issues)
EOF
```

---

### Task 4: Create `Installation.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Installation.md`

- [ ] **Step 1: Write Installation page**

```bash
cat > /tmp/ttrack-wiki/Installation.md << 'EOF'
# Installation

## Requirements

- Linux (uses `/proc` and `SO_PEERCRED`).
- To build from source: Go 1.25+.

## From a released package

Every push to `main` publishes an `rpm`, a `deb`, and a static binary on the [releases page](https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases).

### Debian / Ubuntu (.deb)

```bash
curl -fLO https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases/download/v1.0.2/ttrack_1.0.2_amd64.deb
sudo apt install ./ttrack_1.0.2_amd64.deb
```

### RHEL / Rocky / Fedora (.rpm)

```bash
curl -fLO https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases/download/v1.0.2/ttrack-1.0.2-1.x86_64.rpm
sudo dnf install ./ttrack-1.0.2-1.x86_64.rpm
```

### Static binary (any distro)

```bash
curl -fL -o ttrack https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases/download/v1.0.2/ttrack-1.0.2-linux-amd64
chmod +x ttrack && sudo install -m755 ttrack /usr/bin/ttrack
```

### Always-latest install (auto-detect version)

```bash
VER=$(curl -fsSL https://api.github.com/repos/rushikeshsakharleofficial/ttrack-tracker/releases/latest \
  | grep -oP '"tag_name":\s*"v\K[^"]+')
curl -fLO "https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases/download/v${VER}/ttrack_${VER}_amd64.deb"
sudo apt install "./ttrack_${VER}_amd64.deb"
```

## What the package installs

| Path | Purpose |
|:-----|:--------|
| `/usr/bin/ttrack` | CLI binary |
| `/usr/libexec/ttrackd` | root collector daemon |
| `/usr/libexec/ttrack-ssh-wrap` | optional sshd ForceCommand wrapper |
| `/lib/systemd/system/ttrackd.service` | systemd unit (auto-enabled) |
| `/usr/share/bash-completion/completions/ttrack` | bash tab-completion |
| `/etc/profile.d/ttrack-autorec.sh` | optional auto-record login hook |
| `/usr/share/doc/ttrack/sshd-forcecommand.conf.example` | example sshd config snippet |
| `/usr/share/ttrack/ansible/ttrack.py` | Ansible callback plugin |
| `/usr/share/man/man1/ttrack.1.gz` | man page |

The post-install script creates `/var/lib/ttrack` (`root:root 0700`) and starts `ttrackd`.

## Verify the install

```bash
ttrack --version
# ttrack v1.0.2

sudo systemctl status ttrackd
# ● ttrackd.service - ttrack session collector daemon
#      Loaded: loaded (/lib/systemd/system/ttrackd.service; enabled)
#      Active: active (running) since ...

man ttrack
```

## From source

```bash
git clone https://github.com/rushikeshsakharleofficial/ttrack-tracker.git
cd ttrack-tracker
make build           # produces build/ttrack and build/ttrackd
sudo make install    # installs to /usr/bin, /usr/libexec, systemd, completion, man
```

## Uninstall

```bash
# deb
sudo apt remove ttrack

# rpm
sudo dnf remove ttrack

# manual / source install
sudo rm /usr/bin/ttrack /usr/libexec/ttrackd /usr/libexec/ttrack-ssh-wrap
sudo systemctl disable --now ttrackd
sudo rm /lib/systemd/system/ttrackd.service
```
EOF
```

---

### Task 5: Create `Quick-Start.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Quick-Start.md`

- [ ] **Step 1: Write Quick Start page**

```bash
cat > /tmp/ttrack-wiki/Quick-Start.md << 'EOF'
# Quick Start

## Record your first session

```bash
ttrack rec
```

```
ttrack: recording to /home/alice/.local/share/ttrack/20260528T093012-1428810.cast — type 'exit' or Ctrl-D to stop
alice@host:~$ echo "hello from ttrack"
hello from ttrack
alice@host:~$ uname -sr
Linux 5.14.0-611.55.1.el9_7.x86_64
alice@host:~$ exit

ttrack: session saved to /home/alice/.local/share/ttrack/20260528T093012-1428810.cast
```

## List your recordings

```bash
ttrack ls
```

```
STATUS   FILE                          STARTED              DURATION   COMMAND
SAVED    20260528T093012-1428810.cast  2026-05-28 09:30:12  14s        /bin/bash
```

## Replay it

```bash
ttrack play 20260528T093012-1428810.cast
```

The player opens full-screen. The status bar at the bottom shows:

```
 > 00:00 / 00:14 [          ]   0%  1x   <-/-> seek  pgup scroll  g goto  spc play  q quit
```

Press `space` to pause, `q` to quit. See [[Player-Controls]] for all keys.

## Record a specific command

```bash
ttrack rec /bin/bash -c 'df -h; free -h'
```

```
ttrack: recording to /home/alice/.local/share/ttrack/20260528T093101-1430210.cast — type 'exit' or Ctrl-D to stop
Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1        40G   12G   26G  32% /
...

ttrack: session saved to /home/alice/.local/share/ttrack/20260528T093101-1430210.cast
```

## Enable auto-recording on every login

The package already installs the hook. Enable it:

```bash
# Already installed by the package to /etc/profile.d/ttrack-autorec.sh
# To enable manually:
sudo install -m644 /usr/share/doc/ttrack/ttrack-autorec.sh.example \
                   /etc/profile.d/ttrack-autorec.sh
```

After this, every interactive SSH login is automatically recorded into the central store (requires `ttrackd` running).

## View all users' sessions as root

```bash
sudo ttrack ls-user
```

```
USER                  SESSIONS
root                  3
alice                 12
bob                   5
```

See [[Audit-Mode]] for the full audit command reference.
EOF
```

---

### Task 6: Create `Player-Controls.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Player-Controls.md`

- [ ] **Step 1: Write Player Controls page**

```bash
cat > /tmp/ttrack-wiki/Player-Controls.md << 'EOF'
# Player Controls

`ttrack play` and `ttrack play-user` open a full-screen player when output is a terminal.

## Status bar format

```
 > 01:23 / 05:00 [####      ]  27%  1x   <-/-> seek  pgup scroll  g goto  spc play  q quit
```

- `>` = playing, `||` = paused
- `01:23 / 05:00` = current position / total duration (MM:SS)
- `[####      ]` = progress bar (filled proportional to position)
- `27%` = position as percentage
- `1x` = playback speed (can be `0.5x`, `2x`, `4x`, etc.)

## Keyboard controls

| Key | Effect |
|:----|:-------|
| `space` | Pause / resume |
| `→` or `l` | Seek forward 5 seconds |
| `←` or `h` | Seek backward 5 seconds (re-renders the recording up to that point) |
| `↑` or `+` | Double playback speed (max 64×) |
| `↓` or `-` | Halve playback speed (min 1/64×) |
| `g` | Go to time — the bar changes to `goto:_`. Type `MM:SS` or seconds, Enter to jump, Esc to cancel. |
| `pgup` | Enter scroll view — browse past output; see [Scroll view](#scroll-view) below. |
| `b` | Toggle status bar visibility (full-height playback without the bar) |
| `0` | Restart from the beginning |
| `q` or `Ctrl-C` | Quit |

## Mouse controls

| Action | Effect |
|:-------|:-------|
| Click the `[####   ]` bar | Seek to the clicked position |
| Shift+click anywhere | Selects terminal text (does not seek) |

## Goto-time example

Press `g`, then type `2:30` and Enter:

```
 || 01:23 / 05:00 [####      ]  goto: 2:30_  (enter: jump  esc: cancel)
```

The player jumps to 2m 30s and resumes playback from there.

## Scroll view

Press `pgup` during playback to enter scroll view. The status bar changes to:

```
 [SCROLL] 42 lines   pgup/up/wheel: scroll up   any other key: exit
```

| Key | Effect |
|:----|:-------|
| `pgup` / `↑` / scroll wheel up | Scroll up 3 lines |
| `pgdn` / `↓` / scroll wheel down | Scroll down 3 lines |
| Any other key | Exit scroll view and return to player |

When scrolled back:

```
 [SCROLL] -9/42   pgdn/down/wheel: scroll down   any other key: exit
```

`-9/42` means 9 lines above the latest output, 42 lines total in the buffer.

## Speed reference

| Speed | Key sequence |
|:------|:------------|
| 1/64× | `↓` × 6 from 1× |
| 1/4×  | `↓` × 2 from 1× |
| 1×    | default |
| 2×    | `↑` once |
| 4×    | `↑` twice |
| 16×   | `↑` × 4 |
| 64×   | `↑` × 6 (maximum) |

## Flags

| Flag | Default | Effect |
|:-----|:--------|:-------|
| `--speed N` | `1.0` | Playback speed multiplier (same as pressing `↑`/`↓`) |
| `--idle N` | `0` | Cap idle gaps to N seconds. `0` = exact original timing. Set `>0` to compress long pauses. Ignored in the interactive player (use seek instead). |
EOF
```

---

### Task 7: Create `Audit-Mode.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Audit-Mode.md`

- [ ] **Step 1: Write Audit Mode page**

```bash
cat > /tmp/ttrack-wiki/Audit-Mode.md << 'EOF'
# Audit Mode

`ttrackd` is the root daemon that collects all users' sessions into a root-only encrypted central store at `/var/lib/ttrack`. Audit commands require root.

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

`TYPE` is `interactive` (login shell) or `non-interactive` (`ssh host "cmd"` via ForceCommand wrapper). `ACTIVE` sessions show elapsed time with `+`.

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
alice@host:~$ ls -la /etc/
total 1068
drwxr-xr-x  92 root root  4096 May 28 10:10 .
...
```

Output streams in real time as the user types. `Ctrl-C` stops the tail (does not affect the session).

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

Flags: `--from`/`--to` (date or `YYYY-MM-DD HH:MM`), `--user`, `-i` (case-insensitive), `--all` (list all sessions regardless of match).

## Export a session to plaintext

Central recordings are encrypted at rest. Export to a standard asciinema `.cast` file:

```bash
sudo ttrack export -o session.cast 20260528T093101-1430210.cast
```

```
exported plaintext cast to session.cast
```

Then play with `asciinema play session.cast` or share.

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

The AES-256-GCM key lives at `/var/lib/ttrack/.ttrack.key` (`root:root 0600`, `chattr +i`). It cannot be removed or modified without `chattr -i`. **Back it up** — losing it makes every encrypted recording permanently unreadable.
EOF
```

---

### Task 8: Create `Ansible-Tracking.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Ansible-Tracking.md`

- [ ] **Step 1: Write Ansible Tracking page**

```bash
cat > /tmp/ttrack-wiki/Ansible-Tracking.md << 'EOF'
# Ansible Tracking

ttrack can record Ansible playbook runs on the **controller** host (the machine running `ansible-playbook`). Each run is stored encrypted in the central store alongside terminal sessions, browsable with `ttrack ansible list` / `ttrack ansible show`.

## How it works

```
ansible-playbook (controller)
  └─ ttrack callback plugin (/usr/share/ttrack/ansible/ttrack.py)
       └─ pipes JSON-lines events to: ttrack ansible-ingest (subprocess)
            └─ connects to ttrackd over /run/ttrackd.sock
                 └─ ttrackd encrypts + writes /var/lib/ttrack/<user>/ansible/<runid>.ajsonl
```

## Enable the callback plugin

**Via environment variables (per-run):**

```bash
export ANSIBLE_CALLBACK_PLUGINS=/usr/share/ttrack/ansible
export ANSIBLE_CALLBACKS_ENABLED=ttrack
ansible-playbook site.yml
```

**Via `ansible.cfg` (persistent):**

```ini
[defaults]
callback_plugins  = /usr/share/ttrack/ansible
callbacks_enabled = ttrack
```

The plugin is installed at `/usr/share/ttrack/ansible/ttrack.py` by the deb/rpm packages.

## Browse runs

```bash
sudo ttrack ansible list
```

```
RUN                           PLAYBOOK             CONTROLLER        OK   CHG  FAIL  STARTED              HOSTS
20260528T140300-12345         deploy.yml           ctrl.example.com   8     3     1  2026-05-28 14:03:00  web1,web2
20260527T093000-11001         baseline.yml         ctrl.example.com  24     0     0  2026-05-27 09:30:00  web1,web2,db1
```

```bash
sudo ttrack ansible show 20260528T140300-12345
```

```
Playbook  : deploy.yml
Run ID    : 20260528T140300-12345
Controller: ctrl.example.com
User      : alice
Started   : 2026-05-28 14:03:00
Duration  : 43s

PLAY [Install web server]
  ✓ web1   install nginx           ansible.builtin.dnf      @14:03:01
  ~ web1   configure nginx         ansible.builtin.template @14:03:04  (changed)
  ✓ web2   install nginx           ansible.builtin.dnf      @14:03:01
  ✗ web2   fail intentionally      ansible.builtin.command  @14:03:09
      rc: 1
      stderr: /usr/bin/false: intentional failure

PLAY RECAP
  web1   ok=8    changed=3   failed=0   unreachable=0   skipped=1
  web2   ok=6    changed=1   failed=1   unreachable=0   skipped=1
```

Status icons: `✓` = ok, `~` = changed, `✗` = failed, `!` = unreachable, `-` = skipped.

## Fail-open

If `ttrackd` is unreachable when the playbook runs, the run is saved locally to:

```
~/.local/share/ttrack/ansible/<runid>.ajsonl
```

The playbook is **never aborted** due to ttrack failures. The local file is ingested into the central store when `ttrackd` next starts.

## `no_log` tasks

Tasks with `no_log: true` are recorded but their `stdout` and `stderr` are replaced with `<censored>`:

```bash
sudo ttrack ansible show 20260528T140300-12345
```

```
  ✓ web1   rotate db password    ansible.builtin.command  @14:03:12
      stdout: <censored>
      stderr: <censored>
```

The task name, module, host, status, and rc are always recorded.

## Limitation

Only **controllers** with `ttrack` installed produce Ansible records. Managed hosts still receive raw Ansible SSH execs; if the sshd `ForceCommand` wrapper is configured on them, those execs are captured as terminal sessions (with no task name or status — just the raw `AnsiballZ_<module>.py` invocation).
EOF
```

---

### Task 9: Create `Configuration.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Configuration.md`

- [ ] **Step 1: Write Configuration page**

```bash
cat > /tmp/ttrack-wiki/Configuration.md << 'EOF'
# Configuration

`ttrack` and `ttrackd` need no config file. Behavior is controlled by environment variables, filesystem locations, and the systemd unit.

## Environment variables

| Variable | Default | Used by | Description |
|:---------|:--------|:--------|:------------|
| `TTRACK_DIR` | `~/.local/share/ttrack` | `ttrack` | User-local recordings directory (fail-open fallback + local `ls`/`play`). |
| `TTRACK_CENTRAL_DIR` | `/var/lib/ttrack` | `ttrack`, `ttrackd` | Central root-only store. |
| `TTRACKD_SOCK` | `/run/ttrackd.sock` | `ttrack`, `ttrackd` | Daemon unix socket path. |
| `TTRACK_QUIET` | unset | `ttrack rec` | Any non-empty value suppresses the recording banner and saved-path message. |
| `SHELL` | `/bin/bash` | `ttrack rec` | Shell launched when no command is given. |

## Filesystem layout

| Path | Owner / mode | Purpose |
|:-----|:-------------|:--------|
| `/usr/bin/ttrack` | `root 0755` | CLI binary |
| `/usr/libexec/ttrackd` | `root 0755` | daemon |
| `/var/lib/ttrack/` | `root:root 0700` | central store (no non-root access) |
| `/var/lib/ttrack/<user>/<id>.cast` | `root:root 0600` | encrypted recording |
| `/var/lib/ttrack/.ttrack.key` | `root:root 0600`, `chattr +i` | per-server AES-256-GCM key (immutable) |
| `/run/ttrackd.sock` | `root 0666` | recorder connect socket |
| `/etc/profile.d/ttrack-autorec.sh` | `root 0644` | optional auto-record login hook |
| `~/.local/share/ttrack/` | the user | local fail-open recordings |

## Daemon systemd unit

```bash
sudo systemctl status ttrackd
sudo systemctl restart ttrackd
sudo journalctl -u ttrackd --no-pager --since '10 min ago'
```

Override the store or socket path with a drop-in:

```bash
sudo systemctl edit ttrackd
```

Add:

```ini
[Service]
Environment=TTRACK_CENTRAL_DIR=/srv/ttrack
Environment=TTRACKD_SOCK=/run/ttrackd.sock
```

## Encryption key

The daemon creates a unique random key on first start: `/var/lib/ttrack/.ttrack.key` (`root:root 0600`, `chattr +i`).

**Back it up.** Losing it makes every encrypted recording permanently unreadable. The daemon refuses to start if the key is missing while encrypted recordings exist.

To rotate the key:

```bash
# 1. export all existing recordings first
for id in $(sudo ttrack ls-user --all --ids); do
  sudo ttrack export -o "${id}.cast" "$id"
done

# 2. remove the immutable flag and the key
sudo chattr -i /var/lib/ttrack/.ttrack.key
sudo rm /var/lib/ttrack/.ttrack.key

# 3. restart the daemon (generates a fresh key)
sudo systemctl restart ttrackd
```

New recordings use the new key. Old exported `.cast` files are plaintext asciinema — re-ingest if needed.

## Bash completion

Installed by the package to `/usr/share/bash-completion/completions/ttrack`. Enable manually:

```bash
ttrack completion bash | sudo tee /usr/share/bash-completion/completions/ttrack
```

Completes subcommands, flags, local sessions (for `play`), and — as root — users and central session ids.

## Auto-record on login

The package installs `/etc/profile.d/ttrack-autorec.sh`. It triggers only for interactive shells with a real TTY, skips nested shells (`sudo su -`, subshells) by detecting a `ttrack` process in the ancestry, and is fail-open (if the recorder cannot start, a normal shell continues). Remove the file to disable.

## Non-interactive SSH recording

```bash
sudo cp /usr/share/doc/ttrack/sshd-forcecommand.conf.example \
        /etc/ssh/sshd_config.d/zz-ttrack.conf
sudo sshd -t && sudo systemctl reload ssh
```

- `scp`/`sftp`/`rsync`/git transfers pass through untouched.
- Interactive logins keep recording via the profile.d hook (no double-wrap).
- Fail-open: if anything is off, the command runs normally.

Exclude an account:

```
Match User *,!deploy-bot
    ForceCommand /usr/libexec/ttrack-ssh-wrap
```
EOF
```

---

### Task 10: Create `Troubleshooting.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Troubleshooting.md`

- [ ] **Step 1: Write Troubleshooting page**

```bash
cat > /tmp/ttrack-wiki/Troubleshooting.md << 'EOF'
# Troubleshooting

## ttrack rec hangs / does not produce output

Check the daemon:

```bash
sudo systemctl status ttrackd
sudo journalctl -u ttrackd --since '5 min ago' --no-pager
```

If the daemon is stopped, `ttrack rec` still works (fail-open: saves to `~/.local/share/ttrack`). Start `ttrackd` — those files are ingested on next daemon start:

```bash
sudo systemctl start ttrackd
```

## `man ttrack` shows an old version

A manual install may have left a stale man page at `/usr/local/man/man1/ttrack.1` that shadows the package-installed one:

```bash
man -w ttrack
# /usr/local/man/man1/ttrack.1   ← stale

sudo rm -f /usr/local/man/man1/ttrack.1
man ttrack
# now shows the current version
```

## Player: scroll view shows garbled or concatenated lines

The scrollback viewer parses terminal output heuristically. Cursor-movement sequences (`\x1b[H`, `\x1b[A`, `\x1b[F`, `\x1bM`) are treated as implicit line breaks so tool output that repaints its line (dpkg, apt, bash prompts) appears correctly. Full-screen TUIs (vim, htop) draw to arbitrary cells — they may look approximate in scroll view, but the main player renders them exactly.

## Player: colors bleed into empty cells in scroll view

The scroll renderer resets SGR attributes before and after each line (`\x1b[0m`) and erases trailing cells with the default background (`\x1b[K`). If you see color bleed, check your version:

```bash
ttrack --version
# should be v1.0.2 or later
```

Upgrade if on an older build.

## Player: `ttrack play` shows `^[[...` garbage or ignores keys

The player requires a real TTY. If output is piped or redirected, it runs straight through without the interactive UI:

```bash
ttrack play file.cast | cat    # no player — plain through-mode
ttrack play file.cast          # player opens (stdout is a TTY)
```

## ttrack rec records a session but central store shows nothing

The daemon must be running **when the session starts**. Sessions started while the daemon is down are saved locally (`~/.local/share/ttrack`). They are ingested into the central store when the daemon next starts. Check:

```bash
ls ~/.local/share/ttrack/
# files here = failed to reach daemon at session start

sudo systemctl start ttrackd
# on next start, ttrackd ingests these files
```

## ttrack export fails: "key file missing"

The encryption key at `/var/lib/ttrack/.ttrack.key` was removed or the store is from a different host. Without the key, encrypted recordings cannot be decrypted. If you have a backup:

```bash
sudo cp /backup/ttrack.key /var/lib/ttrack/.ttrack.key
sudo chattr +i /var/lib/ttrack/.ttrack.key
sudo systemctl restart ttrackd
```

If the key is permanently lost, the recordings are unrecoverable.

## ttrackd fails to start: "key missing, encrypted recordings exist"

The daemon detected encrypted `.cast` files but no key. Restore the key from backup (see above), or remove the encrypted files and let the daemon generate a fresh key:

```bash
# WARNING: this permanently destroys all existing encrypted recordings
sudo rm /var/lib/ttrack/.ttrack.key
sudo systemctl start ttrackd   # generates a new key
```

## ttrack ansible list shows nothing after running a playbook

1. Confirm the callback plugin is enabled:

```bash
ansible-config dump | grep -E 'CALLBACKS_ENABLED|CALLBACK_PLUGINS'
```

2. Check if the run was saved locally (daemon was down):

```bash
ls ~/.local/share/ttrack/ansible/
```

3. If files exist there, start ttrackd to ingest them:

```bash
sudo systemctl start ttrackd
```

## SSH recording wraps commands in a PTY (cooked output)

Commands recorded via the `ForceCommand` wrapper run under a PTY. This means:
- Output is line-cooked (CR+LF added).
- `isatty()` returns true inside the command.
- A few tools behave differently (e.g. `git` enables colors, `curl` shows a progress bar).

This is expected behavior. The playback is accurate — the PTY behavior is captured.
EOF
```

---

### Task 11: Create `Development.md`

**Files:**
- Create: `/tmp/ttrack-wiki/Development.md`

- [ ] **Step 1: Write Development page**

```bash
cat > /tmp/ttrack-wiki/Development.md << 'EOF'
# Development

## Build

```bash
git clone https://github.com/rushikeshsakharleofficial/ttrack-tracker.git
cd ttrack-tracker
make build
```

```
go build -o build/ttrack ./cmd/ttrack
go build -o build/ttrackd ./cmd/ttrackd
```

Binaries are in `build/`. Static, CGO disabled, no runtime dependencies.

## Test

```bash
make test
# equivalent to:
go test ./...

# with race detector (recommended before submitting a PR):
go test -race ./...
```

```
ok  	ttrack/internal/cast      0.003s
ok  	ttrack/internal/crypto    0.021s
ok  	ttrack/internal/play      0.008s
ok  	ttrack/internal/store     0.005s
ok  	ttrack/internal/ansible   0.006s
```

## Packages

```bash
# install nfpm first (needed for deb/rpm)
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

make deb          # → release/ttrack_<ver>_amd64.deb
make rpm          # → release/ttrack-<ver>-1.x86_64.rpm
make packages     # both

make VERSION=1.2.3 packages   # explicit version
```

## Project layout

```
cmd/ttrack/        CLI entry point (subcommand dispatch)
cmd/ttrackd/       daemon entry point
internal/cast/     asciinema v2 cast read/write
internal/crypto/   at-rest AES-256-GCM encryption (+ tests)
internal/record/   PTY capture for `ttrack rec`
internal/play/     replay player (snapshot-bounded, scrollback viewer)
internal/store/    storage paths + transparent decrypt
internal/audit/    root-only audit commands (ls-user, tree, search, export, prune)
internal/daemon/   ttrackd socket server, live tail fan-out, ingest, key mgmt
internal/ansible/  Ansible run model + list/show commands
internal/complete/ bash completion
scripts/ansible/   Ansible callback plugin (Python)
scripts/profile.d/ auto-record login hook
man/               ttrack.1 man page source
nfpm.yaml          package metadata (deb/rpm)
```

## CI/CD

Every push to `main` runs the [pipeline](.github/workflows/pipeline.yml):

1. **Build** — `make build`
2. **Test** — `go test -race ./...`
3. **Lint** — `golangci-lint run`
4. **SonarQube scan** — static analysis
5. **Package** — `make packages` (deb + rpm + static binary)
6. **Auto Release** — bumps patch version from latest tag, creates GitHub Release with artifacts + `SHA256SUMS`
7. **Deploy** — deploys deb to the test jump server (`89.167.44.42`) via `dpkg -i` (skips if same version already installed)

## Wire protocol (ttrackd socket)

```
Client sends:  REC\n            → stream a new session cast
               TAIL <id>\n      → receive a live-tail stream
               ANSIBLE <id>\n   → stream a new ansible run JSON-lines
```

Auth: `SO_PEERCRED` (Linux peer credential). Only the UID that owns the connection can write under that user's store directory. Root can read all.

## Encryption

`internal/crypto` implements AES-256-GCM streaming encryption. The key is 32 random bytes stored at `/var/lib/ttrack/.ttrack.key`. Files start with the magic prefix `TTEC1` followed by a random nonce and ciphertext chunks. `ttrack export` and the audit playback commands decrypt transparently.

## Contributing

1. Fork the repository and create a feature branch.
2. Run `make fmt`, `make vet`, `make test` — CI enforces all three.
3. Open a pull request; describe what and why (the diff shows how).

See [CONTRIBUTING.md](https://github.com/rushikeshsakharleofficial/ttrack-tracker/blob/main/CONTRIBUTING.md) for the full guide.
EOF
```

---

### Task 12: Commit and push all wiki pages

**Files:**
- All `*.md` in `/tmp/ttrack-wiki/`

- [ ] **Step 1: Stage all pages**

```bash
cd /tmp/ttrack-wiki
git add Home.md Installation.md Quick-Start.md Player-Controls.md \
        Audit-Mode.md Ansible-Tracking.md Configuration.md \
        Troubleshooting.md Development.md _Sidebar.md
```

- [ ] **Step 2: Commit**

```bash
git commit -m "docs: initial wiki — all pages with sample outputs"
```

- [ ] **Step 3: Push**

```bash
git push origin master 2>&1 || git push origin HEAD:master
```

Expected output:

```
Enumerating objects: 11, done.
Counting objects: 100% (11/11), done.
...
To https://github.com/rushikeshsakharleofficial/ttrack-tracker.wiki.git
 * [new branch]      master -> master
```

- [ ] **Step 4: Verify in browser**

```bash
gh browse --wiki
# opens https://github.com/rushikeshsakharleofficial/ttrack-tracker/wiki
```

---

## Self-Review

### Spec coverage

- ✅ Home/navigation page
- ✅ Installation (deb/rpm/source/static, verify)
- ✅ Quick Start with real sample output
- ✅ Player controls (all keys, bar format, scroll view, speed table)
- ✅ Audit mode (all commands with sample output)
- ✅ Ansible tracking (enable, list, show, fail-open, no_log)
- ✅ Configuration (env vars, filesystem, encryption key, systemd, completion, ForceCommand)
- ✅ Troubleshooting (daemon, man page, scrollback, colors, keys, export, ansible)
- ✅ Development (build, test, packages, layout, CI, protocol, crypto, contributing)
- ✅ Sample command outputs on every page
- ✅ Sidebar navigation

### Placeholder scan — none found.

### Type consistency — all cross-references use `[[PageName]]` wiki links; all commands consistent with codebase.
