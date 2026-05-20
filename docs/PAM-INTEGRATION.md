# PAM Integration

## How it works

`pam_record.so` is a PAM session module. On `pam_sm_open_session`:

1. Mints a new UUIDv4 session ID via `/proc/sys/kernel/random/uuid`
2. Reads any existing `TRACKTERM_REC_SID` from PAM environment (set by parent session)
3. Promotes old SID → `TRACKTERM_REC_PARENT`, stamps new `TRACKTERM_REC_SID`
4. Writes a marker file to `/run/trackterm-rec/sessions/<sid>.json`
5. Returns `PAM_SUCCESS` (fail-open; never blocks login)

The shim (`trackterm-rec`) reads `TRACKTERM_REC_SID` and `TRACKTERM_REC_PARENT` on startup.

## Installation by service

### RHEL 9 — `/etc/pam.d/sshd`
Add after `session required pam_loginuid.so`:
```
session    optional     pam_record.so service=sshd
```

### Debian 12 — `/etc/pam.d/sshd`
Add after `@include common-session`:
```
session    optional     pam_record.so service=sshd
```

### `/etc/pam.d/su` (both)
Append before `pam_xauth.so`:
```
session    optional     pam_record.so service=su
```

### `/etc/pam.d/sudo` and `/etc/pam.d/sudo-i`
```
session    optional     pam_record.so service=sudo
```

### `/etc/pam.d/login`
After `pam_loginuid.so`:
```
session    optional     pam_record.so service=login
```

## sudo env_keep

Add `/etc/sudoers.d/trackterm-rec` (mode 0440):
```
Defaults env_keep += "TRACKTERM_REC_SID TRACKTERM_REC_PARENT TRACKTERM_REC_ACTIVE TRACKTERM_REC_LOGINUID TRACKTERM_REC_SHIM_CHILD"
```

## zsh

zsh does not source `/etc/profile.d`. Append to `/etc/zshenv`:
```zsh
if [[ -o interactive ]] && [[ -t 0 ]] && [[ -t 1 ]]; then
  if [[ -z "${TRACKTERM_REC_ACTIVE:-}" ]] && [[ "${TRACKTERM_REC_SHIM_CHILD:-0}" != "1" ]]; then
    [[ -x /usr/libexec/trackterm-rec ]] && { export TRACKTERM_REC_ACTIVE=1; exec /usr/libexec/trackterm-rec; }
  fi
fi
```

## Promotion from optional to required (M6)

After validating that no logins are broken:
```
session    required     pam_record.so service=sshd
```

This blocks the session if the module returns `PAM_SESSION_ERR`. Currently the
module always returns `PAM_SUCCESS` (fail-open); change `pam_sm_open_session`
to return `PAM_SESSION_ERR` on UUID failure if fail-closed is desired.
