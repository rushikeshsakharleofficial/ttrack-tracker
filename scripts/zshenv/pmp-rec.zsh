# Percona Monitoring Plugins — terminal recorder shim launcher (zsh)
# Append this block to /etc/zshenv (zsh does not source /etc/profile.d).

if [[ -o interactive ]] && [[ -t 0 ]]; then
  if [[ -z "${PMP_REC_ACTIVE:-}" ]] && [[ "${PMP_REC_SHIM_CHILD:-0}" != "1" ]]; then
    if [[ -x /usr/libexec/pmp-rec ]]; then
      exec /usr/libexec/pmp-rec
    fi
  fi
fi
