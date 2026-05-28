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
