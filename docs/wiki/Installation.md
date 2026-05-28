# Installation

## Requirements

- Linux (uses `/proc` and `SO_PEERCRED`).
- To build from source: Go 1.25+.

## From a released package

Every push to `main` publishes an `rpm`, a `deb`, and a static binary on the [releases page](https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases).

### Debian / Ubuntu (.deb)

```bash
curl -fLO https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases/download/v1.0.5/ttrack_1.0.5_amd64.deb
sudo apt install ./ttrack_1.0.5_amd64.deb
```

### RHEL / Rocky / Fedora (.rpm)

```bash
curl -fLO https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases/download/v1.0.5/ttrack-1.0.5-1.x86_64.rpm
sudo dnf install ./ttrack-1.0.5-1.x86_64.rpm
```

### Static binary (any distro)

```bash
curl -fL -o ttrack https://github.com/rushikeshsakharleofficial/ttrack-tracker/releases/download/v1.0.5/ttrack-1.0.5-linux-amd64
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
```

```
ttrack v1.0.5
```

```bash
sudo systemctl status ttrackd
```

```
● ttrackd.service - ttrack session collector daemon
     Loaded: loaded (/lib/systemd/system/ttrackd.service; enabled)
     Active: active (running) since Wed 2026-05-28 09:00:01 UTC; 1h ago
   Main PID: 1024 (ttrackd)
```

```bash
man ttrack   # opens the man page
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
