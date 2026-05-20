Name:           pmp-rec
Version:        0.1.0
Release:        1%{?dist}
Summary:        Percona terminal session recorder daemon
License:        GPL-2.0-only
URL:            https://github.com/percona/percona-monitoring-plugins
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  gcc make
BuildRequires:  pam-devel
BuildRequires:  systemd-devel
BuildRequires:  zlib-devel

Requires:       systemd
Requires(post):   systemd
Requires(preun):  systemd
Requires(postun): systemd

%description
pmp-rec captures all interactive terminal sessions — SSH, su, sudo-i,
local console — to per-session ttyrec files for forensic audit and
compliance. Sessions are automatically recorded via a PAM module that
hooks before the user's shell is exec'd.

Components:
  pmp-rec       — PTY shim (runs as user)
  pmp-recd      — Central daemon (root, epoll, writes recordings)
  pam_record.so — PAM session module
  pmp-rec-cli   — Audit CLI (list/play/tail/purge/tree)

%prep
%autosetup

%build
make %{?_smp_mflags} HAVE_SYSTEMD=1

%install
rm -rf %{buildroot}
install -d %{buildroot}%{_libexecdir}
install -d %{buildroot}%{_bindir}
install -d %{buildroot}/usr/lib64/security
install -d %{buildroot}%{_unitdir}
install -d %{buildroot}/usr/lib/tmpfiles.d
install -d %{buildroot}/etc/profile.d
install -d %{buildroot}/etc/sudoers.d
install -d %{buildroot}/etc/pmp-rec
install -d %{buildroot}/var/lib/pmp-rec
install -d %{buildroot}/usr/share/pmp-rec/pam.d-snippets

install -m755 build/pmp-rec        %{buildroot}%{_libexecdir}/pmp-rec
install -m755 build/pmp-recd       %{buildroot}%{_libexecdir}/pmp-recd
install -m755 build/pmp-rec-cli    %{buildroot}%{_bindir}/pmp-rec-cli
install -m755 build/pam_record.so  %{buildroot}/usr/lib64/security/pam_record.so

install -m644 scripts/systemd/pmp-recd.service      %{buildroot}%{_unitdir}/pmp-recd.service
install -m644 scripts/systemd/pmp-recd.socket       %{buildroot}%{_unitdir}/pmp-recd.socket
install -m644 scripts/systemd/pmp-rec-purge.service %{buildroot}%{_unitdir}/pmp-rec-purge.service
install -m644 scripts/systemd/pmp-rec-purge.timer   %{buildroot}%{_unitdir}/pmp-rec-purge.timer

install -m644 scripts/tmpfiles.d/pmp-rec.conf       %{buildroot}/usr/lib/tmpfiles.d/pmp-rec.conf
install -m644 scripts/profile.d/pmp-rec.sh          %{buildroot}/etc/profile.d/pmp-rec.sh
install -m440 scripts/sudoers.d/pmp-rec             %{buildroot}/etc/sudoers.d/pmp-rec
install -m644 config/recd.conf.sample               %{buildroot}/etc/pmp-rec/recd.conf
install -m644 config/shells.allow.sample            %{buildroot}/etc/pmp-rec/shells.allow

install -m644 scripts/pam.d/pmp-rec-sshd.snippet    %{buildroot}/usr/share/pmp-rec/pam.d-snippets/sshd
install -m644 scripts/pam.d/pmp-rec-su.snippet      %{buildroot}/usr/share/pmp-rec/pam.d-snippets/su
install -m644 scripts/pam.d/pmp-rec-sudo.snippet    %{buildroot}/usr/share/pmp-rec/pam.d-snippets/sudo
install -m644 scripts/pam.d/pmp-rec-login.snippet   %{buildroot}/usr/share/pmp-rec/pam.d-snippets/login
install -m644 scripts/zshenv/pmp-rec.zsh            %{buildroot}/usr/share/pmp-rec/pmp-rec.zsh

%post
%systemd_post pmp-recd.socket pmp-recd.service pmp-rec-purge.timer
systemd-tmpfiles --create /usr/lib/tmpfiles.d/pmp-rec.conf 2>/dev/null || :
# Create audit group if absent
getent group pmp-audit >/dev/null || groupadd -r pmp-audit
# Set storage directory ownership
chown root:pmp-audit /var/lib/pmp-rec 2>/dev/null || :
chmod 750            /var/lib/pmp-rec 2>/dev/null || :

echo ""
echo "pmp-rec installed. Next steps:"
echo "  1. Add PAM hooks (see /usr/share/pmp-rec/pam.d-snippets/)"
echo "  2. For zsh: append /usr/share/pmp-rec/pmp-rec.zsh to /etc/zshenv"
echo "  3. systemctl enable --now pmp-recd.socket pmp-recd.service"
echo "  4. systemctl enable --now pmp-rec-purge.timer"
echo ""

%preun
%systemd_preun pmp-recd.service pmp-recd.socket pmp-rec-purge.timer

%postun
%systemd_postun_with_restart pmp-recd.service

%files
%license LICENSE
%doc docs/
%{_libexecdir}/pmp-rec
%{_libexecdir}/pmp-recd
%{_bindir}/pmp-rec-cli
/usr/lib64/security/pam_record.so
%{_unitdir}/pmp-recd.service
%{_unitdir}/pmp-recd.socket
%{_unitdir}/pmp-rec-purge.service
%{_unitdir}/pmp-rec-purge.timer
/usr/lib/tmpfiles.d/pmp-rec.conf
/etc/profile.d/pmp-rec.sh
%config(noreplace) /etc/sudoers.d/pmp-rec
%dir /etc/pmp-rec
%config(noreplace) /etc/pmp-rec/recd.conf
%config(noreplace) /etc/pmp-rec/shells.allow
%dir %attr(750,root,root) /var/lib/pmp-rec
%dir /usr/share/pmp-rec
%dir /usr/share/pmp-rec/pam.d-snippets
/usr/share/pmp-rec/pam.d-snippets/
/usr/share/pmp-rec/pmp-rec.zsh

%changelog
* Tue May 20 2026 Rushikesh Sakharle <ramsharath@instantly.ai> - 0.1.0-1
- Initial release
