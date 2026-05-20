#!/bin/bash
# Distro-aware installer for pmp-rec
# Run as root after `make all`.

set -euo pipefail

BUILDDIR="$(dirname "$0")/../build"
cd "$(dirname "$0")/.."

detect_distro() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        echo "${ID:-unknown}"
    else
        echo "unknown"
    fi
}

DISTRO=$(detect_distro)

echo "==> Detected distro: $DISTRO"

# PAM security module location
case "$DISTRO" in
    rhel|centos|rocky|almalinux|fedora)
        PAM_DIR=/usr/lib64/security
        ;;
    debian|ubuntu)
        PAM_DIR=/usr/lib/x86_64-linux-gnu/security
        ;;
    *)
        PAM_DIR=/usr/lib/security
        ;;
esac

echo "==> Creating pmp-audit group..."
groupadd -r pmp-audit 2>/dev/null || true

echo "==> Installing binaries..."
install -Dm755 build/pmp-rec     /usr/libexec/pmp-rec
install -Dm755 build/pmp-recd    /usr/libexec/pmp-recd
install -Dm755 build/pmp-rec-cli /usr/bin/pmp-rec-cli
# setgid pmp-audit on shim: process inherits gid=pmp-audit → can connect to 0660 socket
chgrp pmp-audit /usr/libexec/pmp-rec
chmod g+s       /usr/libexec/pmp-rec
install -Dm755 "build/pam_record.so" "${PAM_DIR}/pam_record.so"

echo "==> Installing config..."
install -d /etc/pmp-rec
[ -f /etc/pmp-rec/recd.conf     ] || install -m644 config/recd.conf.sample /etc/pmp-rec/recd.conf
[ -f /etc/pmp-rec/shells.allow  ] || install -m644 config/shells.allow.sample /etc/pmp-rec/shells.allow

echo "==> Installing systemd units..."
install -Dm644 scripts/systemd/pmp-recd.service       /usr/lib/systemd/system/pmp-recd.service
install -Dm644 scripts/systemd/pmp-recd.socket         /usr/lib/systemd/system/pmp-recd.socket
install -Dm644 scripts/systemd/pmp-rec-purge.service  /usr/lib/systemd/system/pmp-rec-purge.service
install -Dm644 scripts/systemd/pmp-rec-purge.timer    /usr/lib/systemd/system/pmp-rec-purge.timer
install -Dm644 scripts/tmpfiles.d/pmp-rec.conf        /usr/lib/tmpfiles.d/pmp-rec.conf

echo "==> Installing shell hooks..."
install -Dm644 scripts/profile.d/pmp-rec.sh /etc/profile.d/pmp-rec.sh

echo "==> Installing sudoers env_keep..."
install -Dm440 scripts/sudoers.d/pmp-rec /etc/sudoers.d/pmp-rec
visudo -c -f /etc/sudoers.d/pmp-rec || {
    echo "WARN: sudoers syntax check failed — removing /etc/sudoers.d/pmp-rec"
    rm -f /etc/sudoers.d/pmp-rec
}

echo "==> Creating storage directory..."
install -d -m750 /var/lib/pmp-rec
systemd-tmpfiles --create /usr/lib/tmpfiles.d/pmp-rec.conf 2>/dev/null || true

echo ""
echo "==> Next steps:"
echo "    1. Add pam_record.so to PAM stacks — see scripts/pam.d/*.snippet"
echo "    2. For zsh users: append scripts/zshenv/pmp-rec.zsh to /etc/zshenv"
echo "    3. Enable daemon: systemctl enable --now pmp-recd.socket pmp-recd.service"
echo "    4. Enable purge timer: systemctl enable --now pmp-rec-purge.timer"
echo ""
echo "==> Installation complete."
