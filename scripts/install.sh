#!/bin/bash
# Distro-aware installer for trackterm-rec
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

echo "==> Installing binaries..."
install -Dm755 build/trackterm-rec     /usr/libexec/trackterm-rec
install -Dm755 build/trackterm-recd    /usr/libexec/trackterm-recd
install -Dm755 build/trackterm-cli /usr/bin/trackterm-cli
install -Dm755 "build/pam_record.so" "${PAM_DIR}/pam_record.so"

echo "==> Installing config..."
install -d /etc/trackterm-rec
[ -f /etc/trackterm-rec/recd.conf     ] || install -m644 config/recd.conf.sample /etc/trackterm-rec/recd.conf
[ -f /etc/trackterm-rec/shells.allow  ] || install -m644 config/shells.allow.sample /etc/trackterm-rec/shells.allow

echo "==> Installing systemd units..."
install -Dm644 scripts/systemd/trackterm-recd.service       /usr/lib/systemd/system/trackterm-recd.service
install -Dm644 scripts/systemd/trackterm-recd.socket         /usr/lib/systemd/system/trackterm-recd.socket
install -Dm644 scripts/systemd/trackterm-rec-purge.service  /usr/lib/systemd/system/trackterm-rec-purge.service
install -Dm644 scripts/systemd/trackterm-rec-purge.timer    /usr/lib/systemd/system/trackterm-rec-purge.timer
install -Dm644 scripts/tmpfiles.d/trackterm-rec.conf        /usr/lib/tmpfiles.d/trackterm-rec.conf

echo "==> Installing shell hooks..."
install -Dm644 scripts/profile.d/trackterm-rec.sh /etc/profile.d/trackterm-rec.sh

echo "==> Installing sudoers env_keep..."
install -Dm440 scripts/sudoers.d/trackterm-rec /etc/sudoers.d/trackterm-rec
visudo -c -f /etc/sudoers.d/trackterm-rec || {
    echo "WARN: sudoers syntax check failed — removing /etc/sudoers.d/trackterm-rec"
    rm -f /etc/sudoers.d/trackterm-rec
}

echo "==> Creating storage directory..."
install -d -m750 /var/lib/trackterm-rec
systemd-tmpfiles --create /usr/lib/tmpfiles.d/trackterm-rec.conf 2>/dev/null || true

echo ""
echo "==> Next steps:"
echo "    1. Add pam_record.so to PAM stacks — see scripts/pam.d/*.snippet"
echo "    2. For zsh users: append scripts/zshenv/trackterm-rec.zsh to /etc/zshenv"
echo "    3. Enable daemon: systemctl enable --now trackterm-recd.socket trackterm-recd.service"
echo "    4. Enable purge timer: systemctl enable --now trackterm-rec-purge.timer"
echo ""
echo "==> Installation complete."
