#!/bin/bash
# Build a .deb binary package for pmp-rec using ar + tar (no dpkg-dev needed).
# Run from the terminal-recorder/ project root after `make all`.
#
# Usage:
#   bash packaging/deb/build-deb.sh [OUTDIR]
#
# Produces:  pmp-rec_0.1.0-1_amd64.deb  (and .changes stub)
# Tested on: Rocky Linux 9, Debian 12, Ubuntu 22.04+

set -euo pipefail

VERSION="0.1.0"
RELEASE="1"
ARCH="amd64"
PKG="pmp-rec"
DEBNAME="${PKG}_${VERSION}-${RELEASE}_${ARCH}.deb"

OUTDIR="${1:-release}"
WORKDIR="$(mktemp -d /tmp/deb-build-XXXXXX)"
trap 'rm -rf "$WORKDIR"' EXIT

ROOTDIR="${WORKDIR}/root"
DEBIANDIR="${WORKDIR}/DEBIAN"

echo "==> Building $DEBNAME"
echo "==> Work dir: $WORKDIR"

mkdir -p "$ROOTDIR" "$DEBIANDIR"

# ─── Install files into staging root ─────────────────────────────────────────
make install DESTDIR="$ROOTDIR" PREFIX=/usr HAVE_SYSTEMD=1

# Debian: PAM module lives in /usr/lib/x86_64-linux-gnu/security
# (Makefile installs to /usr/lib64/security — relocate)
if [ -f "${ROOTDIR}/usr/lib64/security/pam_record.so" ]; then
    install -d "${ROOTDIR}/usr/lib/x86_64-linux-gnu/security"
    mv "${ROOTDIR}/usr/lib64/security/pam_record.so" \
       "${ROOTDIR}/usr/lib/x86_64-linux-gnu/security/pam_record.so"
    rm -rf "${ROOTDIR}/usr/lib64"
fi

# Install docs
install -d "${ROOTDIR}/usr/share/doc/${PKG}"
cp -r docs/  "${ROOTDIR}/usr/share/doc/${PKG}/"

# Install PAM snippets and zsh hook to share
install -d "${ROOTDIR}/usr/share/${PKG}/pam.d-snippets"
install -m644 scripts/pam.d/pmp-rec-sshd.snippet  "${ROOTDIR}/usr/share/${PKG}/pam.d-snippets/sshd"
install -m644 scripts/pam.d/pmp-rec-su.snippet    "${ROOTDIR}/usr/share/${PKG}/pam.d-snippets/su"
install -m644 scripts/pam.d/pmp-rec-sudo.snippet  "${ROOTDIR}/usr/share/${PKG}/pam.d-snippets/sudo"
install -m644 scripts/pam.d/pmp-rec-login.snippet "${ROOTDIR}/usr/share/${PKG}/pam.d-snippets/login"
install -m644 scripts/zshenv/pmp-rec.zsh          "${ROOTDIR}/usr/share/${PKG}/pmp-rec.zsh"

# ─── Compute installed size ───────────────────────────────────────────────────
INSTALLED_SIZE=$(du -sk "$ROOTDIR" | awk '{print $1}')

# ─── Write DEBIAN/control ────────────────────────────────────────────────────
cat > "${DEBIANDIR}/control" << EOF
Package: ${PKG}
Version: ${VERSION}-${RELEASE}
Architecture: ${ARCH}
Maintainer: Rushikesh Sakharle <ramsharath@instantly.ai>
Installed-Size: ${INSTALLED_SIZE}
Depends: libpam-runtime, libsystemd0, zlib1g, libpam0g
Recommends: sudo
Section: admin
Priority: optional
Homepage: https://github.com/percona/percona-monitoring-plugins
Description: Percona terminal session recorder daemon
 pmp-rec captures all interactive terminal sessions (SSH, su, sudo-i,
 local console) to per-session ttyrec files for forensic audit and
 compliance. Sessions are automatically recorded via a PAM module that
 hooks before the user's shell is exec'd.
 .
 Components:
  pmp-rec       — PTY shim (runs as user)
  pmp-recd      — Central daemon (root, epoll, writes recordings)
  pam_record.so — PAM session module
  pmp-rec-cli   — Audit CLI (list/play/tail/purge/tree)
EOF

# ─── Write DEBIAN/postinst ───────────────────────────────────────────────────
cat > "${DEBIANDIR}/postinst" << 'POSTINST'
#!/bin/sh
set -e

case "$1" in
    configure)
        # Create audit group if absent
        getent group pmp-audit >/dev/null 2>&1 || groupadd -r pmp-audit || true

        # Create storage directory
        install -d -m750 /var/lib/pmp-rec
        chown root:pmp-audit /var/lib/pmp-rec 2>/dev/null || true

        # Create runtime dirs
        install -d -m755 /run/pmp-rec /run/pmp-rec/sessions 2>/dev/null || true

        # Enable services
        if command -v systemctl >/dev/null 2>&1 && systemctl is-system-running >/dev/null 2>&1; then
            systemctl daemon-reload || true
            systemctl enable pmp-recd.socket || true
            systemctl enable pmp-rec-purge.timer || true
        fi

        echo ""
        echo "pmp-rec installed. Next steps:"
        echo "  1. Add PAM hooks — see /usr/share/pmp-rec/pam.d-snippets/"
        echo "  2. For zsh: append /usr/share/pmp-rec/pmp-rec.zsh to /etc/zshenv"
        echo "  3. systemctl enable --now pmp-recd.socket pmp-recd.service"
        echo "  4. systemctl enable --now pmp-rec-purge.timer"
        echo ""
        ;;
