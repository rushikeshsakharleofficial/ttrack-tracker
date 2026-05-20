Name:           trackterm-rec
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
trackterm-rec captures all interactive terminal sessions — SSH, su, sudo-i,
local console — to per-session ttyrec files for forensic audit and
compliance. Sessions are automatically recorded via a PAM module that
hooks before the user's shell is exec'd.

Components:
  trackterm-rec       — PTY shim (runs as user)
  trackterm-recd      — Central daemon (root, epoll, writes recordings)
  pam_record.so — PAM session module
  trackterm-cli   — Audit CLI (list/play/tail/purge/tree)

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
install -d %{buildroot}/etc/trackterm-rec
install -d %{buildroot}/var/lib/trackterm-rec
install -d %{buildroot}/usr/share/trackterm-rec/pam.d-snippets

install -m755 build/trackterm-rec        %{buildroot}%{_libexecdir}/trackterm-rec
install -m755 build/trackterm-recd       %{buildroot}%{_libexecdir}/trackterm-recd
install -m755 build/trackterm-cli    %{buildroot}%{_bindir}/trackterm-cli
install -m755 build/pam_record.so  %{buildroot}/usr/lib64/security/pam_record.so

install -m644 scripts/systemd/trackterm-recd.service      %{buildroot}%{_unitdir}/trackterm-recd.service
install -m644 scripts/systemd/trackterm-recd.socket       %{buildroot}%{_unitdir}/trackterm-recd.socket
install -m644 scripts/systemd/trackterm-rec-purge.service %{buildroot}%{_unitdir}/trackterm-rec-purge.service
install -m644 scripts/systemd/trackterm-rec-purge.timer   %{buildroot}%{_unitdir}/trackterm-rec-purge.timer

install -m644 scripts/tmpfiles.d/trackterm-rec.conf       %{buildroot}/usr/lib/tmpfiles.d/trackterm-rec.conf
install -m644 scripts/profile.d/trackterm-rec.sh          %{buildroot}/etc/profile.d/trackterm-rec.sh
install -m440 scripts/sudoers.d/trackterm-rec             %{buildroot}/etc/sudoers.d/trackterm-rec
install -m644 config/recd.conf.sample               %{buildroot}/etc/trackterm-rec/recd.conf
install -m644 config/shells.allow.sample            %{buildroot}/etc/trackterm-rec/shells.allow

install -m644 scripts/pam.d/trackterm-rec-sshd.snippet    %{buildroot}/usr/share/trackterm-rec/pam.d-snippets/sshd
install -m644 scripts/pam.d/trackterm-rec-su.snippet      %{buildroot}/usr/share/trackterm-rec/pam.d-snippets/su
install -m644 scripts/pam.d/trackterm-rec-sudo.snippet    %{buildroot}/usr/share/trackterm-rec/pam.d-snippets/sudo
install -m644 scripts/pam.d/trackterm-rec-login.snippet   %{buildroot}/usr/share/trackterm-rec/pam.d-snippets/login
install -m644 scripts/zshenv/trackterm-rec.zsh            %{buildroot}/usr/share/trackterm-rec/trackterm-rec.zsh

%post
%systemd_post trackterm-recd.socket trackterm-recd.service trackterm-rec-purge.timer
systemd-tmpfiles --create /usr/lib/tmpfiles.d/trackterm-rec.conf 2>/dev/null || :
# Create audit group if absent
getent group trackterm-audit >/dev/null || groupadd -r trackterm-audit
# Set storage directory ownership
chown root:trackterm-audit /var/lib/trackterm-rec 2>/dev/null || :
chmod 750            /var/lib/trackterm-rec 2>/dev/null || :

echo ""
echo "trackterm-rec installed. Next steps:"
echo "  1. Add PAM hooks (see /usr/share/trackterm-rec/pam.d-snippets/)"
echo "  2. For zsh: append /usr/share/trackterm-rec/trackterm-rec.zsh to /etc/zshenv"
echo "  3. systemctl enable --now trackterm-recd.socket trackterm-recd.service"
echo "  4. systemctl enable --now trackterm-rec-purge.timer"
echo ""

%preun
%systemd_preun trackterm-recd.service trackterm-recd.socket trackterm-rec-purge.timer

%postun
%systemd_postun_with_restart trackterm-recd.service

%files
%license LICENSE
%doc docs/
%{_libexecdir}/trackterm-rec
%{_libexecdir}/trackterm-recd
%{_bindir}/trackterm-cli
/usr/lib64/security/pam_record.so
%{_unitdir}/trackterm-recd.service
%{_unitdir}/trackterm-recd.socket
%{_unitdir}/trackterm-rec-purge.service
%{_unitdir}/trackterm-rec-purge.timer
/usr/lib/tmpfiles.d/trackterm-rec.conf
/etc/profile.d/trackterm-rec.sh
%config(noreplace) /etc/sudoers.d/trackterm-rec
%dir /etc/trackterm-rec
%config(noreplace) /etc/trackterm-rec/recd.conf
%config(noreplace) /etc/trackterm-rec/shells.allow
%dir %attr(750,root,root) /var/lib/trackterm-rec
%dir /usr/share/trackterm-rec
%dir /usr/share/trackterm-rec/pam.d-snippets
/usr/share/trackterm-rec/pam.d-snippets/
/usr/share/trackterm-rec/trackterm-rec.zsh

%changelog
* Tue May 20 2026 Rushikesh Sakharle <ramsharath@instantly.ai> - 0.1.0-1
- Initial release