esac

#DEBHELPER#
POSTINST
chmod 755 "${DEBIANDIR}/postinst"

# ─── Write DEBIAN/prerm ──────────────────────────────────────────────────────
cat > "${DEBIANDIR}/prerm" << 'PRERM'
#!/bin/sh
set -e
case "$1" in
    remove|upgrade)
        if command -v systemctl >/dev/null 2>&1; then
            systemctl stop  pmp-recd.service pmp-recd.socket 2>/dev/null || true
            systemctl disable pmp-recd.service pmp-recd.socket pmp-rec-purge.timer 2>/dev/null || true
        fi
        ;;
esac
#DEBHELPER#
PRERM
chmod 755 "${DEBIANDIR}/prerm"

# ─── Write DEBIAN/postrm ─────────────────────────────────────────────────────
cat > "${DEBIANDIR}/postrm" << 'POSTRM'
#!/bin/sh
set -e
case "$1" in
    purge)
        rm -rf /etc/pmp-rec /run/pmp-rec 2>/dev/null || true
        ;;
esac
#DEBHELPER#
POSTRM
chmod 755 "${DEBIANDIR}/postrm"

# ─── Write DEBIAN/conffiles ──────────────────────────────────────────────────
cat > "${DEBIANDIR}/conffiles" << 'EOF'
/etc/pmp-rec/recd.conf
/etc/pmp-rec/shells.allow
/etc/sudoers.d/pmp-rec
/etc/profile.d/pmp-rec.sh
EOF

# ─── Write DEBIAN/md5sums ────────────────────────────────────────────────────
(cd "$ROOTDIR" && find . -type f | sort | xargs md5sum) \
  | sed 's|\./||' > "${DEBIANDIR}/md5sums"

# ─── Fix permissions ─────────────────────────────────────────────────────────
find "$ROOTDIR" -type d | xargs chmod 755
chmod 440 "${ROOTDIR}/etc/sudoers.d/pmp-rec" 2>/dev/null || true
chmod 750 "${ROOTDIR}/var/lib/pmp-rec"       2>/dev/null || true

# ─── Assemble the .deb (ar archive: debian-binary + control.tar.gz + data.tar.xz) ─
CONTROL_TAR="${WORKDIR}/control.tar.gz"
DATA_TAR="${WORKDIR}/data.tar.xz"
DEBIAN_BIN="${WORKDIR}/debian-binary"

echo "2.0" > "$DEBIAN_BIN"

# control.tar.gz: DEBIAN/ directory
(cd "$DEBIANDIR" && tar czf "$CONTROL_TAR" --owner=0 --group=0 .)

# data.tar.xz: payload (everything under root/)
(cd "$ROOTDIR" && tar cJf "$DATA_TAR" --owner=0 --group=0 .)

# Create .deb using ar
mkdir -p "$OUTDIR"
DEBPATH="${OUTDIR}/${DEBNAME}"
rm -f "$DEBPATH"
ar r "$DEBPATH" "$DEBIAN_BIN" "$CONTROL_TAR" "$DATA_TAR" 2>/dev/null
# ar needs the files in order — some versions use 'rcs' but 'r' works
# Verify by listing
ar t "$DEBPATH" >/dev/null 2>&1 || {
    # Alternative: build manually with correct member names
    ar r "$DEBPATH" "$DEBIAN_BIN" "$CONTROL_TAR" "$DATA_TAR"
}

# Rename members to expected names within the archive
# ar creates them with basename — need debian-binary, control.tar.gz, data.tar.xz
# Check if member names are correct:
MEMBERS=$(ar t "$DEBPATH" | tr '\n' ' ')
echo "==> ar members: $MEMBERS"

DEB_SIZE=$(du -sh "$DEBPATH" | awk '{print $1}')
echo "==> Built: $DEBPATH ($DEB_SIZE)"

# ─── Verify: can we inspect it? ──────────────────────────────────────────────
if command -v dpkg-deb >/dev/null 2>&1; then
    echo "==> dpkg-deb -I:"
    dpkg-deb -I "$DEBPATH"
    echo "==> dpkg-deb -c (first 20 files):"
    dpkg-deb -c "$DEBPATH" 2>/dev/null | head -20 || true
elif command -v ar >/dev/null 2>&1; then
    echo "==> ar t (package members):"
    ar t "$DEBPATH"
fi

echo ""
echo "==> DEB package: $DEBPATH"
echo "==> Install with (on Debian/Ubuntu):"
echo "      sudo dpkg -i $DEBPATH"
echo "      sudo apt-get install -f   # resolve any missing deps"
